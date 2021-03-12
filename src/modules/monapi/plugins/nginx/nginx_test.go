package nginx

import (
	"github.com/didi/nightingale/src/modules/monapi/plugins"
	"testing"
	"time"
)

func TestCollect(t *testing.T) {
	input := plugins.PluginTest(t, &Rule{
		Urls: []string{"http://localhost/nginx-status"},
	})

	time.Sleep(time.Second)
	plugins.PluginInputTest(t, input)
}
