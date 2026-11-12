package doris

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource"
	dsdoris "github.com/ccfos/nightingale/v6/dskit/doris"
	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/macros"
	"github.com/glebarez/sqlite"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func queryDoris(t *testing.T) *Doris {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	d := &Doris{Doris: dsdoris.Doris{Addr: "unit-" + t.Name(), User: "reader", MaxQueryRows: 5}}
	key := d.Addr + ":reader:"
	previous, existed := pool.PoolClient.Load(key)
	pool.PoolClient.Store(key, sqlDB)
	t.Cleanup(func() {
		pool.PoolClient.Delete(key)
		if existed {
			pool.PoolClient.Store(key, previous)
		}
		require.NoError(t, sqlDB.Close())
	})
	return d
}

func TestInit_DecodesDorisSettings(t *testing.T) {
	settings := map[string]interface{}{"doris.addr": "fe:9030", "doris.user": "reader", "doris.max_query_rows": 25}

	ds, err := (&Doris{}).Init(settings)

	require.NoError(t, err)
	d := ds.(*Doris)
	require.Equal(t, "fe:9030", d.Addr)
	require.Equal(t, "reader", d.User)
	require.Equal(t, 25, d.MaxQueryRows)
}

func TestValidate_RequiresAddressAndUser(t *testing.T) {
	tests := []struct {
		name string
		addr string
		user string
		want string
	}{
		{name: "missing address", user: "reader", want: "addr is invalid"},
		{name: "blank address", addr: "  ", user: "reader", want: "addr is invalid"},
		{name: "blank user", addr: "fe:9030", user: " ", want: "user is invalid"},
		{name: "valid", addr: "fe:9030", user: "reader"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Doris{Doris: dsdoris.Doris{Addr: tt.addr, User: tt.user}}

			err := d.Validate(context.Background())

			if tt.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.want)
			}
		})
	}
}

func TestEqual_DetectsDorisConfigurationChanges(t *testing.T) {
	base := Doris{Doris: dsdoris.Doris{Addr: "fe:9030", User: "reader"}}
	tests := []struct {
		name   string
		change func(*Doris)
	}{
		{name: "address", change: func(d *Doris) { d.Addr = "other:9030" }},
		{name: "internal address", change: func(d *Doris) { d.InternalAddr = "internal:9030" }},
		{name: "frontend address", change: func(d *Doris) { d.FeAddr = "fe:8030" }},
		{name: "user", change: func(d *Doris) { d.User = "other" }},
		{name: "password", change: func(d *Doris) { d.Password = "secret" }},
		{name: "write enabled", change: func(d *Doris) { d.EnableWrite = true }},
		{name: "write user", change: func(d *Doris) { d.UserWrite = "writer" }},
		{name: "write password", change: func(d *Doris) { d.PasswordWrite = "secret" }},
		{name: "row limit", change: func(d *Doris) { d.MaxQueryRows = 25 }},
		{name: "timeout", change: func(d *Doris) { d.Timeout = 10 }},
		{name: "idle connections", change: func(d *Doris) { d.MaxIdleConns = 2 }},
		{name: "open connections", change: func(d *Doris) { d.MaxOpenConns = 4 }},
		{name: "connection lifetime", change: func(d *Doris) { d.ConnMaxLifetime = 60 }},
		{name: "cluster", change: func(d *Doris) { d.ClusterName = "cluster" }},
	}
	require.True(t, base.Equal(&base))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := base
			tt.change(&changed)

			equal := base.Equal(&changed)

			require.False(t, equal)
		})
	}
}

func TestInitClient_RejectsMissingAddress(t *testing.T) {
	err := (&Doris{}).InitClient()

	require.ErrorContains(t, err, "not found doris addr")
}

func TestInitClient_ReusesCachedConnection(t *testing.T) {
	d := queryDoris(t)

	err := d.InitClient()

	require.NoError(t, err)
}

func TestQueryMapData_ConvertsSQLValuesAndOmitsNulls(t *testing.T) {
	d := queryDoris(t)
	query := QueryParam{SQL: "SELECT 'api' AS job, CAST('raw' AS BLOB) AS bytes, 2.5 AS value, NULL AS optional"}

	rows, err := d.QueryMapData(context.Background(), query)

	require.NoError(t, err)
	require.Equal(t, []map[string]string{{"job": "api", "bytes": "raw", "value": "2.5"}}, rows)
}

func TestQueryMapData_DiscardsRowsContainingOnlyNulls(t *testing.T) {
	d := queryDoris(t)

	rows, err := d.QueryMapData(context.Background(), QueryParam{SQL: "SELECT NULL AS optional"})

	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestQueryData_ExpandsOffsetWindowAndReturnsReferencedSeries(t *testing.T) {
	d := queryDoris(t)
	previous := macros.Macro
	t.Cleanup(func() { macros.Macro = previous })
	var start, end int64
	var sql, dialect string
	macros.Macro = func(query string, from, to int64, source string) (string, error) {
		sql, start, end, dialect = query, from, to, source
		return "SELECT 100 AS ts, 2.5 AS value, 'api' AS job", nil
	}
	query := QueryParam{
		Ref: "A", SQL: "SELECT $__window", From: 100, To: 200, Offset: 10,
		Keys: datasource.Keys{TimeKey: "ts", ValueKey: "value", LabelKey: "job"},
	}

	data, err := d.QueryData(context.Background(), query)

	require.NoError(t, err)
	require.Equal(t, "SELECT $__window", sql)
	require.Equal(t, int64(90), start)
	require.Equal(t, int64(190), end)
	require.Equal(t, DorisType, dialect)
	// The offset shifts the query window; explicit timestamps returned by SQL stay unchanged.
	require.Equal(t, []models.DataResp{{
		Ref: "A", Metric: model.Metric{"__name__": "value", "job": "api"}, Values: [][]float64{{100, 2.5}},
	}}, data)
}

func TestQueryData_RequiresValueKey(t *testing.T) {
	data, err := (&Doris{}).QueryData(context.Background(), QueryParam{SQL: "SELECT 1"})

	require.ErrorContains(t, err, "valueKey is required")
	require.Nil(t, data)
}

func TestQueryLog_AppliesOffsetAndReturnsRowCount(t *testing.T) {
	d := queryDoris(t)
	previous := macros.Macro
	t.Cleanup(func() { macros.Macro = previous })
	var source string
	macros.Macro = func(_ string, start, end int64, dialect string) (string, error) {
		source = dialect
		return fmt.Sprintf("SELECT %d AS start, %d AS end", start, end), nil
	}

	rows, total, err := d.QueryLog(context.Background(), QueryParam{
		SQL: "SELECT $__window", From: 100, To: 200, Offset: 10,
	})

	require.NoError(t, err)
	require.Equal(t, DorisType, source)
	require.Equal(t, []interface{}{map[string]interface{}{"start": int64(90), "end": int64(190)}}, rows)
	require.Equal(t, int64(1), total)
}

func TestQueryMethods_PropagateMacroFailure(t *testing.T) {
	previous := macros.Macro
	t.Cleanup(func() { macros.Macro = previous })
	want := errors.New("macro expansion failed")
	macros.Macro = func(string, int64, int64, string) (string, error) { return "", want }
	d := &Doris{}
	query := QueryParam{SQL: "SELECT $__window", Keys: datasource.Keys{ValueKey: "value"}}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "data", run: func() error { _, err := d.QueryData(context.Background(), query); return err }},
		{name: "log", run: func() error { _, _, err := d.QueryLog(context.Background(), query); return err }},
		{name: "map", run: func() error { _, err := d.QueryMapData(context.Background(), query); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()

			require.ErrorIs(t, err, want)
		})
	}
}

func TestQueryMethods_RejectUndecodableParameters(t *testing.T) {
	d := &Doris{}
	bad := map[string]interface{}{"from": []string{"invalid"}}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "data", run: func() error { _, err := d.QueryData(context.Background(), bad); return err }},
		{name: "log", run: func() error { _, _, err := d.QueryLog(context.Background(), bad); return err }},
		{name: "describe", run: func() error { _, err := d.DescribeTable(context.Background(), bad); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()

			require.ErrorContains(t, err, "from")
		})
	}
}

func TestQueryData_PropagatesRowLimitFailure(t *testing.T) {
	d := queryDoris(t)
	d.MaxQueryRows = 1
	query := QueryParam{SQL: "SELECT 1 AS value UNION ALL SELECT 2", Keys: datasource.Keys{ValueKey: "value"}}

	data, err := d.QueryData(context.Background(), query)

	require.ErrorContains(t, err, "exceeds the maximum limit 1")
	require.Empty(t, data)
}

func TestQueryLog_PropagatesInvalidSQL(t *testing.T) {
	d := queryDoris(t)

	rows, total, err := d.QueryLog(context.Background(), QueryParam{SQL: "SELECT * FROM missing_table"})

	require.Error(t, err)
	require.Empty(t, rows)
	require.Zero(t, total)
}

func TestDescribeTable_RejectsUnsafeIdentifier(t *testing.T) {
	columns, err := (&Doris{}).DescribeTable(context.Background(), QueryParam{Database: "db", Table: "tbl;DROP TABLE x"})

	require.ErrorContains(t, err, "describe table")
	require.Nil(t, columns)
}
