package github

import (
	"testing"

	"github.com/didi/nightingale/v4/src/modules/server/plugins"
)

func TestCollect(t *testing.T) {
	plugins.PluginTest(t, &GitHubRule{
		Repositories: []string{"didi/nightingale"},
	})
}
