package models

type NotifyRule struct {
	// 基本配置
	Name         string  `json:"name"`           // 名称
	Description  string  `json:"description"`    // 备注
	Enable       bool    `json:"enable"`         // 启用状态
	UserGroupIds []int64 `json:"user_group_ids"` // 告警组ID

	// 事件处理
	// Handlers []Handler `json:"handlers"` // 处理器列表

	// 聚合配置
	// AggregateConfig AggregateConfig `json:"aggregate_config"`

	// 通知配置
	NotifyConfigs []NotifyConfig `json:"notify_configs"`
}

type NotifyConfig struct {
	Channel  string      `json:"channel"`  // 通知媒介(如：阿里云短信)
	Template string      `json:"template"` // 通知模板
	Params   interface{} `json:"params"`   // 通知参数

	Severities []int         `json:"severities"`  // 适用级别(一级告警、二级告警、三级告警)
	TimeRanges []TimeRanges  `json:"time_ranges"` // 适用时段
	LabelKeys  []LabelFilter `json:"label_keys"`  // 适用标签
}

type TimeRanges struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Week  string `json:"week"`
}

type LabelFilter struct {
	Key   string `json:"key"`
	Op    string `json:"op"` // == != in not in =~ !~
	Value string `json:"value"`
}

type AggregateConfig struct {
	CriticalInterval string   `json:"critical_interval"` // Critical级别聚合时间
	WarningInterval  string   `json:"warning_interval"`  // Warning级别聚合时间
	InfoInterval     string   `json:"info_interval"`     // Info级别聚合时间
	LabelKeys        []string `json:"label_keys"`        // 根据标签
}

// 处理器定义
type Handler struct {
	Type   string `json:"type"` // 处理器类型(Relabel/Enrich/Mute/Inhibit)
	Config struct {
		// Relabel处理器配置
		Action string `json:"action,omitempty"` // labelkeep等
		Regex  string `json:"regex,omitempty"`

		// Enrich处理器配置
		Source string `json:"source,omitempty"`
		Target string `json:"target,omitempty"`

		// Mute处理器配置
		Expression string `json:"expression,omitempty"` // 匹配表达式

		// Inhibit处理器配置
		SourceLabel string `json:"source_label,omitempty"`
		TargetLabel string `json:"target_label,omitempty"`
		Equal       string `json:"equal,omitempty"`
	} `json:"config"`
}
