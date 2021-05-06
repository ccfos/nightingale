package net_response

import (
	"testing"

	"github.com/didi/nightingale/v4/src/modules/server/plugins"
)

func TestCollect(t *testing.T) {
	plugins.PluginTest(t, &Rule{
		Address: "github.com:443",
	})
}
