package mysql

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/glebarez/sqlite"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func cachedSQLiteMySQL(t *testing.T, database string) (*MySQL, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	m := &MySQL{Shards: []Shard{{Addr: "unit-" + t.Name(), User: "reader"}}}
	key := m.Shards[0].Addr + "::reader"
	if database != "" {
		key += ":" + database
	}
	previous, existed := pool.PoolClient.Load(key)
	pool.PoolClient.Store(key, db)
	t.Cleanup(func() {
		pool.PoolClient.Delete(key)
		if existed {
			pool.PoolClient.Store(key, previous)
		}
		require.NoError(t, sqlDB.Close())
	})
	return m, db
}

func TestNewMySQLWithSettings_DecodesShardValues(t *testing.T) {
	settings := map[string]interface{}{
		"mysql.shards": []map[string]interface{}{{
			"mysql.addr": "db:3306", "mysql.user": "reader", "mysql.max_query_rows": 25,
		}},
	}

	m, err := NewMySQLWithSettings(context.Background(), settings)

	require.NoError(t, err)
	require.Equal(t, []Shard{{Addr: "db:3306", User: "reader", MaxQueryRows: 25}}, m.Shards)
}

func TestNewMySQLWithSettings_RejectsMalformedSettings(t *testing.T) {
	for _, settings := range []interface{}{"{", map[string]interface{}{"mysql.shards": "not an array"}} {
		m, err := NewMySQLWithSettings(context.Background(), settings)

		require.Error(t, err)
		require.Nil(t, m)
	}
}

func TestNewConn_RejectsInvalidConfigurationBeforeDialing(t *testing.T) {
	tests := []struct {
		name     string
		shards   []Shard
		database string
		want     string
	}{
		{name: "unsafe database", database: "db;DROP DATABASE x", want: "mysql connect"},
		{name: "missing shards", want: "empty mysql shards"},
		{name: "missing address", shards: []Shard{{User: "reader"}}, want: "empty addr"},
		{
			name: "invalid DSN parameters", shards: []Shard{{Addr: "unused:3306", DsnExtraParams: "bad"}},
			want: "invalid mysql.dsn_extra_params",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MySQL{Shards: tt.shards}

			db, err := m.NewConn(context.Background(), tt.database)

			require.ErrorContains(t, err, tt.want)
			require.Nil(t, db)
		})
	}
}

func TestNewConn_ReusesDatabaseAndNormalizedParameterCacheKey(t *testing.T) {
	m, db := cachedSQLiteMySQL(t, "metrics")
	m.Shards[0].DsnExtraParams = "?timeout=5s"
	key := m.Shards[0].Addr + "::reader:metrics:timeout=5s"
	previous, existed := pool.PoolClient.Load(key)
	pool.PoolClient.Store(key, db)
	t.Cleanup(func() {
		pool.PoolClient.Delete(key)
		if existed {
			pool.PoolClient.Store(key, previous)
		}
	})

	got, err := m.NewConn(context.Background(), "metrics")

	require.NoError(t, err)
	require.Same(t, db, got)
}

func TestSelectRows_QuotesTableAndAppliesPredicate(t *testing.T) {
	m, db := cachedSQLiteMySQL(t, "main")
	require.NoError(t, db.AutoMigrate(&models.User{}))
	require.NoError(t, db.Create(&models.User{Username: "alice"}).Error)
	require.NoError(t, db.Create(&models.User{Username: "bob"}).Error)

	rows, err := m.SelectRows(context.Background(), "main", "users", "username = 'alice'")

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "alice", rows[0]["username"])
}

func TestSelectRows_EmptyPredicateReturnsAllRows(t *testing.T) {
	m, db := cachedSQLiteMySQL(t, "")
	require.NoError(t, db.AutoMigrate(&models.User{}))
	require.NoError(t, db.Create(&models.User{Username: "alice"}).Error)

	rows, err := m.SelectRows(context.Background(), "", "users", "")

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "alice", rows[0]["username"])
}

func TestExecQuery_ConvertsByteValuesAndPreservesNull(t *testing.T) {
	m, _ := cachedSQLiteMySQL(t, "")

	rows, err := m.ExecQuery(context.Background(), "", "SELECT CAST('api' AS BLOB) AS job, NULL AS optional, 7 AS count")

	require.NoError(t, err)
	require.Equal(t, []map[string]interface{}{{"job": "api", "optional": nil, "count": int64(7)}}, rows)
}

func TestQuery_ReturnsRowsWithinLimit(t *testing.T) {
	m, _ := cachedSQLiteMySQL(t, "")
	m.Shards[0].MaxQueryRows = 2

	rows, err := m.Query(context.Background(), &sqlbase.QueryParam{Sql: "SELECT 1 AS value UNION ALL SELECT 2;"})

	require.NoError(t, err)
	require.Equal(t, []map[string]interface{}{{"value": int64(1)}, {"value": int64(2)}}, rows)
}

func TestQuery_RejectsRowsOverLimit(t *testing.T) {
	m, _ := cachedSQLiteMySQL(t, "")
	m.Shards[0].MaxQueryRows = 1

	rows, err := m.Query(context.Background(), &sqlbase.QueryParam{Sql: "SELECT 1 AS value UNION ALL SELECT 2"})

	require.ErrorContains(t, err, "rows count 2 exceeds the maximum limit 1")
	require.Nil(t, rows)
}

func TestQueryTimeseries_GroupsLabelsAndSortsPoints(t *testing.T) {
	m, _ := cachedSQLiteMySQL(t, "")
	query := &sqlbase.QueryParam{
		Sql:  "SELECT 200 AS ts, 4 AS value, 'api' AS job UNION ALL SELECT 100, 2, 'api'",
		Keys: types.Keys{TimeKey: "ts", ValueKey: "value", LabelKey: "job"},
	}

	series, err := m.QueryTimeseries(context.Background(), query)

	require.NoError(t, err)
	require.Equal(t, []types.MetricValues{{
		Metric: model.Metric{"__name__": "value", "job": "api"}, Values: [][]float64{{100, 2}, {200, 4}},
	}}, series)
}

func TestQueryTimeseries_PropagatesInvalidSQL(t *testing.T) {
	m, _ := cachedSQLiteMySQL(t, "")

	series, err := m.QueryTimeseries(context.Background(), &sqlbase.QueryParam{Sql: "SELECT * FROM missing_table"})

	require.Error(t, err)
	require.Nil(t, series)
}

func TestCheckMaxQueryRows_DefaultLimitAllowsSmallQueries(t *testing.T) {
	m, db := cachedSQLiteMySQL(t, "")

	err := m.CheckMaxQueryRows(db, context.Background(), &sqlbase.QueryParam{Sql: "SELECT 1 AS value;"})

	require.NoError(t, err)
}

func TestQueryMethods_PropagateMissingConnectionConfiguration(t *testing.T) {
	m := &MySQL{}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "databases", run: func() error { _, err := m.ShowDatabases(context.Background()); return err }},
		{name: "tables", run: func() error { _, err := m.ShowTables(context.Background(), "db"); return err }},
		{name: "describe", run: func() error { _, err := m.DescTable(context.Background(), "db", "tbl"); return err }},
		{name: "select", run: func() error { _, err := m.SelectRows(context.Background(), "db", "tbl", ""); return err }},
		{name: "exec", run: func() error { _, err := m.ExecQuery(context.Background(), "db", "SELECT 1"); return err }},
		{name: "query", run: func() error { _, err := m.Query(context.Background(), &sqlbase.QueryParam{}); return err }},
		{
			name: "timeseries",
			run:  func() error { _, err := m.QueryTimeseries(context.Background(), &sqlbase.QueryParam{}); return err },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()

			require.ErrorContains(t, err, "empty mysql shards")
		})
	}
}

func TestTableQueries_RejectUnsafeTableIdentifiers(t *testing.T) {
	m := &MySQL{}

	_, err := m.SelectRows(context.Background(), "db", "tbl;DROP TABLE other", "")

	require.ErrorContains(t, err, "select rows")
	_, err = m.DescTable(context.Background(), "db", "tbl;DROP TABLE other")
	require.ErrorContains(t, err, "describe table")
}
