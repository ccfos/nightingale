package tengine

import (
	"github.com/didi/nightingale/src/modules/monapi/plugins"
	"testing"
	"time"
)

func TestCollect(t *testing.T) {
	input := plugins.PluginTest(t, &Rule{
		Urls: []string{"http://localhost/us"},
	})

	time.Sleep(time.Second)
	plugins.PluginInputTest(t, input)
}
