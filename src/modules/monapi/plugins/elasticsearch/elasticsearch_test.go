package elasticsearch

import (
	"github.com/didi/nightingale/src/modules/monapi/plugins"
	"testing"
	"time"
)

func TestCollect(t *testing.T) {
	input := plugins.PluginTest(t, &Rule{
		Servers: []string{"http://localhost:9200"},
	})

	time.Sleep(time.Second)
	plugins.PluginInputTest(t, input)
}
