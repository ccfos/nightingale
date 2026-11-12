package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/stretchr/testify/require"
)

func schemaDepsRound6(t *testing.T, typ string, fake *queryRound3Datasource) *aiagent.ToolDeps {
	t.Helper()
	return &aiagent.ToolDeps{GetSQLDatasource: func(gotType string, id int64) (datasource.Datasource, bool) {
		require.Equal(t, typ, gotType)
		require.Equal(t, int64(7), id)
		return fake, true
	}}
}

func TestListDatabasesTool_QueriesDialectAndReturnsNames(t *testing.T) {
	cases := []struct {
		typ   string
		query map[string]interface{}
	}{
		{typ: "mysql", query: map[string]interface{}{"sql": "SHOW DATABASES"}},
		{typ: "doris", query: map[string]interface{}{"sql": "SHOW DATABASES"}},
		{typ: "clickhouse", query: map[string]interface{}{"sql": "SHOW DATABASES"}},
		{typ: "ck", query: map[string]interface{}{"sql": "SHOW DATABASES"}},
		{
			typ:   "pgsql",
			query: map[string]interface{}{"sql": "SELECT datname FROM pg_database WHERE datistemplate = false"},
		},
		{
			typ:   "postgresql",
			query: map[string]interface{}{"sql": "SELECT datname FROM pg_database WHERE datistemplate = false"},
		},
		{typ: "tdengine", query: map[string]interface{}{"query": "SELECT name FROM information_schema.ins_databases"}},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			fake := &queryRound3Datasource{logs: []interface{}{
				map[string]interface{}{"Database": "metrics"}, map[string]interface{}{"datname": "archive"},
			}}
			deps := schemaDepsRound6(t, tc.typ, fake)

			out, err := listDatabasesTool(context.Background(), deps,
				map[string]interface{}{"datasource_id": "7", "datasource_type": tc.typ}, nil)

			require.NoError(t, err)
			require.Equal(t, tc.query, fake.logParam)
			require.JSONEq(t, `["metrics","archive"]`, out)
		})
	}
}

func TestListTablesTool_QueriesDialectAndReturnsNames(t *testing.T) {
	cases := []struct {
		typ string
		sql string
	}{
		{typ: "mysql", sql: "SHOW TABLES FROM `metrics`"},
		{typ: "doris", sql: "SHOW TABLES FROM `metrics`"},
		{typ: "clickhouse", sql: "SHOW TABLES FROM `metrics`"},
		{typ: "pgsql", sql: "SELECT tablename FROM pg_tables WHERE schemaname = 'public'"},
		{
			typ: "tdengine",
			sql: "SELECT table_name FROM information_schema.ins_tables WHERE db_name='metrics' UNION ALL " +
				"SELECT stable_name AS table_name FROM information_schema.ins_stables WHERE db_name='metrics'",
		},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			fake := &queryRound3Datasource{logs: []interface{}{
				map[string]interface{}{"Tables_in_metrics": "samples"}, map[string]interface{}{"table_name": "hosts"},
			}}
			deps := schemaDepsRound6(t, tc.typ, fake)
			wantQuery := map[string]interface{}{"sql": tc.sql, "database": "metrics"}
			if tc.typ == "tdengine" {
				wantQuery = map[string]interface{}{"query": tc.sql}
			}

			out, err := listTablesTool(context.Background(), deps, map[string]interface{}{
				"datasource_id": "7", "datasource_type": tc.typ, "database": "metrics",
			}, nil)

			require.NoError(t, err)
			require.Equal(t, wantQuery, fake.logParam)
			require.JSONEq(t, `["samples","hosts"]`, out)
		})
	}
}

func TestDescribeTableTool_QueriesDialectAndReturnsColumns(t *testing.T) {
	cases := []struct {
		typ string
		sql string
	}{
		{typ: "mysql", sql: "DESCRIBE `metrics`.`samples`"},
		{typ: "doris", sql: "DESCRIBE `metrics`.`samples`"},
		{typ: "clickhouse", sql: "DESCRIBE TABLE `metrics`.`samples`"},
		{
			typ: "pgsql",
			sql: `SELECT column_name as "Field", data_type as "Type", is_nullable as "Null", ` +
				`column_default as "Default" FROM information_schema.columns ` +
				`WHERE table_schema = 'public' AND table_name = 'samples'`,
		},
		{
			typ: "tdengine",
			sql: "SELECT col_name AS `Field`, col_type AS `Type` FROM information_schema.ins_columns " +
				"WHERE db_name='metrics' AND table_name='samples' UNION ALL " +
				"SELECT tag_name AS `Field`, tag_type AS `Type` FROM information_schema.ins_tags " +
				"WHERE db_name='metrics' AND stable_name='samples'",
		},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			fake := &queryRound3Datasource{logs: []interface{}{
				map[string]interface{}{"Field": "ts", "Type": "timestamp", "Comment": "sample time"},
				map[string]interface{}{"column_name": "value", "data_type": "double"},
			}}
			deps := schemaDepsRound6(t, tc.typ, fake)
			wantQuery := map[string]interface{}{"sql": tc.sql, "database": "metrics"}
			if tc.typ == "tdengine" {
				wantQuery = map[string]interface{}{"query": tc.sql}
			}

			out, err := describeTableTool(context.Background(), deps, map[string]interface{}{
				"datasource_id": "7", "datasource_type": tc.typ, "database": "metrics", "table": "samples",
			}, nil)

			require.NoError(t, err)
			require.Equal(t, wantQuery, fake.logParam)
			var columns []columnInfo
			require.NoError(t, json.Unmarshal([]byte(out), &columns))
			require.Equal(t, []columnInfo{
				{Name: "ts", Type: "timestamp", Comment: "sample time"}, {Name: "value", Type: "double"},
			}, columns)
		})
	}
}

func TestSchemaTools_PropagateQueryFailure(t *testing.T) {
	cases := []struct {
		name    string
		handler aiagent.BuiltinToolFunc
		want    string
	}{
		{name: "databases", handler: listDatabasesTool, want: "failed to list databases"},
		{name: "tables", handler: listTablesTool, want: "failed to list tables"},
		{name: "columns", handler: describeTableTool, want: "failed to describe table"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &queryRound3Datasource{err: errors.New("permission denied")}
			deps := schemaDepsRound6(t, "mysql", fake)

			out, err := tc.handler(context.Background(), deps, map[string]interface{}{
				"datasource_id": "7", "datasource_type": "mysql", "database": "metrics", "table": "samples",
			}, nil)

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want+": permission denied")
		})
	}
}

func TestSchemaTools_RejectUnsafeIdentifiersBeforeQuerying(t *testing.T) {
	cases := []struct {
		name     string
		handler  aiagent.BuiltinToolFunc
		database string
		table    string
		want     string
	}{
		{name: "table list", handler: listTablesTool, database: "db'; DROP TABLE x", want: "invalid database name"},
		{
			name: "column database", handler: describeTableTool, database: "db`", table: "samples",
			want: "invalid database name",
		},
		{
			name: "column table", handler: describeTableTool, database: "metrics", table: "x; DROP TABLE y",
			want: "invalid table name",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &queryRound3Datasource{}
			deps := schemaDepsRound6(t, "mysql", fake)

			out, err := tc.handler(context.Background(), deps, map[string]interface{}{
				"datasource_id": "7", "datasource_type": "mysql", "database": tc.database, "table": tc.table,
			}, nil)

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
			require.Nil(t, fake.logParam)
		})
	}
}

func TestExtractColumnValues_PrefersKnownKeysAndSkipsInvalidRows(t *testing.T) {
	rows := []interface{}{
		"not a row",
		map[string]interface{}{"Database": "primary", "name": "fallback"},
		map[string]interface{}{"database": 123, "datname": "postgres"},
		map[string]interface{}{"name": ""},
		map[string]interface{}{"unrelated": "ignored"},
	}

	names := extractColumnValues(rows, "database")

	require.Equal(t, []string{"primary", "postgres"}, names)
}

func TestConvertToColumnInfo_UsesAliasesAndSkipsNamelessRows(t *testing.T) {
	rows := []interface{}{
		nil,
		map[string]interface{}{"Field": 123, "field": "value", "type": "double", "column_comment": "load"},
		map[string]interface{}{"name": "host", "data_type": "varchar", "comment": "server"},
		map[string]interface{}{"Type": "int"},
	}

	columns := convertToColumnInfo(rows)

	require.Equal(t, []columnInfo{
		{Name: "value", Type: "double", Comment: "load"}, {Name: "host", Type: "varchar", Comment: "server"},
	}, columns)
}
