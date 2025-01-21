package models

// NotifyRule 通知规则结构体
type NotifyChannelConfig struct {
	// 基础配置
	Name        string `json:"name"`        // 媒介名称
	Ident       string `json:"ident"`       // 媒介标识
	Description string `json:"description"` // 媒介描述

	// 用户参数配置
	ParamConfig NotifyParamConfig `json:"param_config"`

	// 通知请求配置
	RequestType         string              `json:"request_type"` // http, stmp, script
	HTTPRequestConfig   HTTPRequestConfig   `json:"http_request_config"`
	SMTPRequestConfig   SMTPRequestConfig   `json:"smtp_request_config"`
	ScriptRequestConfig ScriptRequestConfig `json:"script_request_config"`
}

// NotifyParamConfig 参数配置
type NotifyParamConfig struct {
	ParamType string         `json:"param_type"` // user_info, flashduty, custom
	UserInfo  UserInfoParam  `json:"user_info"`  // user_info 类型的参数配置
	FlashDuty FlashDutyParam `json:"flashduty"`  // flashduty 类型的参数配置
	Custom    CustomParam    `json:"custom"`     // custom 类型的参数配置
}

// UserInfoParam user_info 类型的参数配置
type UserInfoParam struct {
	ContactKey string `json:"contact_key"` // phone, email, dingtalk_robot_token 等
}

// FlashDutyParam flashduty 类型的参数配置
type FlashDutyParam struct {
	ChannelID int64 `json:"channel_id"`
}

// CustomParam custom 类型的参数配置
type CustomParam struct {
	Params []ParamItem `json:"params"`
}

// ParamItem 自定义参数项
type ParamItem struct {
	Key   string `json:"key"`   // 参数键名
	CName string `json:"cname"` // 参数别名
	Type  string `json:"type"`  // 参数类型，目前支持 string
}

type SMTPRequestConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type ScriptRequestConfig struct {
	ScriptType string `json:"script_type"` // 脚本类型，目前支持 python, shell
	Timeout    int    `json:"timeout"`     // 超时时间（秒）
	Script     string `json:"script"`      // 脚本内容
	Path       string `json:"path"`        // 脚本路径
}

// HTTPRequestConfig 通知请求配置
type HTTPRequestConfig struct {
	URL           string            `json:"url"`
	Method        string            `json:"method"` // GET, POST, PUT
	Headers       map[string]string `json:"headers"`
	Proxy         string            `json:"proxy"`
	Timeout       int               `json:"timeout"`        // 超时时间（秒）
	Concurrency   int               `json:"concurrency"`    // 并发数
	RetryTimes    int               `json:"retry_times"`    // 重试次数
	RetryInterval int               `json:"retry_interval"` // 重试间隔（秒）
	TLS           *TLSConfig        `json:"tls,omitempty"`
	Request       RequestDetail     `json:"request"`
}

// TLSConfig TLS 配置
type TLSConfig struct {
	Enable     bool   `json:"enable"`
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
	CAFile     string `json:"ca_file"`
	SkipVerify bool   `json:"skip_verify"`
}

// RequestDetail 请求详情配置
type RequestDetail struct {
	Parameters map[string]string `json:"parameters"` // URL 参数
	Form       string            `json:"form"`       // 来源
	Body       interface{}       `json:"body"`       // 请求体
}
