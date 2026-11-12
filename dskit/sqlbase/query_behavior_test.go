package sqlbase

import (
	"context"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func queryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := NewDB(context.Background(), sqlite.Open(":memory:"), 1, 1, time.Minute)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, CloseDB(db)) })
	return db
}

func TestNewDB_ConfiguresPoolAndContext(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "query-context")
	db, err := NewDB(ctx, sqlite.Open(":memory:"), 1, 3, time.Minute)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, CloseDB(db)) })

	sqlDB, err := db.DB()

	require.NoError(t, err)
	require.Equal(t, 3, sqlDB.Stats().MaxOpenConnections)
	require.Equal(t, "query-context", db.Statement.Context.Value(contextKey{}))
}

func TestCloseDB_ClosesConnection(t *testing.T) {
	db, err := NewDB(context.Background(), sqlite.Open(":memory:"), 1, 1, 0)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)

	require.NoError(t, CloseDB(db))

	require.Error(t, sqlDB.Ping())
	require.NoError(t, CloseDB(nil))
}

func TestExecQuery_ConvertsRowsAndBindsArguments(t *testing.T) {
	db := queryTestDB(t)

	rows, err := ExecQuery(context.Background(), db,
		"SELECT ? AS name, CAST('payload' AS BLOB) AS body, 7 AS count, NULL AS missing", "alice")

	require.NoError(t, err)
	require.Equal(t, []map[string]interface{}{{
		"name": "alice", "body": "payload", "count": int64(7), "missing": nil,
	}}, rows)
}

func TestShowTables_ReturnsOrderedNames(t *testing.T) {
	db := queryTestDB(t)

	names, err := ShowTables(context.Background(), db, "SELECT ? UNION ALL SELECT ?", "alerts", "metrics")

	require.NoError(t, err)
	require.Equal(t, []string{"alerts", "metrics"}, names)
}

func TestShowDatabases_ReturnsNames(t *testing.T) {
	db := queryTestDB(t)

	names, err := ShowDatabases(context.Background(), db, "SELECT ? UNION ALL SELECT ?", "main", "archive")

	require.NoError(t, err)
	require.Equal(t, []string{"main", "archive"}, names)
}

func TestSQLReaders_ReportQueryErrors(t *testing.T) {
	db := queryTestDB(t)

	_, execErr := ExecQuery(context.Background(), db, "SELECT * FROM missing_table")
	_, tablesErr := ShowTables(context.Background(), db, "SELECT * FROM missing_table")
	_, databasesErr := ShowDatabases(context.Background(), db, "SELECT * FROM missing_table")

	require.ErrorContains(t, execErr, "missing_table")
	require.ErrorContains(t, tablesErr, "missing_table")
	require.ErrorContains(t, databasesErr, "missing_table")
}

func TestQuery_RejectsWritesBeforeExecution(t *testing.T) {
	db := queryTestDB(t)
	ctx := types.WithCallContext(context.Background(), types.CallContext{EnforceReadOnly: true})

	_, strictErr := Query(ctx, db, &QueryParam{Sql: "DELETE\nFROM missing_table"})
	_, normalErr := Query(context.Background(), db, &QueryParam{Sql: "DELETE FROM missing_table"})

	require.Error(t, strictErr)
	require.ErrorContains(t, normalErr, "operation DELETE is forbidden")
}

func TestQueryTimeseries_GroupsAndSortsSQLRows(t *testing.T) {
	db := queryTestDB(t)
	query := &QueryParam{
		Sql:  "SELECT 20 AS time, 'api' AS host, 4 AS value UNION ALL SELECT 10, 'api', 2",
		Keys: types.Keys{ValueKey: "value", LabelKey: "host"},
	}

	series, err := QueryTimeseries(context.Background(), db, query)

	require.NoError(t, err)
	require.Len(t, series, 1)
	require.Equal(t, "api", string(series[0].Metric["host"]))
	require.Equal(t, "value", string(series[0].Metric["__name__"]))
	require.Equal(t, [][]float64{{10, 2}, {20, 4}}, series[0].Values)
}

func TestConvertDBType_ClassifiesSupportedTypes(t *testing.T) {
	cases := []struct {
		name      string
		dialect   string
		dbType    string
		want      string
		indexable bool
	}{
		{name: "integer", dialect: "mysql", dbType: "BIGINT unsigned", want: types.LogExtractValueTypeLong, indexable: true},
		{
			name: "text", dialect: "postgres", dbType: "CHARACTER VARYING(64)",
			want: types.LogExtractValueTypeText, indexable: true,
		},
		{name: "decimal", dialect: "mysql", dbType: "DECIMAL(12,2)", want: types.LogExtractValueTypeFloat, indexable: true},
		{name: "date", dialect: "postgres", dbType: "TIMESTAMPTZ", want: types.LogExtractValueTypeDate},
		{name: "bool", dialect: "mysql", dbType: "BOOLEAN", want: types.LogExtractValueTypeBool},
		{name: "mysql unknown", dialect: "mysql", dbType: "JSON", want: "json"},
		{name: "postgres unknown", dialect: "postgres", dbType: "UUID", want: "uuid"},
		{name: "oracle unknown", dialect: "oracle", dbType: "RAW", want: "raw"},
		{name: "other unknown", dialect: "sqlite", dbType: "BLOB", want: "blob"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, indexable := ConvertDBType(tc.dialect, tc.dbType)

			require.Equal(t, tc.want, got)
			require.Equal(t, tc.indexable, indexable)
		})
	}
}
