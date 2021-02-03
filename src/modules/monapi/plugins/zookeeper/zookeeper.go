package zookeeper

import (
	"fmt"
	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/monapi/plugins"
	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs/zookeeper"
	"time"
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
		"zookeeper",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &Rule{} },
	)}
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"Servers": "服务",
			"An array of address to gather stats about. Specify an ip or hostname <br /> with port. ie localhost:2181, 10.0.0.1:2181, etc.": "服务地址，格式[localhost:2182]，服务需开启四字指令",
			"Timeout": "请求超时时间",
			"Timeout for metric collections from all servers.  Minimum timeout is 1s": "获取监控指标的超时时间（单位: 秒），最小值为1秒",
		},
	}
)

type Rule struct {
	Servers []string `label:"Servers" json:"servers,required" description:"An array of address to gather stats about. Specify an ip or hostname <br /> with port. ie localhost:2181, 10.0.0.1:2181, etc." example:"localhost:2181"`
	Timeout int      `label:"Timeout" json:"http_timeout" default:"5" description:"Timeout for metric collections from all servers.  Minimum timeout is 1s"`
	plugins.ClientConfig
}

func (p *Rule) Validate() error {
	if len(p.Servers) == 0 || p.Servers[0] == "" {
		return fmt.Errorf("zookeeper.rule.Servers must be set")
	}

	return nil
}

func (p *Rule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	input := &zookeeper.Zookeeper{
		Servers: p.Servers,
	}

	if err := plugins.SetValue(&input.Timeout.Duration, time.Second*time.Duration(p.Timeout)); err != nil {
		return nil, err
	}

	return input, nil
}
