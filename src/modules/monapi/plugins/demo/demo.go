package demo

import (
	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/monapi/plugins/demo/demo"
	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/influxdata/telegraf"
)

func init() {
	collector.CollectorRegister(NewDemoCollector()) // for monapi
	i18n.DictRegister(langDict)
}

type DemoCollector struct {
	*collector.BaseCollector
}

func NewDemoCollector() *DemoCollector {
	return &DemoCollector{BaseCollector: collector.NewBaseCollector(
		"demo",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &DemoRule{} },
	)}
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"Servers":   "服务",
			"Databases": "数据库",
		},
	}
)

type DemoRule struct {
	Period int `label:"Period" json:"period,required" description:"The period of the function, in seconds" default:"3600"`
	Count  int `label:"Count" json:"count,required" description:"The Count of the series" default:"10"`
}

func (p *DemoRule) Validate() error {
	if p.Period == 0 {
		p.Period = 3600
	}
	if p.Count == 0 {
		p.Period = 10
	}
	return nil
}

func (p *DemoRule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	return &demo.Demo{
		Period: p.Period,
		Count:  p.Count,
	}, nil
}
