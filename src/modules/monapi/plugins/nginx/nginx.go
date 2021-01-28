package nginx

import (
	"fmt"
	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs/nginx"
)

func init() {
	collector.CollectorRegister(NewCollector()) // for monapi
	i18n.DictRegister(langDict)
}

type Collector struct {
	*collector.BaseCollector
}

func NewCollector() *Collector {
	return &Collector{BaseCollector: collector.NewBaseCollector(
		"nginx",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &Rule{} },
	)}
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"nginx status uri": "查看Nginx状态的地址",
		},
	}
)

type Rule struct {
	Urls []string `label:"nginx status uri" json:"url,required" example:"http://localhost/status"`
}

func (p *Rule) Validate() error {
	if len(p.Urls) == 0 || p.Urls[0] == "" {
		return fmt.Errorf("ningx.rule.urls must be set")
	}

	return nil
}

func (p *Rule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &nginx.Nginx{
		Urls: p.Urls,
	}, nil
}
