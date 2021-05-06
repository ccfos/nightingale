package elasticsearch

import (
	"testing"
	"time"

	"github.com/didi/nightingale/v4/src/modules/server/plugins"
)

func TestCollect(t *testing.T) {
	input := plugins.PluginTest(t, &Rule{
		Servers: []string{"http://localhost:9200"},
	})

	time.Sleep(time.Second)
	plugins.PluginInputTest(t, input)
}
