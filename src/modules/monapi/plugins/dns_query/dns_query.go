package dns_query

import (
        "fmt"
        "github.com/didi/nightingale/src/modules/monapi/collector"
        "github.com/didi/nightingale/src/toolkits/i18n"
        "github.com/influxdata/telegraf"
        "github.com/influxdata/telegraf/plugins/inputs/dns_query"
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
                "dns_query",
                collector.RemoteCategory,
                func() collector.TelegrafPlugin { return &Rule{} },
        )}
}

var (
        langDict = map[string]map[string]string{
                "zh": map[string]string{
                        "Servers":                        "DNS地址",
                        "Network":                        "协议",
                        "Domains":                        "域名",
                        "RecordType":                     "记录类型",
                        "Port":                           "端口",
                        "Timeout":                        "超时",
                        "List of DNS":                    "DNS服务器列表",
                        "Protocol, must be tcp or udp":   "请求协议，必须是 tcp 或 udp",
                        "List of Domains":                "解析域名列表",
                        "DNS Record Type":  "DNS记录类型",
                        "Port, default is 53":            "DNS端口号，默认是53",
                        "Set timeout":                    "设置超时，单位是秒",
                },
        }
)

type Rule struct {
        Servers      []string      `label:"Servers" json:"servers,required" description:"List of DNS" example:"223.5.5.5"`
        Network      string        `label:"Network"  json:"network" description:"Protocol, must be tcp or udp" example:"udp"`
        Domains      []string      `label:"Domains" json:"domains,required" description:"List of Domains", example:"www.baidu.com"`
        RecordType   string        `label:"RecordType" json:"record_type" enum:"[\"A\", \"AAAA\", \"CNAME\", \"MX\", \"NS\", \"PTR\", \"TXT\", \"SOA\", \"SPF\", \"SRV\"]" description:"DNS Record Type"`
        Port         int           `label:"Port" json:"port" default:"53" description:"Port"`
        Timeout      int    			 `label:"Timeout" json:"timeout" default:"10" description:"Set timeout"`
}

func (p *Rule) Validate() error {
	if len(p.Servers) == 0 || p.Servers[0] == "" {
					return fmt.Errorf("dns.rule.servers must be set")
	}
	if p.Network == "" {
					p.Network = "udp"
	}
	if !(p.Network == "tcp" || p.Network == "udp") {
		return fmt.Errorf("net_response.rule.Network must be tcp or udp")
	}
	if len(p.Domains) == 0 || p.Domains[0] == "" {
					return fmt.Errorf("dns.rule.domians must be set")
	}
	if p.RecordType == "" {
					p.RecordType = "A"
	}
	if p.Port == 0 {
					p.Port = 53
	}
	if p.Timeout == 0 {
					p.Timeout = 10
	}

	return nil
}


func (p *Rule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
					return nil, err
	}

	return &dns_query.DnsQuery{
					Servers:                                p.Servers,
					Network:                                p.Network,
					Domains:                                p.Domains,
					RecordType:                             p.RecordType,
					Port:                                   p.Port,
					Timeout:                                p.Timeout,
	}, nil
}
