package models

import (
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/str"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

// NotifyRule 通知规则结构体
type NotifyChannelConfig struct {
	ID          int64  `json:"id" gorm:"primarykey"`
	Name        string `json:"name"`        // 媒介名称
	Ident       string `json:"ident"`       // 媒介标识
	Description string `json:"description"` // 媒介描述

	// 用户参数配置
	ParamConfig *NotifyParamConfig `json:"param_config,omitempty" gorm:"serializer:json"`

	// 通知请求配置
	RequestType         string               `json:"request_type"` // http, stmp, script
	HTTPRequestConfig   *HTTPRequestConfig   `json:"http_request_config,omitempty" gorm:"serializer:json"`
	SMTPRequestConfig   *SMTPRequestConfig   `json:"smtp_request_config,omitempty" gorm:"serializer:json"`
	ScriptRequestConfig *ScriptRequestConfig `json:"script_request_config,omitempty" gorm:"serializer:json"`

	CreateAt int64  `json:"create_at"`
	CreateBy string `json:"create_by"`
	UpdateAt int64  `json:"update_at"`
	UpdateBy string `json:"update_by"`
}

func (c *NotifyChannelConfig) TableName() string {
	return "notify_channel"
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
	IntegrationUrl string `json:"integration_url"`
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
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	From               string `json:"from"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
	Batch              int    `json:"batch"`
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

func (c *NotifyChannelConfig) Verify() error {
	if c.Name == "" {
		return errors.New("channel name cannot be empty")
	}

	if c.Ident == "" {
		return errors.New("channel identifier cannot be empty")
	}

	if c.RequestType != "http" && c.RequestType != "smtp" && c.RequestType != "script" {
		return errors.New("invalid request type, must be 'http', 'smtp' or 'script'")
	}

	if c.ParamConfig != nil {
		switch c.ParamConfig.ParamType {
		case "user_info":
			if c.ParamConfig.UserInfo.ContactKey == "" {
				return errors.New("user_info param must have a valid contact_key")
			}
		case "flashduty":
			if c.ParamConfig.FlashDuty.IntegrationUrl == "" {
				return errors.New("flashduty param must have valid integration_url")
			}
		case "custom":
			if len(c.ParamConfig.Custom.Params) == 0 {
				return errors.New("custom param must have valid params")
			}
			// 校验每个自定义参数项
			for _, param := range c.ParamConfig.Custom.Params {
				if param.Key == "" || param.CName == "" || param.Type == "" {
					return errors.New("custom param items must have valid key, cname and type")
				}
			}
		default:
			return errors.New("invalid param type, must be 'user_info', 'flashduty' or 'custom'")
		}
	}

	// 校验 Request 配置
	switch c.RequestType {
	case "http":
		if err := c.ValidateHTTPRequestConfig(); err != nil {
			return err
		}
	case "smtp":
		if err := c.ValidateSMTPRequestConfig(); err != nil {
			return err
		}
	case "script":
		if err := c.ValidateScriptRequestConfig(); err != nil {
			return err
		}
	}

	return nil
}

func (c *NotifyChannelConfig) ValidateHTTPRequestConfig() error {
	if c.HTTPRequestConfig == nil {
		return errors.New("http request config cannot be nil")
	}
	return c.HTTPRequestConfig.Verify()
}

func (c *HTTPRequestConfig) Verify() error {
	if c.URL == "" {
		return errors.New("http request URL cannot be empty")
	}
	if c.Method == "" {
		return errors.New("http request method cannot be empty")
	}
	if !(c.Method == "GET" || c.Method == "POST" || c.Method == "PUT") {
		return errors.New("http request method must be GET, POST or PUT")
	}

	if !str.IsValidURL(c.URL) {
		return errors.New("invalid URL format")
	}
	return nil
}

func (c *NotifyChannelConfig) ValidateSMTPRequestConfig() error {
	if c.SMTPRequestConfig == nil {
		return errors.New("smtp request config cannot be nil")
	}
	return c.SMTPRequestConfig.Verify()
}

func (c *SMTPRequestConfig) Verify() error {
	if c.Host == "" {
		return errors.New("smtp host cannot be empty")
	}
	if c.Port <= 0 {
		return errors.New("smtp port must be greater than 0")
	}
	if c.Username == "" {
		return errors.New("smtp username cannot be empty")
	}
	if c.Password == "" {
		return errors.New("smtp password cannot be empty")
	}
	if c.From == "" {
		return errors.New("smtp from address cannot be empty")
	}

	return nil
}

func (c *NotifyChannelConfig) ValidateScriptRequestConfig() error {
	if c.ScriptRequestConfig == nil {
		return errors.New("script request config cannot be nil")
	}
	if !(c.ScriptRequestConfig.ScriptType == "script" || c.ScriptRequestConfig.ScriptType == "path") {
		return errors.New("script type must be 'script' or 'path'")
	}
	if c.ScriptRequestConfig.Script == "" && c.ScriptRequestConfig.Path == "" {
		return errors.New("either script content or script path must be provided")
	}

	return nil
}

func (c *NotifyChannelConfig) Update(ctx *ctx.Context, ref NotifyChannelConfig) error {
	// ref.FE2DB()
	if c.Ident != ref.Ident {
		return errors.New("cannot update ident")
	}

	ref.ID = c.ID
	ref.CreateAt = c.CreateAt
	ref.CreateBy = c.CreateBy
	ref.UpdateAt = time.Now().Unix()

	err := ref.Verify()
	if err != nil {
		return err
	}
	return DB(ctx).Model(c).Select("*").Updates(ref).Error
}

func NotifyChannelGet(ctx *ctx.Context, where string, args ...interface{}) (
	*NotifyChannelConfig, error) {
	lst, err := NotifyChannelsGet(ctx, where, args...)
	if err != nil || len(lst) == 0 {
		return nil, err
	}
	return lst[0], err
}

func NotifyChannelsGet(ctx *ctx.Context, where string, args ...interface{}) (
	[]*NotifyChannelConfig, error) {
	lst := make([]*NotifyChannelConfig, 0)
	session := DB(ctx)
	if where != "" && len(args) > 0 {
		session = session.Where(where, args...)
	}
	err := session.Find(&lst).Error
	if err != nil {
		return nil, err
	}
	return lst, nil
}

type NotiChList []*NotifyChannelConfig

func (c NotiChList) GetIdentSet() map[int64]struct{} {
	idents := make(map[int64]struct{}, len(c))
	for _, tpl := range c {
		idents[tpl.ID] = struct{}{}
	}
	return idents
}

func (c NotiChList) IfUsed(nr *NotifyRule) bool {
	identSet := c.GetIdentSet()
	for _, nc := range nr.NotifyConfigs {
		if _, ok := identSet[nc.ChannelID]; ok {
			return true
		}
	}
	return false
}

var NotiChMap = map[string]*NotifyChannelConfig{
	Dingtalk: &NotifyChannelConfig{
		Name: Dingtalk, Ident: Dingtalk, RequestType: "http",
		HTTPRequestConfig: &HTTPRequestConfig{
			URL: "https://oapi.dingtalk.com/robot/send", Method: "POST",
			Headers: map[string]string{"Content-Type": "application/json"},
			Timeout: 10, Concurrency: 5, RetryTimes: 3, RetryInterval: 5,
			Request: RequestDetail{
				Parameters: map[string]string{"access_token": "your-access-token"},
				Body:       "This is a Dingtalk notification.",
			},
		}},
	Feishu: &NotifyChannelConfig{
		Name: Feishu, Ident: Feishu, RequestType: "http",
		HTTPRequestConfig: &HTTPRequestConfig{
			URL:    "https://open.feishu.cn/open-apis/bot/v2/hook/your-feishu-token",
			Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
			Timeout: 10, Concurrency: 5, RetryTimes: 3, RetryInterval: 5,
			Request: RequestDetail{
				Body: "This is a FeiShu notification.",
			},
		}},
	FeishuCard: &NotifyChannelConfig{
		Name: FeishuCard, Ident: FeishuCard, RequestType: "http",
		HTTPRequestConfig: &HTTPRequestConfig{
			URL:    "https://open.feishu.cn/open-apis/bot/v2/hook/your-feishu-token",
			Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
			Timeout: 10, Concurrency: 5, RetryTimes: 3, RetryInterval: 5,
			Request: RequestDetail{
				Body: "This is a FeiShuCard notification.",
			},
		}},
	Wecom: &NotifyChannelConfig{
		Name: Wecom, Ident: Wecom, RequestType: "http",
		HTTPRequestConfig: &HTTPRequestConfig{
			URL:    "https://qyapi.weixin.qq.com/cgi-bin/webhook/send",
			Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
			Timeout: 10, Concurrency: 5, RetryTimes: 3, RetryInterval: 5,
			Request: RequestDetail{
				Body: "This is a Wecom notification.",
			},
		}},
	Email: &NotifyChannelConfig{
		Name: Email, Ident: Email, RequestType: "smtp",
		SMTPRequestConfig: &SMTPRequestConfig{
			Host:               "smtp.host",
			Port:               25,
			Username:           "your-username",
			Password:           "your-password",
			From:               "your-email",
			InsecureSkipVerify: true,
			Batch:              10,
		}},
}

func InitNotifyChannel(ctx *ctx.Context) {
	for channel, notiCh := range NotiChMap {
		notiCh.CreateBy = "system"
		notiCh.CreateAt = time.Now().Unix()
		notiCh.UpdateBy = "system"
		notiCh.UpdateAt = time.Now().Unix()
		err := notiCh.Upsert(ctx, channel)
		if err != nil {
			logger.Warningf("failed to upsert notify channels %v", err)
		}
	}
}

func (c *NotifyChannelConfig) Upsert(ctx *ctx.Context, ident string) error {
	ch, err := NotifyChannelGet(ctx, "ident = ?", ident)
	if err != nil {
		return errors.WithMessage(err, "failed to get message tpl")
	}
	if ch == nil {
		return Insert(ctx, c)
	}

	if ch.UpdateBy != "" && ch.UpdateBy != "system" {
		return nil
	}
	return ch.Update(ctx, *c)
}
