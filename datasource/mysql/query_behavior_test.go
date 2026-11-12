package mysql

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource"
	dsmysql "github.com/ccfos/nightingale/v6/dskit/mysql"
	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/macros"
	"github.com/glebarez/sqlite"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func queryMySQL(t *testing.T) (*MySQL, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	m := newMySQL(dsmysql.Shard{Addr: "unit-" + t.Name(), User: "reader", MaxQueryRows: 5})
	key := m.Shards[0].Addr + "::reader"
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

func TestInit_DecodesMySQLShardSettings(t *testing.T) {
	settings := map[string]interface{}{
		"mysql.shards": []map[string]interface{}{{"mysql.addr": "db:3306", "mysql.user": "reader", "mysql.timeout": 8}},
	}

	ds, err := (&MySQL{}).Init(settings)

	require.NoError(t, err)
	m := ds.(*MySQL)
	require.Equal(t, []dsmysql.Shard{{Addr: "db:3306", User: "reader", Timeout: 8}}, m.Shards)
}

func TestValidate_RequiresAddressAndUser(t *testing.T) {
	tests := []struct {
		name   string
		shards []dsmysql.Shard
		want   string
	}{
		{name: "no shards", want: "addr is invalid"},
		{name: "blank address", shards: []dsmysql.Shard{{Addr: "  ", User: "reader"}}, want: "addr is invalid"},
		{name: "blank user", shards: []dsmysql.Shard{{Addr: "db:3306", User: " "}}, want: "user is invalid"},
		{name: "valid", shards: []dsmysql.Shard{{Addr: "db:3306", User: "reader"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MySQL{MySQL: dsmysql.MySQL{Shards: tt.shards}}

			err := m.Validate(context.Background())

			if tt.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.want)
			}
		})
	}
}

func TestEqual_DetectsConnectionAndQuerySettingChanges(t *testing.T) {
	base := dsmysql.Shard{Addr: "db:3306", User: "reader"}
	tests := []struct {
		name   string
		change func(*dsmysql.Shard)
	}{
		{name: "address", change: func(s *dsmysql.Shard) { s.Addr = "other:3306" }},
		{name: "user", change: func(s *dsmysql.Shard) { s.User = "writer" }},
		{name: "password", change: func(s *dsmysql.Shard) { s.Password = "secret" }},
		{name: "row limit", change: func(s *dsmysql.Shard) { s.MaxQueryRows = 10 }},
		{name: "timeout", change: func(s *dsmysql.Shard) { s.Timeout = 5 }},
		{name: "idle connections", change: func(s *dsmysql.Shard) { s.MaxIdleConns = 1 }},
		{name: "open connections", change: func(s *dsmysql.Shard) { s.MaxOpenConns = 2 }},
		{name: "connection lifetime", change: func(s *dsmysql.Shard) { s.ConnMaxLifetime = 30 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := base
			tt.change(&changed)

			equal := newMySQL(base).Equal(newMySQL(changed))

			require.False(t, equal)
		})
	}
}

func TestInitClient_RejectsMissingShards(t *testing.T) {
	err := (&MySQL{}).InitClient()

	require.ErrorContains(t, err, "not found mysql addr")
}

func TestInitClient_ReusesCachedConnection(t *testing.T) {
	m, _ := queryMySQL(t)

	err := m.InitClient()

	require.NoError(t, err)
}

func TestQueryData_ExpandsExplicitWindowAndReturnsReferencedSeries(t *testing.T) {
	m, _ := queryMySQL(t)
	previous := macros.Macro
	t.Cleanup(func() { macros.Macro = previous })
	var start, end int64
	var sql, dialect string
	macros.Macro = func(query string, from, to int64, source string) (string, error) {
		sql, start, end, dialect = query, from, to, source
		return "SELECT 100 AS ts, 2.5 AS value, 'api' AS job", nil
	}
	query := QueryParam{
		Ref: "A", SQL: "SELECT $__window", From: 100, To: 200,
		Keys: datasource.Keys{TimeKey: "ts", ValueKey: "value", LabelKey: "job"},
	}

	data, err := m.QueryData(context.Background(), query)

	require.NoError(t, err)
	require.Equal(t, "SELECT $__window", sql)
	require.Equal(t, int64(100), start)
	require.Equal(t, int64(200), end)
	require.Equal(t, MySQLType, dialect)
	require.Equal(t, []models.DataResp{{
		Ref: "A", Metric: model.Metric{"__name__": "value", "job": "api"}, Values: [][]float64{{100, 2.5}},
	}}, data)
}

func TestQueryData_RequiresValueKey(t *testing.T) {
	data, err := (&MySQL{}).QueryData(context.Background(), QueryParam{SQL: "SELECT 1"})

	require.ErrorContains(t, err, "valueKey is required")
	require.Nil(t, data)
}

func TestQueryMapData_ConvertsRowsToStrings(t *testing.T) {
	m, _ := queryMySQL(t)
	query := QueryParam{SQL: "SELECT 'api' AS job, 12 AS count, NULL AS optional", From: 100, To: 200}

	rows, err := m.QueryMapData(context.Background(), query)

	require.NoError(t, err)
	require.Equal(t, []map[string]string{{"job": "api", "count": "12"}}, rows)
}

func TestQueryLog_AppliesOffsetToExplicitMacroWindow(t *testing.T) {
	m, _ := queryMySQL(t)
	previous := macros.Macro
	t.Cleanup(func() { macros.Macro = previous })
	var source string
	macros.Macro = func(_ string, start, end int64, dialect string) (string, error) {
		source = dialect
		return fmt.Sprintf("SELECT %d AS start, %d AS end", start, end), nil
	}

	rows, _, err := m.QueryLog(context.Background(), QueryParam{SQL: "SELECT $__window", From: 100, To: 200, Offset: 10})

	require.NoError(t, err)
	require.Equal(t, MySQLType, source)
	require.Equal(t, []interface{}{map[string]interface{}{"start": int64(90), "end": int64(190)}}, rows)
}

func TestQueryMethods_PropagateMacroFailure(t *testing.T) {
	previous := macros.Macro
	t.Cleanup(func() { macros.Macro = previous })
	want := errors.New("macro expansion failed")
	macros.Macro = func(string, int64, int64, string) (string, error) { return "", want }
	m := &MySQL{}
	query := QueryParam{SQL: "SELECT $__window"}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "data", run: func() error { _, err := m.QueryData(context.Background(), query); return err }},
		{name: "log", run: func() error { _, _, err := m.QueryLog(context.Background(), query); return err }},
		{name: "map", run: func() error { _, err := m.QueryMapData(context.Background(), query); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()

			require.ErrorIs(t, err, want)
		})
	}
}

func TestQueryMethods_RejectUndecodableParameters(t *testing.T) {
	m := &MySQL{}
	bad := map[string]interface{}{"from": []string{"invalid"}}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "data", run: func() error { _, err := m.QueryData(context.Background(), bad); return err }},
		{name: "log", run: func() error { _, _, err := m.QueryLog(context.Background(), bad); return err }},
		{name: "map", run: func() error { _, err := m.QueryMapData(context.Background(), bad); return err }},
		{name: "describe", run: func() error { _, err := m.DescribeTable(context.Background(), bad); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()

			require.ErrorContains(t, err, "from")
		})
	}
}

func TestQueryData_PropagatesRowLimitFailure(t *testing.T) {
	m, _ := queryMySQL(t)
	m.Shards[0].MaxQueryRows = 1
	query := QueryParam{SQL: "SELECT 1 AS value UNION ALL SELECT 2", Keys: datasource.Keys{ValueKey: "value"}}

	data, err := m.QueryData(context.Background(), query)

	require.ErrorContains(t, err, "exceeds the maximum limit 1")
	require.Empty(t, data)
}

func TestQueryLog_PropagatesInvalidSQL(t *testing.T) {
	m, _ := queryMySQL(t)

	rows, total, err := m.QueryLog(context.Background(), QueryParam{SQL: "SELECT * FROM missing_table"})

	require.Error(t, err)
	require.Empty(t, rows)
	require.Zero(t, total)
}
