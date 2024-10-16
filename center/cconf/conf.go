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
	EventHistoryGroupView  bool
	CleanNotifyRecordDay   int
	MigrateBusiGroupLabel  bool
}

type Plugin struct {
	Id       int64  `json:"id"`
	Category string `json:"category"`
	Type     string `json:"plugin_type"`
	TypeName string `json:"plugin_type_name"`
}

type FlashDuty struct {
	Api     string
	Headers map[string]string
	Timeout time.Duration
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
