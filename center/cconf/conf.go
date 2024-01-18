package cconf

import "time"

type Center struct {
	Plugins                []Plugin
	MetricsYamlFile        string
	OpsYamlFile            string
	BuiltinIntegrationsDir string
	I18NHeaderKey          string
	MetricDesc             MetricDescType
	AnonymousAccess        AnonymousAccess
	UseFileAssets          bool
	FlashDuty              FlashDuty
}

type Plugin struct {
	Id       int64  `json:"id"`
	Category string `json:"category"`
	Type     string `json:"plugin_type"`
	TypeName string `json:"plugin_type_name"`
}

type FlashDuty struct {
	Api     string        `json:"api"`
	Timeout time.Duration `json:"timeout"`
}

type AnonymousAccess struct {
	PromQuerier bool
	AlertDetail bool
}

func (c *Center) PreCheck() {
	if len(c.Plugins) == 0 {
		c.Plugins = Plugins
	}
}
