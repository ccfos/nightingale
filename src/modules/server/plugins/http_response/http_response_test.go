package http_response

import (
	"testing"

	"github.com/didi/nightingale/v4/src/modules/server/plugins"
)

func TestCollect(t *testing.T) {
	plugins.PluginTest(t, &Rule{
		URLs:                []string{"https://github.com"},
		ResponseStatusCode:  200,
		ResponseStringMatch: "github",
	})
}
