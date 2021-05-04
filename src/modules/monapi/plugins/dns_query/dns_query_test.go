package dns_query

import (
	"github.com/didi/nightingale/src/modules/monapi/plugins"
	"testing"
)

func TestCollect(t *testing.T) {
	plugins.PluginTest(t, &Rule{
                Servers: []string{"223.5.5.5"},
                Domains: []string{"www.baidu.com"},
  })
}
