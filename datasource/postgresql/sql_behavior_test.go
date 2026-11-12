package postgresql

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/ccfos/nightingale/v6/dskit/postgres"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/glebarez/sqlite"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPostgreSQL_ValidateRequiresAddressAndUser(t *testing.T) {
	tests := []struct {
		name   string
		shards []*postgres.PostgreSQL
		want   string
	}{
		{name: "no shards", want: "addr is invalid"},
		{
			name:   "blank address",
			shards: []*postgres.PostgreSQL{{Shard: postgres.Shard{Addr: "  "}}},
			want:   "addr is invalid",
		},
		{
			name:   "blank user",
			shards: []*postgres.PostgreSQL{{Shard: postgres.Shard{Addr: "localhost", User: " "}}},
			want:   "user is invalid",
		},
		{name: "valid", shards: []*postgres.PostgreSQL{{Shard: postgres.Shard{Addr: "localhost", User: "reader"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PostgreSQL{Shards: tt.shards}

			err := p.Validate(context.Background())

			if tt.want != "" {
				require.ErrorContains(t, err, tt.want)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPostgreSQL_EqualDetectsShardConnectionChanges(t *testing.T) {
	tests := []struct {
		name   string
		change func(*postgres.PostgreSQL)
	}{
		{name: "address", change: func(p *postgres.PostgreSQL) { p.Addr = "other" }},
		{name: "user", change: func(p *postgres.PostgreSQL) { p.User = "other" }},
		{name: "password", change: func(p *postgres.PostgreSQL) { p.Password = "other" }},
		{name: "row limit", change: func(p *postgres.PostgreSQL) { p.MaxQueryRows++ }},
		{name: "timeout", change: func(p *postgres.PostgreSQL) { p.Timeout++ }},
		{name: "idle connections", change: func(p *postgres.PostgreSQL) { p.MaxIdleConns++ }},
		{name: "open connections", change: func(p *postgres.PostgreSQL) { p.MaxOpenConns++ }},
		{name: "connection lifetime", change: func(p *postgres.PostgreSQL) { p.ConnMaxLifetime++ }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shard := postgres.PostgreSQL{Shard: postgres.Shard{Addr: "localhost", User: "reader"}}
			changed := shard
			tt.change(&changed)
			p := &PostgreSQL{Shards: []*postgres.PostgreSQL{&shard}}

			require.False(t, p.Equal(&PostgreSQL{Shards: []*postgres.PostgreSQL{&changed}}))
			require.True(t, p.Equal(&PostgreSQL{Shards: []*postgres.PostgreSQL{&shard}}))
		})
	}
	require.False(t, (&PostgreSQL{}).Equal(&PostgreSQL{}))
}

func TestParseDBName_ExtractsQuotedAndBareDatabase(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{name: "bare", sql: "SELECT * FROM telemetry.public.samples", want: "telemetry"},
		{name: "quoted", sql: `select * from "Telemetry"."Public"."Samples"`, want: "Telemetry"},
		{name: "mixed", sql: `select * FROM "Telemetry".public.samples`, want: "Telemetry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDBName(tt.sql)

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseDBName_RejectsMissingDatabaseQualification(t *testing.T) {
	for _, sql := range []string{"select 1", "select * from public.samples", "select * from samples"} {
		t.Run(sql, func(t *testing.T) {
			_, err := parseDBName(sql)

			require.ErrorContains(t, err, "database.schema.table")
		})
	}
}

func TestFormatSQLDatabaseName_QuotesQualifiedIdentifiers(t *testing.T) {
	sql := "SELECT value FROM Telemetry . Public . Samples WHERE value > 1"

	got := formatSQLDatabaseNameWithRegex(sql)

	require.Equal(t, `SELECT value from "Telemetry"."Public"."Samples" WHERE value > 1`, got)
	require.Equal(t, got, formatSQLDatabaseNameWithRegex(got))
}

func TestExtractColumns_PreservesExpressionBoundaries(t *testing.T) {
	sql := "SELECT host, coalesce(value, 0), concat('a,b', host) FROM telemetry.public.samples"

	got, err := extractColumns(sql)

	require.NoError(t, err)
	require.Equal(t, []string{"host", "coalesce(value, 0)", "concat('a,b', host)"}, got)
}

func TestExtractColumns_RejectsMissingSelectList(t *testing.T) {
	_, err := extractColumns("delete from telemetry.public.samples")

	require.ErrorContains(t, err, "no columns found")
}

func TestPostgreSQL_QueryMethodsRejectMalformedParameters(t *testing.T) {
	p := &PostgreSQL{}
	query := map[string]interface{}{"from": []int{1, 2}}

	_, dataErr := p.QueryData(context.Background(), query)
	_, _, logErr := p.QueryLog(context.Background(), query)
	_, mapErr := p.QueryMapData(context.Background(), query)
	_, descErr := p.DescribeTable(context.Background(), query)

	require.Error(t, dataErr)
	require.Error(t, logErr)
	require.Error(t, mapErr)
	require.Error(t, descErr)
}

func TestPostgreSQL_QueryMethodsRejectMissingDatabaseBeforeConnecting(t *testing.T) {
	p := &PostgreSQL{}
	query := map[string]interface{}{"sql": "select * from samples", "from": 10, "to": 20}

	_, dataErr := p.QueryData(context.Background(), query)
	_, _, logErr := p.QueryLog(context.Background(), query)
	_, mapErr := p.QueryMapData(context.Background(), query)

	require.ErrorContains(t, dataErr, "database.schema.table")
	require.ErrorContains(t, logErr, "database.schema.table")
	require.ErrorContains(t, mapErr, "database.schema.table")
	require.ErrorContains(t, p.InitClient(), "not found postgresql addr")
}

func TestPostgreSQL_QueryDataConvertsSQLRowsWithoutLiveDatabase(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.User{}))
	require.NoError(t, db.Create(&models.User{Id: 7, Username: "host-a", CreateAt: 1700000000}).Error)
	shard := &postgres.PostgreSQL{Shard: postgres.Shard{
		Addr: "unit-test-data", User: "reader", DB: "metrics",
	}}
	// Reuse the connection cache to exercise SQL conversion using an in-memory database.
	cacheKey := "unit-test-data::reader:metrics"
	previous, existed := pool.PoolClient.Load(cacheKey)
	pool.PoolClient.Store(cacheKey, db)
	t.Cleanup(func() {
		if existed {
			pool.PoolClient.Store(cacheKey, previous)
		} else {
			pool.PoolClient.Delete(cacheKey)
		}
	})
	client := &PostgreSQL{Shards: []*postgres.PostgreSQL{shard}}

	got, err := client.QueryData(context.Background(), QueryParam{
		Database: "metrics", SQL: "select id, username, create_at from users", Ref: "A",
		Keys: datasource.Keys{ValueKey: "id", LabelKey: "username", TimeKey: "create_at"},
	})

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "A", got[0].Ref)
	require.Equal(t, model.Metric{"__name__": "id", "username": "host-a"}, got[0].Metric)
	require.Equal(t, [][]float64{{1700000000, 7}}, got[0].Values)
}

func TestPostgreSQL_QueryLogRejectsUnsafeDatabaseIdentifier(t *testing.T) {
	client := &PostgreSQL{Shards: []*postgres.PostgreSQL{{Shard: postgres.Shard{Addr: "unused"}}}}

	_, _, err := client.QueryLog(context.Background(), QueryParam{
		Database: "metrics;drop table users", SQL: "select 1", From: 10, To: 20,
	})

	require.ErrorContains(t, err, "postgres connect")
}
