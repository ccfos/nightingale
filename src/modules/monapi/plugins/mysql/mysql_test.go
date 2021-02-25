package mysql

import (
	"os"
	"testing"

	"github.com/didi/nightingale/src/modules/monapi/plugins"
)

func TestCollect(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Error("unable to get DATA_SOURCE_NAME from environment")
	}

	plugins.PluginTest(t, &MysqlRule{
		Servers:           []string{dsn},
		GatherSlaveStatus: true,
	})
}
