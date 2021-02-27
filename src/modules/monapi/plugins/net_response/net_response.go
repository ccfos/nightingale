package net_response

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/monapi/plugins"
	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs/net_response"
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
		"net_response",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &Rule{} },
	)}
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"Address":     "地址",
			"Protocol":    "协议",
			"Timeout":     "请求超时",
			"ReadTimeout": "读取超时",
			"Send":        "Send",
			"Expect":      "Expect",
			"readme - https://github.com/influxdata/telegraf/tree/master/plugins/inputs/net_response": "更多说明详细详见 https://github.com/influxdata/telegraf/tree/master/plugins/inputs/net_response",
			"Protocol, must be tcp or udp": "请求协议，必须是 tcp 或 udp",
			"Set timeout":                  "设置超时，单位是秒",
			"Set read timeout (only used if expecting a response)": "设置读取的超时（仅当配置了 expect response 时使用），单位是秒",
			"string sent to the server, udp required":              "发送给服务器的字符串，udp 必须",
			"expected string in answer, udp required":              "期待服务器返回的字符串（部分），udp 必须",
		},
	}
)

type Rule struct {
	Address     string `label:"Address" json:"address,required"  description:"readme - https://github.com/influxdata/telegraf/tree/master/plugins/inputs/net_response" example:"localhost:80"`
	Protocol    string `label:"Protocol"  json:"protocol" description:"Protocol, must be tcp or udp" example:"tcp"`
	Timeout     int    `label:"Timeout" json:"timeout" default:"1" description:"Set timeout"`
	ReadTimeout int    `label:"ReadTimeout" json:"read_timeout" default:"1" description:"Set read timeout (only used if expecting a response)"`
	Send        string `label:"Send" json:"send"  description:"string sent to the server, udp required" example:"hello"`
	Expect      string `label:"Expect" json:"expect"  description:"expected string in answer, udp required" example:"hello"`
}

func (p *Rule) Validate() error {
	if p.Address == "" {
		return fmt.Errorf("net_response.rule.address must be set")
	}
	if p.Protocol == "" {
		p.Protocol = "tcp"
	}
	if !(p.Protocol == "tcp" || p.Protocol == "udp") {
		return fmt.Errorf("net_response.rule.protocol must be tcp or udp")
	}
	if p.Timeout == 0 {
		p.Timeout = 5
	}
	if p.ReadTimeout == 0 {
		p.ReadTimeout = 5
	}

	return nil
}

func (p *Rule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	input := &net_response.NetResponse{
		Address:  p.Address,
		Protocol: p.Protocol,
		Send:     p.Send,
		Expect:   p.Expect,
	}
	if err := plugins.SetValue(&input.Timeout.Duration, time.Second*time.Duration(p.Timeout)); err != nil {
		return nil, err
	}
	if err := plugins.SetValue(&input.ReadTimeout.Duration, time.Second*time.Duration(p.ReadTimeout)); err != nil {
		return nil, err
	}

	return input, nil
}
