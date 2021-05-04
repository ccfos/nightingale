package demo

import (
	"github.com/didi/nightingale/v4/src/common/i18n"
	"github.com/didi/nightingale/v4/src/modules/server/collector"
	"github.com/didi/nightingale/v4/src/modules/server/plugins/demo/demo"
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
			"Period": "周期",
			"The period of the function, in seconds": "函数周期，单位 秒",
			"Count":                   "数量",
			"The Count of the series": "指标数量",
		},
	}
)

type DemoRule struct {
	Period int `label:"Period" json:"period,required" description:"The period of the function, in seconds" default:"3600"`
	Count  int `label:"Count" json:"count,required" enum:"[1, 2, 4, 8, 16]" description:"The Count of the series" default:"8"`
}

func (p *DemoRule) Validate() error {
	if p.Period == 0 {
		p.Period = 3600
	}
	if p.Count == 0 {
		p.Period = 8
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
