package models

type NotifyRule struct {
	// 基本配置
	Name         string  `json:"name"`           // 名称
	Description  string  `json:"description"`    // 备注
	Enable       bool    `json:"enable"`         // 启用状态
	UserGroupIds []int64 `json:"user_group_ids"` // 告警组ID

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
