package http_response

import (
	"testing"

	"github.com/didi/nightingale/src/modules/monapi/plugins"
)

func TestCollect(t *testing.T) {
	plugins.PluginTest(t, &Rule{
		URLs:                []string{"https://github.com"},
		ResponseStatusCode:  200,
		ResponseStringMatch: "github",
	})
}
