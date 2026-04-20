package models

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/pkg/errors"
	"gopkg.in/gomail.v2"
)

var VerifyByProvider func(*NotifyChannelConfig) error

type EmailContext struct {
	NotifyRuleId int64
	Events       []*AlertCurEvent
	Mail         *gomail.Message
}

// NotifyChannelConfig 通知媒介
type NotifyChannelConfig struct {
	ID int64 `json:"id" gorm:"primaryKey"`
	// 基础配置
	Name        string `json:"name"`        // 媒介名称
	Ident       string `json:"ident"`       // 媒介类型
	Description string `json:"description"` // 媒介描述
	Enable      bool   `json:"enable"`      // 是否启用

	// 用户参数配置
	ParamConfig *NotifyParamConfig `json:"param_config,omitempty" gorm:"serializer:json"`

	// 通知请求配置
	RequestType   string         `json:"request_type"` // http, stmp, script, flashduty
	RequestConfig *RequestConfig `json:"request_config,omitempty" gorm:"serializer:json"`

	Weight           int    `json:"weight"` // 权重，根据此字段对内置模板进行排序
	CreateAt         int64  `json:"create_at"`
	CreateBy         string `json:"create_by"`
	UpdateAt         int64  `json:"update_at"`
	UpdateBy         string `json:"update_by"`
	UpdateByNickname string `json:"update_by_nickname" gorm:"-"`
}

func (ncc *NotifyChannelConfig) TableName() string {
	return "notify_channel"
}

type RequestConfig struct {
	HTTPRequestConfig        *HTTPRequestConfig        `json:"http_request_config,omitempty" gorm:"serializer:json"`
	SMTPRequestConfig        *SMTPRequestConfig        `json:"smtp_request_config,omitempty" gorm:"serializer:json"`
	ScriptRequestConfig      *ScriptRequestConfig      `json:"script_request_config,omitempty" gorm:"serializer:json"`
	FlashDutyRequestConfig   *FlashDutyRequestConfig   `json:"flashduty_request_config,omitempty" gorm:"serializer:json"`
	PagerDutyRequestConfig   *PagerDutyRequestConfig   `json:"pagerduty_request_config,omitempty" gorm:"serializer:json"`
	DingtalkAppRequestConfig *DingtalkAppRequestConfig `json:"dingtalkapp_request_config,omitempty" gorm:"serializer:json"`
	FeishuAppRequestConfig   *FeishuAppRequestConfig   `json:"feishuapp_request_config,omitempty" gorm:"serializer:json"`
	WecomAppRequestConfig    *WecomAppRequestConfig    `json:"wecomapp_request_config,omitempty" gorm:"serializer:json"`
	// 兼容旧版本
	DingtalkRequestConfig *DingtalkRequestConfig `json:"dingtalk_request_config,omitempty" gorm:"serializer:json"`
	FeishuRequestConfig   *FeishuRequestConfig   `json:"feishu_request_config,omitempty" gorm:"serializer:json"`
	WecomRequestConfig    *WecomRequestConfig    `json:"wecom_request_config,omitempty" gorm:"serializer:json"`
}

// NotifyParamConfig 参数配置
type NotifyParamConfig struct {
	UserInfo *UserInfo `json:"user_info,omitempty"`
	Custom   Params    `json:"custom"` // 自定义参数配置
}

type Params struct {
	Params []ParamItem `json:"params"`
}

type UserInfo struct {
	ContactKey string `json:"contact_key"` // phone, email, dingtalk_robot_token 等
}

// FlashDutyParam flashduty 类型的参数配置
type FlashDutyRequestConfig struct {
	Proxy          string `json:"proxy"`
	IntegrationUrl string `json:"integration_url"`
	Timeout        int    `json:"timeout"`     // 超时时间（毫秒）
	RetryTimes     int    `json:"retry_times"` // 重试次数
	RetrySleep     int    `json:"retry_sleep"` // 重试等待时间（毫秒）
}

// PagerDutyRequestConfig PagerDuty 类型的参数配置
type PagerDutyRequestConfig struct {
	Proxy      string `json:"proxy"`
	ApiKey     string `json:"api_key"`     // PagerDuty 账户或用户的 API Key，不是集成的 Integration Key (routing key)
	Timeout    int    `json:"timeout"`     // 超时时间（毫秒）
	RetryTimes int    `json:"retry_times"` // 重试次数
	RetrySleep int    `json:"retry_sleep"` // 重试等待时间（毫秒）
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
	Timeout    int    `json:"timeout"`     // 超时时间（毫秒）
	Script     string `json:"script"`      // 脚本内容
	Path       string `json:"path"`        // 脚本路径
}

// HTTPRequestConfig 通知请求配置
type HTTPRequestConfig struct {
	URL           string            `json:"url"`
	Method        string            `json:"method"` // GET, POST, PUT
	Headers       map[string]string `json:"headers"`
	Proxy         string            `json:"proxy"`
	Timeout       int               `json:"timeout"`        // 超时时间（毫秒）
	Concurrency   int               `json:"concurrency"`    // 并发数
	RetryTimes    int               `json:"retry_times"`    // 重试次数
	RetryInterval int               `json:"retry_interval"` // 重试间隔（毫秒）
	TLS           *TLSConfig        `json:"tls,omitempty"`
	Request       RequestDetail     `json:"request"`
}

type DingtalkAppRequestConfig struct {
	AppKey     string `json:"app_key"`
	AppSecret  string `json:"app_secret"`
	Proxy      string `json:"proxy"`
	Timeout    int    `json:"timeout"`     // 超时时间（毫秒）
	RetryTimes int    `json:"retry_times"` // 重试次数
	RetrySleep int    `json:"retry_sleep"` // 重试等待时间（毫秒）
}

type FeishuAppRequestConfig struct {
	AppID         string `json:"app_id"`
	AppSecret     string `json:"app_secret"`
	ReceiveIDType string `json:"receive_id_type,omitempty"`
	Proxy         string `json:"proxy"`
	Timeout       int    `json:"timeout"`     // 超时时间（毫秒）
	RetryTimes    int    `json:"retry_times"` // 重试次数
	RetrySleep    int    `json:"retry_sleep"` // 重试等待时间（毫秒）
}

type FeishuRequestConfig struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

type DingtalkRequestConfig struct {
	AppKey    string `json:"app_key"`
	AppSecret string `json:"app_secret"`
}

type WecomRequestConfig struct {
	CorpID     string `json:"corp_id"`
	CorpSecret string `json:"corp_secret"`
	AgentID    int    `json:"agent_id"`
}

type WecomAppRequestConfig struct {
	CorpID     string `json:"corp_id"`
	CorpSecret string `json:"corp_secret"`
	AgentID    int    `json:"agent_id"`
	Proxy      string `json:"proxy"`
	Timeout    int    `json:"timeout"`     // 超时时间（毫秒）
	RetryTimes int    `json:"retry_times"` // 重试次数
	RetrySleep int    `json:"retry_sleep"` // 重试等待时间（毫秒）
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
	Body       string            `json:"body"`       // 请求体
}

func NotifyChannelStatistics(ctx *ctx.Context) (*Statistics, error) {
	if !ctx.IsCenter {
		s, err := poster.GetByUrls[*Statistics](ctx, "/v1/n9e/statistic?name=notify_channel")
		return s, err
	}

	session := DB(ctx).Model(&NotifyChannelConfig{}).Select("count(*) as total", "max(update_at) as last_updated").Where("enable = ?", true)

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func NotifyChannelGetsAll(ctx *ctx.Context) ([]*NotifyChannelConfig, error) {
	if !ctx.IsCenter {
		channels, err := poster.GetByUrls[[]*NotifyChannelConfig](ctx, "/v1/n9e/notify-channels")
		return channels, err
	}

	var channels []*NotifyChannelConfig
	err := DB(ctx).Where("enable = ?", true).Find(&channels).Error
	if err != nil {
		return nil, err
	}

	return channels, nil
}

func NotifyChannelGets(ctx *ctx.Context, id int64, name, ident string, enabled int) ([]*NotifyChannelConfig, error) {
	session := DB(ctx)

	if id != 0 {
		session = session.Where("id = ?", id)
	}

	if name != "" {
		session = session.Where("name = ?", name)
	}

	if ident != "" {
		session = session.Where("ident = ?", ident)
	}

	if enabled != -1 {
		session = session.Where("enable = ?", enabled)
	}

	var channels []*NotifyChannelConfig
	err := session.Find(&channels).Error

	return channels, err
}

func GetHTTPClient(nc *NotifyChannelConfig) (*http.Client, error) {
	if nc.RequestConfig == nil {
		return nil, fmt.Errorf("%+v request config not found", nc)
	}

	httpConfig := nc.RequestConfig.HTTPRequestConfig
	if httpConfig == nil {
		httpConfig = &HTTPRequestConfig{
			Timeout:       10000,
			Concurrency:   5,
			RetryTimes:    3,
			RetryInterval: 100,
		}
	}
	// 设置代理
	var proxyFunc func(*http.Request) (*url.URL, error)
	proxy := httpConfig.Proxy
	// 对于 FlashDuty 类型，优先使用 FlashDuty 配置中的超时时间
	timeout := httpConfig.Timeout
	if nc.RequestType == "flashduty" && nc.RequestConfig.FlashDutyRequestConfig != nil {
		flashDutyTimeout := nc.RequestConfig.FlashDutyRequestConfig.Timeout
		if flashDutyTimeout > 0 {
			timeout = flashDutyTimeout
		}
		if nc.RequestConfig.FlashDutyRequestConfig.Proxy != "" {
			proxy = nc.RequestConfig.FlashDutyRequestConfig.Proxy
		}
	}

	// 对于 PagerDuty 类型，优先使用 PagerDuty 配置中的代理
	if nc.RequestType == "pagerduty" && nc.RequestConfig.PagerDutyRequestConfig != nil && nc.RequestConfig.PagerDutyRequestConfig.Proxy != "" {
		proxy = nc.RequestConfig.PagerDutyRequestConfig.Proxy
	}
	// TODO(dingtalkapp): 钉钉应用本次不上线，DingtalkApp 超时/代理合并分支先注释；上线时恢复。
	// if nc.RequestType == "dingtalkapp" && nc.RequestConfig.DingtalkAppRequestConfig != nil {
	// 	dingtalkAppTimeout := nc.RequestConfig.DingtalkAppRequestConfig.Timeout
	// 	if dingtalkAppTimeout > 0 {
	// 		timeout = dingtalkAppTimeout
	// 	}
	// 	if nc.RequestConfig.DingtalkAppRequestConfig.Proxy != "" {
	// 		proxy = nc.RequestConfig.DingtalkAppRequestConfig.Proxy
	// 	}
	// }
	// 对于 FeishuApp 类型，优先使用 FeishuApp 配置中的超时时间和代理
	if nc.RequestType == "feishuapp" && nc.RequestConfig.FeishuAppRequestConfig != nil {
		feishuAppTimeout := nc.RequestConfig.FeishuAppRequestConfig.Timeout
		if feishuAppTimeout > 0 {
			timeout = feishuAppTimeout
		}
		if nc.RequestConfig.FeishuAppRequestConfig.Proxy != "" {
			proxy = nc.RequestConfig.FeishuAppRequestConfig.Proxy
		}
	}

	// 对于 WecomApp 类型，优先使用 WecomApp 配置中的超时时间和代理
	if nc.RequestType == "wecomapp" && nc.RequestConfig.WecomAppRequestConfig != nil {
		wecomAppTimeout := nc.RequestConfig.WecomAppRequestConfig.Timeout
		if wecomAppTimeout > 0 {
			timeout = wecomAppTimeout
		}
		if nc.RequestConfig.WecomAppRequestConfig.Proxy != "" {
			proxy = nc.RequestConfig.WecomAppRequestConfig.Proxy
		}
	}

	if timeout == 0 {
		timeout = 10000 // HTTP 默认 10 秒
	}

	if httpConfig.Concurrency == 0 {
		httpConfig.Concurrency = 5
	}

	if httpConfig.RetryTimes == 0 {
		httpConfig.RetryTimes = 3
	}
	if httpConfig.RetryInterval == 0 {
		httpConfig.RetryInterval = 100
	}

	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %v", err)
		}
		proxyFunc = http.ProxyURL(proxyURL)
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: httpConfig.TLS != nil && httpConfig.TLS.SkipVerify,
	}

	transport := &http.Transport{
		Proxy:           proxyFunc,
		TLSClientConfig: tlsConfig,
		DialContext: (&net.Dialer{
			Timeout: time.Duration(timeout) * time.Millisecond,
		}).DialContext,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(timeout) * time.Millisecond,
	}

	return client, nil
}

func (ncc *NotifyChannelConfig) Verify() error {
	if ncc.Name == "" {
		return errors.New("channel name cannot be empty")
	}

	if ncc.Ident == "" {
		return errors.New("channel identifier cannot be empty")
	}

	if !regexp.MustCompile("^[a-zA-Z0-9_-]+$").MatchString(ncc.Ident) {
		return fmt.Errorf("channel identifier must be ^[a-zA-Z0-9_-]+$, current: %s", ncc.Ident)
	}

	// TODO(dingtalkapp): 钉钉应用本次不上线，白名单中暂不放行 dingtalkapp；上线时恢复下面两处注释行。
	if ncc.RequestType != "http" &&
		ncc.RequestType != "smtp" &&
		ncc.RequestType != "script" &&
		ncc.RequestType != "flashduty" &&
		ncc.RequestType != "pagerduty" &&
		// ncc.RequestType != "dingtalkapp" &&
		ncc.RequestType != "feishuapp" &&
		ncc.RequestType != "wecomapp" {
		return errors.New("invalid request type, must be one of 'http', 'smtp', 'script', 'flashduty', 'pagerduty', 'feishuapp', 'wecomapp'")
	}

	if ncc.ParamConfig != nil {
		for _, param := range ncc.ParamConfig.Custom.Params {
			if param.Key != "" && param.CName == "" {
				return errors.New("param items must have valid cname")
			}
		}
	}

	// 校验 Request 配置
	if VerifyByProvider != nil {
		return VerifyByProvider(ncc)
	}
	return nil
}

func (ncc *NotifyChannelConfig) ValidateHTTPRequestConfig() error {
	if ncc.RequestConfig.HTTPRequestConfig == nil {
		return errors.New("http request config cannot be nil")
	}
	return ncc.RequestConfig.HTTPRequestConfig.Verify()
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

	return nil
}

func (ncc *NotifyChannelConfig) ValidateSMTPRequestConfig() error {
	if ncc.RequestConfig.SMTPRequestConfig == nil {
		return errors.New("smtp request config cannot be nil")
	}
	return ncc.RequestConfig.SMTPRequestConfig.Verify()
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

func (ncc *NotifyChannelConfig) ValidateScriptRequestConfig() error {
	if ncc.RequestConfig.ScriptRequestConfig == nil {
		return errors.New("script request config cannot be nil")
	}
	if !(ncc.RequestConfig.ScriptRequestConfig.ScriptType == "script" || ncc.RequestConfig.ScriptRequestConfig.ScriptType == "path") {
		return errors.New("script type must be 'script' or 'path'")
	}
	if ncc.RequestConfig.ScriptRequestConfig.Script == "" && ncc.RequestConfig.ScriptRequestConfig.Path == "" {
		return errors.New("either script content or script path must be provided")
	}

	return nil
}

func (ncc *NotifyChannelConfig) ValidateFlashDutyRequestConfig() error {
	if ncc.RequestConfig.FlashDutyRequestConfig == nil {
		return errors.New("flashduty request config cannot be nil")
	}
	return nil
}

func (ncc *NotifyChannelConfig) ValidatePagerDutyRequestConfig() error {
	if ncc.RequestConfig.PagerDutyRequestConfig == nil {
		return errors.New("pagerduty request config cannot be nil")
	}
	return nil
}

func (ncc *NotifyChannelConfig) Update(ctx *ctx.Context, ref NotifyChannelConfig) error {
	ref.ID = ncc.ID
	ref.CreateAt = ncc.CreateAt
	ref.CreateBy = ncc.CreateBy
	ref.UpdateAt = time.Now().Unix()

	err := ref.Verify()
	if err != nil {
		return err
	}
	return DB(ctx).Model(ncc).Select("*").Updates(ref).Error
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
	err := session.Order("weight asc").Find(&lst).Error
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

func (ncc *NotifyChannelConfig) Upsert(ctx *ctx.Context) error {
	ch, err := NotifyChannelGet(ctx, "name = ?", ncc.Name)
	if err != nil {
		return errors.WithMessage(err, "notify channel init failed to get message tpl")
	}

	if ch == nil {
		return Insert(ctx, ncc)
	}

	if ch.UpdateBy != "" && ch.UpdateBy != "system" {
		return nil
	}
	return ch.Update(ctx, *ncc)
}
