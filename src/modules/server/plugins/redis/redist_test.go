package redis

import (
	"os"
	"testing"

	"github.com/didi/nightingale/v4/src/modules/server/plugins"
)

func TestCollect(t *testing.T) {
	dsn := os.Getenv("REDIS_SERVER")
	pwd := os.Getenv("REDIS_PWD")
	if dsn == "" {
		t.Error("unable to get REDIS_SERVER from environment")
	}

	plugins.PluginTest(t, &RedisRule{
		Servers:  []string{dsn},
		Password: pwd,
	})
}
