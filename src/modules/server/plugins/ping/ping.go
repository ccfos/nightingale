package ping

import (
	"fmt"

	"github.com/didi/nightingale/v4/src/common/i18n"
	"github.com/didi/nightingale/v4/src/modules/server/collector"
	"github.com/didi/nightingale/v4/src/modules/server/plugins"
	"github.com/didi/nightingale/v4/src/modules/server/plugins/ping/ping"

	"github.com/influxdata/telegraf"
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
		"ping",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &Rule{} },
	)}
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"URLs":         "地址",
			"PingInterval": "Ping间隔",
			"Count":        "次数",
			"Timeout":      "单Ping超时",
			"Deadline":     "总体超时",
			"Method":       "模式",
			"Interface":    "Interface",
			"Binary":       "Binary",
			"IPv6":         "IPv6",
			"URLs to ping": "要 Ping 的目标地址，IP或域名",
			"Interval at which to ping (ping -i <INTERVAL>), default 1":                       "Ping 包的间隔(ping -i <INTERVAL>)，默认是 1 秒",
			"Number of pings to send (ping -c <COUNT>), default 4":                            "Ping 包的次数(ping -c <COUNT>)，默认是 4 次",
			"Per-ping timeout, in seconds. (ping -W <TIMEOUT>), default 1.0":                  "每个 Ping 的超时时间(ping -W <TIMEOUT>)，默认是 1秒",
			"Ping deadline, in seconds. (ping -w <DEADLINE>), default 10":                     "整个 Ping 周期的超时时间(ping -w <DEADLINE>)，默认是 10 秒",
			"Method defines how to ping (native or exec), default native":                     "ping的模式，命令行（exec）或原生（natvie), 默认是 native",
			"Interface or source address to send ping from (ping -I/-S <INTERFACE/SRC_ADDR>)": "Ping 发起的源接口（或源地址），(ping -I/-S <INTERFACE/SRC_ADDR>)",
			"Ping executable binary, default ping":                                            "exec 模式时，ping 的命令。默认是 ping",
			"Use only IPv6 addresses when resolving a hostname":                               "仅将目标域名解析为 IPv6 地址",
		},
	}
)

type Rule struct {
	Urls         []string `label:"Urls" json:"urls,required" description:"URLs to ping" example:"github.com"`
	PingInterval int      `label:"PingInterval" json:"ping_interval" default:"1" description:"Interval at which to ping (ping -i <INTERVAL>), default 1"`
	Count        int      `label:"Count" json:"count" default:"4" description:"Number of pings to send (ping -c <COUNT>), default 4"`
	Timeout      int      `label:"Timeout" json:"timeout" default:"1" description:"Per-ping timeout, in seconds. (ping -W <TIMEOUT>), default 1.0"`
	Deadline     int      `label:"Deadline" json:"deadline" default:"10" description:"Ping deadline, in seconds. (ping -w <DEADLINE>), default 10"`
	Method       string   `label:"Method" json:"method" description:"Method defines how to ping (native or exec), default native" example:"native"`
	Interface    string   `label:"Interface" json:"interface" description:"Interface or source address to send ping from (ping -I/-S <INTERFACE/SRC_ADDR>)" example:"eth0"`
	Binary       string   `label:"Binary" json:"binary" description:"Ping executable binary, default ping" example:"ping"`
	IPv6         bool     `label:"Ipv6" json:"ipv6" description:"Use only IPv6 addresses when resolving a hostname."`
}

func (p *Rule) Validate() error {
	if len(p.Urls) == 0 || p.Urls[0] == "" {
		return fmt.Errorf("ping.rule.urls must be set")
	}
	if p.PingInterval == 0 {
		p.PingInterval = 1
	}
	if p.Count == 0 {
		p.Count = 1
	}
	if p.Timeout == 0 {
		p.Timeout = 1
	}
	if p.Deadline == 0 {
		p.Deadline = 10
	}
	if p.Method == "" {
		p.Method = "native"
	}
	if !(p.Method == "exec" || p.Method == "native") {
		return fmt.Errorf("ping.rule.method must be exec or native")
	}
	if p.Binary == "" {
		p.Binary = "ping"
	}

	return nil
}

func (p *Rule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	input := &ping.Ping{
		PingHost:     ping.MyHostPinger,
		PingInterval: float64(p.PingInterval),
		Count:        p.Count,
		Timeout:      float64(p.Timeout),
		Deadline:     p.Deadline,
		Method:       p.Method,
		Interface:    p.Interface,
		Urls:         p.Urls,
		Binary:       p.Binary,
		IPv6:         p.IPv6,
		Log:          plugins.GetLogger(),
		Arguments:    []string{},
		Percentiles:  []int{50, 95, 99},
	}
	input.NativePingFunc = input.NativePing
	if err := input.Init(); err != nil {
		return nil, err
	}
	return input, nil
}
