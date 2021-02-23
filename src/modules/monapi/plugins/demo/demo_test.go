package demo

import (
	"testing"

	"github.com/didi/nightingale/src/modules/monapi/plugins"
)

func TestCollect(t *testing.T) {
	plugins.PluginTest(t, &DemoRule{
		Period: 3600,
		Count:  10,
	})
}
