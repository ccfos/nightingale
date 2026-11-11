package postgresql

import (
	"testing"

	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/ccfos/nightingale/v6/dskit/postgres"
	"gorm.io/gorm"
)

func TestInitDecodesConfiguredDatabase(t *testing.T) {
	p := new(PostgreSQL)
	datasource, err := p.Init(map[string]interface{}{
		"pgsql.shards": []map[string]interface{}{
			{"pgsql.db": "customdb"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := datasource.(*PostgreSQL)
	if len(got.Shards) != 1 {
		t.Fatalf("expected one shard, got %d", len(got.Shards))
	}
	if got.Shards[0].DB != "customdb" {
		t.Fatalf("expected database %q, got %q", "customdb", got.Shards[0].DB)
	}
}

func TestInitClientUsesConfiguredDatabase(t *testing.T) {
	shard := &postgres.PostgreSQL{
		Shard: postgres.Shard{
			Addr:     "127.0.0.1:1",
			DB:       "customdb",
			User:     "configured-db-user",
			Password: "configured-db-password",
		},
	}
	cacheKey := "127.0.0.1:1:configured-db-password:configured-db-user:customdb"
	pool.PoolClient.Store(cacheKey, &gorm.DB{})
	t.Cleanup(func() { pool.PoolClient.Delete(cacheKey) })

	if err := (&PostgreSQL{Shards: []*postgres.PostgreSQL{shard}}).InitClient(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInitClientFallsBackToPostgres(t *testing.T) {
	shard := &postgres.PostgreSQL{
		Shard: postgres.Shard{
			Addr:     "127.0.0.1:1",
			User:     "fallback-db-user",
			Password: "fallback-db-password",
		},
	}
	cacheKey := "127.0.0.1:1:fallback-db-password:fallback-db-user:postgres"
	pool.PoolClient.Store(cacheKey, &gorm.DB{})
	t.Cleanup(func() { pool.PoolClient.Delete(cacheKey) })

	if err := (&PostgreSQL{Shards: []*postgres.PostgreSQL{shard}}).InitClient(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
