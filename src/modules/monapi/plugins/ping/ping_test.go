package ping

import (
	"github.com/didi/nightingale/src/modules/monapi/plugins"
	"testing"
)

func TestCollect(t *testing.T) {
	plugins.PluginTest(t, &Rule{
		Urls: []string{"github.com", "n9e.didiyun.com"},
	})
}
