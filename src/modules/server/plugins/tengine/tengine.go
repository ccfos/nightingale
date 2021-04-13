package tengine

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/v4/src/common/i18n"
	"github.com/didi/nightingale/v4/src/modules/server/collector"
	"github.com/didi/nightingale/v4/src/modules/server/plugins"

	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs/tengine"
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
		"tengine",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &Rule{} },
	)}
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"Urls": "服务",
			"An array of Tengine reqstat module URI to gather stats.": "查看Tengine状态的地址",
			"ResponseTimeout":                                         "响应超时时间",
			"HTTP response timeout (default: 5s)":                     "HTTP响应超时时间(单位: 秒)，默认5秒",
		},
	}
)

type Rule struct {
	Urls            []string `label:"Urls" json:"urls,required" description:"An array of Tengine reqstat module URI to gather stats." example:"http://localhost/us"`
	ResponseTimeout int      `label:"ResponseTimeout" json:"response_timeout" default:"5" description:"HTTP response timeout (default: 5s)"`
	plugins.ClientConfig
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
	input := &tengine.Tengine{
		Urls: p.Urls,
	}

	if err := plugins.SetValue(&input.ResponseTimeout.Duration, time.Second*time.Duration(p.ResponseTimeout)); err != nil {
		return nil, err
	}

	return input, nil
}
