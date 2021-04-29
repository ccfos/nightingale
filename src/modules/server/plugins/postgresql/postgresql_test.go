package postgresql

import (
	"github.com/didi/nightingale/v4/src/modules/server/plugins"
	_ "github.com/lib/pq"
	"testing"
	"time"
)

func TestCollect(t *testing.T) {
	input := plugins.PluginTest(
		t, &PostgresqlRule{
			Dsn:                      "postgres://postgres:xxxx@127.0.0.1:5432/postgres?sslmode=disable",
			ExcludeDatabases:         []string{},
			GatherPgReplicationSlots: true,

			ClientConfig: plugins.ClientConfig{},
		})
	time.Sleep(2 * time.Second)
	plugins.PluginInputTest(t, input)
}
