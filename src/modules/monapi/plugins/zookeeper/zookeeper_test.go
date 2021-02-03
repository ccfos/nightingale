package zookeeper

import (
	"github.com/didi/nightingale/src/modules/monapi/plugins"
	"testing"
	"time"
)

func TestCollect(t *testing.T) {
	input := plugins.PluginTest(t, &Rule{
		Servers: []string{"localhost:2181"},
	})

	time.Sleep(time.Second)
	plugins.PluginInputTest(t, input)
}
