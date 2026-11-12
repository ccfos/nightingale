package ck

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource/es"
	kit "github.com/ccfos/nightingale/v6/dskit/clickhouse"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInit_DecodesSettingsIntoIndependentDatasource(t *testing.T) {
	original := &Clickhouse{}

	result, err := original.Init(map[string]interface{}{
		"ck.nodes": []string{"127.0.0.1:8123"}, "ck.user": "reader", "ck.password": "secret",
		"ck.protocol": "http", "ck.timeout": 1200, "ck.max_query_rows": 42,
	})

	require.NoError(t, err)
	got := result.(*Clickhouse)
	require.NotSame(t, original, got)
	require.Equal(t, kit.Clickhouse{
		Nodes: []string{"127.0.0.1:8123"}, User: "reader", Password: "secret",
		Protocol: "http", Timeout: 1200, MaxQueryRows: 42,
	}, got.Clickhouse)
	require.Empty(t, original.Nodes)
}

func TestInit_RejectsInvalidNumericSetting(t *testing.T) {
	_, err := (&Clickhouse{}).Init(map[string]interface{}{"ck.timeout": []string{"invalid"}})

	require.ErrorContains(t, err, "ck.timeout")
}

func TestValidate_RejectsMissingConnectionSettings(t *testing.T) {
	cases := []struct {
		name   string
		config kit.Clickhouse
		want   string
	}{
		{name: "nodes", want: "ck shard is invalid"},
		{name: "user", config: kit.Clickhouse{Nodes: []string{"localhost:8123"}, User: "  "}, want: "user is invalid"},
		{name: "address", config: kit.Clickhouse{Nodes: []string{"  "}, User: "reader"}, want: "addr is invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &Clickhouse{Clickhouse: tc.config}

			err := client.Validate(context.Background())

			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestValidate_FillsDefaultsBeforeCacheComparison(t *testing.T) {
	client := &Clickhouse{Clickhouse: kit.Clickhouse{Nodes: []string{"localhost:8123"}, User: "reader"}}

	err := client.Validate(context.Background())

	require.NoError(t, err)
	require.Equal(t, 10000, client.Timeout)
	require.Equal(t, 500, client.MaxQueryRows)
	require.True(t, client.Equal(&Clickhouse{Clickhouse: kit.Clickhouse{
		Nodes: []string{"localhost:8123"}, User: "reader", Timeout: 10000, MaxQueryRows: 500,
	}}))
}

func TestEqual_DetectsConnectionAndPoolChanges(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Clickhouse)
	}{
		{name: "user", change: func(c *Clickhouse) { c.User = "other" }},
		{name: "node", change: func(c *Clickhouse) { c.Nodes = []string{"other:8123"} }},
		{name: "password", change: func(c *Clickhouse) { c.Password = "other" }},
		{name: "timeout", change: func(c *Clickhouse) { c.Timeout++ }},
		{name: "row limit", change: func(c *Clickhouse) { c.MaxQueryRows++ }},
		{name: "protocol", change: func(c *Clickhouse) { c.Protocol = "native" }},
		{name: "skip verification", change: func(c *Clickhouse) { c.SkipSSLVerify = true }},
		{name: "secure", change: func(c *Clickhouse) { c.SecureConnection = true }},
		{name: "idle pool", change: func(c *Clickhouse) { c.MaxIdleConns++ }},
		{name: "open pool", change: func(c *Clickhouse) { c.MaxOpenConns++ }},
		{name: "lifetime", change: func(c *Clickhouse) { c.ConnMaxLifetime++ }},
		{name: "missing nodes", change: func(c *Clickhouse) { c.Nodes = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := &Clickhouse{Clickhouse: kit.Clickhouse{Nodes: []string{"localhost:8123"}, User: "reader"}}
			candidate := *original
			tc.change(&candidate)

			equal := original.Equal(&candidate)

			require.False(t, equal)
		})
	}
}

func TestEqual_RejectsDifferentDatasourceType(t *testing.T) {
	client := &Clickhouse{Clickhouse: kit.Clickhouse{Nodes: []string{"localhost:8123"}}}

	require.False(t, client.Equal(&es.Elasticsearch{}))
	require.False(t, (&Clickhouse{}).Equal(client))
}

func TestGetLimit_RespectsRowCountAndConfiguredCap(t *testing.T) {
	cases := []struct {
		name              string
		rows, limit, want int
	}{
		{name: "empty", rows: 0, limit: 5, want: 0},
		{name: "default cap", rows: 600, want: 500},
		{name: "negative uses default", rows: 600, limit: -1, want: 500},
		{name: "explicit cap", rows: 8, limit: 3, want: 3},
		{name: "fewer rows", rows: 2, limit: 3, want: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, getLimit(tc.rows, tc.limit))
		})
	}
}

func TestQueryLog_TruncatesRowsReturnedByDatabase(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	client := &Clickhouse{Clickhouse: kit.Clickhouse{Client: db, MaxQueryRows: 10}}

	logs, total, err := client.QueryLog(context.Background(), QueryParam{
		Sql: "SELECT 'first' AS message UNION ALL SELECT 'second'", Limit: 1,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, []interface{}{map[string]interface{}{"message": "first"}}, logs)
}

func TestInitClient_RejectsMissingNodesWithoutConnecting(t *testing.T) {
	err := (&Clickhouse{}).InitClient()

	require.ErrorContains(t, err, "not found ck shard")
}

func TestClose_ReleasesDatabasePool(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	client := &Clickhouse{Clickhouse: kit.Clickhouse{Client: db}}

	err = client.Close()

	require.NoError(t, err)
	require.Error(t, sqlDB.Ping())
}
