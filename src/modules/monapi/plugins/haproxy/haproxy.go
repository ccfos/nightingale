package haproxy

import (
	"fmt"
	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/monapi/plugins"
	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/influxdata/telegraf"
	"github.com/didi/nightingale/src/modules/monapi/plugins/haproxy/haproxy"
)

func init() {
	collector.CollectorRegister(NewHaproxyCollector()) // for monapi
	i18n.DictRegister(langDict)
}

type HaproxyCollector struct {
	*collector.BaseCollector
}

func NewHaproxyCollector() *HaproxyCollector {
	return &HaproxyCollector{BaseCollector: collector.NewBaseCollector(
		"haproxy",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &HaproxyRule{} },
	)}
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"Servers":          "Servers",
			"Username":         "用户名",
			"Password":         "密码",
		},
	}
)

type HaproxyRule struct {
	Servers               []string `label:"Servers" json:"servers,required" example:"http://myhaproxy.com:1936/haproxy?stats"`
	KeepFieldNames        bool     `label:"KeepFieldNames" json:"keepFieldNames" default:"false" description:"Setting this option to true results in the plugin keeping the original"`
	Username              string   `label:"Username" json:"username" description:"specify username"`
	Password              string   `label:"Password" json:"password" format:"password" description:"specify server password"`

	plugins.ClientConfig
}

func (p *HaproxyRule) Validate() error {
	if len(p.Servers) == 0 || p.Servers[0] == "" {
		return fmt.Errorf("haproxy.rule.servers must be set")
	}
	return nil
}

func (p *HaproxyRule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	ha := &haproxy.Haproxy{

		Servers:               p.Servers,
        KeepFieldNames:		   p.KeepFieldNames,
		Username:              p.Username,
		Password:              p.Password,
		ClientConfig:          p.ClientConfig.TlsClientConfig(),
	}

	return ha, nil
}

