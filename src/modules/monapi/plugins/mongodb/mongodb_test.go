package mongodb

import (
	"testing"
	"time"

	"github.com/didi/nightingale/src/modules/monapi/plugins"
)

func TestCollect(t *testing.T) {
	input := plugins.PluginTest(t, &MongodbRule{
		Servers:             []string{"mongodb://root:root@127.0.0.1:27017"},
		GatherClusterStatus: true,
		GatherPerdbStats:    true,
		GatherColStats:      true,
	})

	time.Sleep(time.Second)
	plugins.PluginInputTest(t, input)
}
