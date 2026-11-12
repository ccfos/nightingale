package doris

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/glebarez/sqlite"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func sqliteDoris(t *testing.T, database string) (*Doris, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	d := &Doris{Addr: "unit-" + t.Name(), User: "reader", Password: "secret", MaxQueryRows: 5}
	key := d.Addr + ":reader:secret"
	if database != "" {
		key += ":" + database
	}
	previous, existed := pool.PoolClient.Load(key)
	pool.PoolClient.Store(key, sqlDB)
	t.Cleanup(func() {
		pool.PoolClient.Delete(key)
		if existed {
			pool.PoolClient.Store(key, previous)
		}
		_ = sqlDB.Close()
	})
	return d, db
}

func TestNewDorisWithSettings_DecodesMapAndJSON(t *testing.T) {
	tests := []struct {
		name     string
		settings interface{}
	}{
		{name: "map", settings: map[string]interface{}{"doris.addr": "fe:9030", "doris.max_query_rows": 25}},
		{name: "json", settings: `{"doris.addr":"fe:9030","doris.max_query_rows":25}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := NewDorisWithSettings(context.Background(), tt.settings)

			require.NoError(t, err)
			require.Equal(t, "fe:9030", d.Addr)
			require.Equal(t, 25, d.MaxQueryRows)
		})
	}
}

func TestNewDorisWithSettings_RejectsMalformedSettings(t *testing.T) {
	for _, settings := range []interface{}{42, "{", map[string]interface{}{"doris.timeout": []string{"bad"}}} {
		d, err := NewDorisWithSettings(context.Background(), settings)

		require.Error(t, err)
		require.Nil(t, d)
	}
}

func TestConnection_RejectsUnsafeDatabaseIdentifiersBeforeOpening(t *testing.T) {
	d := &Doris{Addr: "unused:9030", EnableWrite: true, UserWrite: "writer"}
	tests := []struct {
		name    string
		connect func(context.Context, string) (*sql.DB, error)
	}{
		{name: "read", connect: d.NewConn},
		{name: "write", connect: d.NewWriteConn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := tt.connect(context.Background(), "db;DROP DATABASE other")

			require.ErrorContains(t, err, "doris connect")
			require.Nil(t, conn)
		})
	}
}

func TestMetadataQueries_RejectInvalidIdentifiers(t *testing.T) {
	d := &Doris{}
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "show tables",
			run: func() error {
				_, err := d.ShowTables(context.Background(), "bad;db")
				return err
			},
		},
		{
			name: "describe database",
			run: func() error {
				_, err := d.DescTable(context.Background(), "bad;db", "tbl")
				return err
			},
		},
		{
			name: "describe table",
			run: func() error {
				_, err := d.DescTable(context.Background(), "db", "bad;tbl")
				return err
			},
		},
		{
			name: "indexes missing names",
			run: func() error {
				_, err := d.ShowIndexes(context.Background(), "", "")
				return err
			},
		},
		{
			name: "indexes database",
			run: func() error {
				_, err := d.ShowIndexes(context.Background(), "bad;db", "tbl")
				return err
			},
		},
		{
			name: "indexes table",
			run: func() error {
				_, err := d.ShowIndexes(context.Background(), "db", "bad;tbl")
				return err
			},
		},
		{
			name: "select database",
			run: func() error {
				_, err := d.SelectRows(context.Background(), "bad;db", "tbl", "")
				return err
			},
		},
		{
			name: "select table",
			run: func() error {
				_, err := d.SelectRows(context.Background(), "db", "bad;tbl", "")
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()

			require.Error(t, err)
		})
	}
}

func TestExecQuery_ConvertsByteColumnsAndReportsHook(t *testing.T) {
	d, _ := sqliteDoris(t, "")
	previous := OnQuery
	t.Cleanup(func() { OnQuery = previous })
	var observed QueryEvent
	var observedCtx context.Context
	OnQuery = func(c context.Context, event QueryEvent) { observedCtx, observed = c, event }
	c := types.WithCallContext(context.Background(), types.CallContext{DatasourceID: 7, Operator: "alice"})
	query := "SELECT CAST('api' AS BLOB) AS job, 3 AS value, NULL AS optional"

	rows, err := d.ExecQuery(c, "", query)

	require.NoError(t, err)
	require.Equal(t, []map[string]interface{}{{"job": "api", "value": int64(3), "optional": nil}}, rows)
	require.Equal(t, query, observed.SQL)
	require.Equal(t, "", observed.Database)
	require.Equal(t, 1, observed.RowCount)
	require.NoError(t, observed.Err)
	call, ok := types.CallContextFromCtx(observedCtx)
	require.True(t, ok)
	require.Equal(t, int64(7), call.DatasourceID)
}

func TestExecQuery_ReportsQueryFailureToHook(t *testing.T) {
	d, _ := sqliteDoris(t, "")
	previous := OnQuery
	t.Cleanup(func() { OnQuery = previous })
	var observed QueryEvent
	OnQuery = func(_ context.Context, event QueryEvent) { observed = event }

	rows, err := d.ExecQuery(context.Background(), "", "SELECT * FROM missing_table")

	require.Error(t, err)
	require.Empty(t, rows)
	require.Error(t, observed.Err)
	require.Zero(t, observed.RowCount)
	require.Equal(t, "SELECT * FROM missing_table", observed.SQL)
}

func TestQuery_RejectsWriteStatements(t *testing.T) {
	d := &Doris{}

	rows, err := d.Query(context.Background(), &QueryParam{Sql: "DROP TABLE users"})

	require.ErrorContains(t, err, "operation DROP is forbid")
	require.Nil(t, rows)
}

func TestCheckMaxQueryRows_ProbesAndRejectsExcessRows(t *testing.T) {
	d, _ := sqliteDoris(t, "")
	d.MaxQueryRows = 1
	previous := OnQuery
	t.Cleanup(func() { OnQuery = previous })
	var queries []string
	OnQuery = func(_ context.Context, event QueryEvent) { queries = append(queries, event.SQL) }

	err := d.CheckMaxQueryRows(context.Background(), "", " SELECT 1 UNION ALL SELECT 2; ")

	require.ErrorContains(t, err, "exceeds the maximum limit 1")
	require.Equal(t, []string{"SELECT 1 FROM (SELECT 1 UNION ALL SELECT 2) AS __probe_chk LIMIT 2"}, queries)
}

func TestQueryTimeseries_GroupsLabelsAndSortsPoints(t *testing.T) {
	d, _ := sqliteDoris(t, "")
	query := &QueryParam{
		Sql:  "SELECT 200 AS ts, 4 AS value, 'api' AS job UNION ALL SELECT 100, 2, 'api'",
		Keys: types.Keys{TimeKey: "ts", ValueKey: "value", LabelKey: "job"},
	}

	series, err := d.QueryTimeseries(context.Background(), query)

	require.NoError(t, err)
	require.Equal(t, []types.MetricValues{{
		Metric: model.Metric{"__name__": "value", "job": "api"}, Values: [][]float64{{100, 2}, {200, 4}},
	}}, series)
}

func TestSelectRows_QuotesIdentifiersAndAppliesFilter(t *testing.T) {
	d, db := sqliteDoris(t, "main")
	require.NoError(t, db.AutoMigrate(&models.User{}))
	require.NoError(t, db.Create(&models.User{Username: "alice"}).Error)
	require.NoError(t, db.Create(&models.User{Username: "bob"}).Error)
	previous := OnQuery
	t.Cleanup(func() { OnQuery = previous })
	var query string
	OnQuery = func(_ context.Context, event QueryEvent) { query = event.SQL }

	rows, err := d.SelectRows(context.Background(), "main", "users", "WHERE username = 'alice' LIMIT 1")

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "alice", rows[0]["username"])
	require.Equal(t, "SELECT * FROM `main`.`users` WHERE username = 'alice' LIMIT 1", query)
}

func TestQueryHistogram_ReturnsSortedPoints(t *testing.T) {
	d, _ := sqliteDoris(t, "")

	points, err := d.QueryHistogram(context.Background(), &QueryParam{
		Sql:  "SELECT 200 AS time, 4 AS count UNION ALL SELECT 100, 2",
		Keys: types.Keys{ValueKey: "count"},
	})

	require.NoError(t, err)
	require.Equal(t, [][]float64{{100, 2}, {200, 4}}, points)
}

func TestQueryHistogram_ReturnsQueryError(t *testing.T) {
	d := &Doris{}

	points, err := d.QueryHistogram(context.Background(), &QueryParam{Sql: "DROP TABLE users"})

	require.ErrorContains(t, err, "operation DROP is forbid")
	require.Empty(t, points)
}

func TestSplitSQLStatements_PreservesQuotedAndCommentSemicolons(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "empty", input: " \n"},
		{name: "quoted semicolon", input: "SELECT 'a;b'; SELECT 2", want: []string{"SELECT 'a;b';", "SELECT 2"}},
		{name: "escaped quote", input: `SELECT 'a\';b'; SELECT 2;`, want: []string{`SELECT 'a\';b';`, "SELECT 2;"}},
		{
			name: "block comment", input: "SELECT /* ; */ 1; SELECT 2;",
			want: []string{"SELECT /* ; */ 1;", "SELECT 2;"},
		},
		{
			name: "line comment", input: "SELECT 1; -- comment;\nSELECT 2;",
			want: []string{"SELECT 1; -- comment;", "SELECT 2;"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statements := SplitSQLStatements(tt.input)

			require.Equal(t, tt.want, statements)
		})
	}
}

func TestNewConn_AppliesDefaultsAndReusesLazyConnection(t *testing.T) {
	d := &Doris{Addr: "127.0.0.1:1", User: t.Name()}
	key := d.Addr + ":" + d.User + ":"
	previous, existed := pool.PoolClient.Load(key)
	t.Cleanup(func() {
		pool.PoolClient.Delete(key)
		if existed {
			pool.PoolClient.Store(key, previous)
		}
	})

	// sql.Open creates a lazy handle. Never Ping or query this MySQL connection.
	first, err := d.NewConn(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	second, err := d.NewConn(context.Background(), "")

	require.NoError(t, err)
	require.Same(t, first, second)
	require.Equal(t, 60000, d.Timeout)
	require.Equal(t, 10, d.MaxIdleConns)
	require.Equal(t, 100, d.MaxOpenConns)
	require.Equal(t, 14400, d.ConnMaxLifetime)
	require.Equal(t, 500, d.MaxQueryRows)
	require.Equal(t, 100, first.Stats().MaxOpenConnections)
}

func TestNewWriteConn_UsesDedicatedCredentialsAndConservativePool(t *testing.T) {
	d := &Doris{Addr: "127.0.0.1:1", User: "reader", EnableWrite: true, UserWrite: t.Name()}
	key := d.Addr + ":" + d.UserWrite + "::metrics"
	previous, existed := pool.PoolClient.Load(key)
	t.Cleanup(func() {
		pool.PoolClient.Delete(key)
		if existed {
			pool.PoolClient.Store(key, previous)
		}
	})

	// The lazy handle lets us inspect pooling without a real MySQL connection.
	first, err := d.NewWriteConn(context.Background(), "metrics")
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	second, err := d.NewWriteConn(context.Background(), "metrics")

	require.NoError(t, err)
	require.Same(t, first, second)
	cached, found := pool.PoolClient.Load(key)
	require.True(t, found)
	require.Same(t, first, cached)
	require.Equal(t, 10, first.Stats().MaxOpenConnections)
}

func TestNewWriteConn_UsesReadConnectionWithoutWriteCredentials(t *testing.T) {
	d, db := sqliteDoris(t, "")
	expected, err := db.DB()
	require.NoError(t, err)

	got, err := d.NewWriteConn(context.Background(), "")

	require.NoError(t, err)
	require.Same(t, expected, got)
}

func TestExecBatchSQL_ExecutesStatementsInOrder(t *testing.T) {
	d, db := sqliteDoris(t, "")
	require.NoError(t, db.AutoMigrate(&models.User{}))

	err := d.ExecBatchSQL(context.Background(), "", "INSERT INTO users (username) VALUES ('alice'); "+
		"UPDATE users SET username = 'updated' WHERE username = 'alice';")

	require.NoError(t, err)
	var users []models.User
	require.NoError(t, db.Find(&users).Error)
	require.Len(t, users, 1)
	require.Equal(t, "updated", users[0].Username)
}
