package github

import (
	"testing"

	"github.com/didi/nightingale/src/modules/monapi/plugins"
)

func TestCollect(t *testing.T) {
	plugins.PluginTest(t, &GitHubRule{
		Repositories: []string{"didi/nightingale"},
	})
}
