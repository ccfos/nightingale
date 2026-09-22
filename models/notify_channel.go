package models

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
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
	JiraRequestConfig        *JiraRequestConfig        `json:"jira_request_config,omitempty" gorm:"serializer:json"`
	DiscordRequestConfig     *DiscordRequestConfig     `json:"discord_request_config,omitempty" gorm:"serializer:json"`
	JSMAlertRequestConfig    *JSMAlertRequestConfig    `json:"jsm_alert_request_config,omitempty" gorm:"serializer:json"`
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

// 原生对接的海外媒介：request_type 与 ident 同名，provider 自己组包、自管重试与成功判定。
// 旧版同名 ident 的 request_type=http 记录不受影响：新 provider 的 Check 要求 request_type
// 与 ident 一致，校验不过时 Registry.Resolve 按 request_type 兜底到 callback。
const (
	RequestTypeJira     = "jira"
	RequestTypeDiscord  = "discord"
	RequestTypeJSMAlert = "jsm_alert"
)

var nativeRequestTypes = map[string]struct{}{
	RequestTypeJira:     {},
	RequestTypeDiscord:  {},
	RequestTypeJSMAlert: {},
}

// IsNativeRequestType 表示该媒介类型是否为原生对接：这类媒介的 provider 用 json.Marshal
// 组包，消息模板必须按纯文本渲染（RenderEventPlain），不能做通用 HTTP 那层 JSON 转义。
func IsNativeRequestType(requestType string) bool {
	_, ok := nativeRequestTypes[requestType]
	return ok
}

// NativeNetworkConfig 原生媒介共用的网络设置，内嵌进各自的 *RequestConfig，JSON 平铺。
type NativeNetworkConfig struct {
	Proxy              string `json:"proxy"`
	Timeout            int    `json:"timeout"`     // 超时时间（毫秒）
	RetryTimes         int    `json:"retry_times"` // 重试次数（不含首次请求）
	RetrySleep         int    `json:"retry_sleep"` // 重试等待时间（毫秒），对方返回 Retry-After 时以对方为准
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
}

const (
	JiraDeploymentCloud      = "cloud"
	JiraDeploymentDataCenter = "datacenter"

	// JiraTokenScoped 带权限范围的令牌（含服务账号的令牌），必须经 api.atlassian.com 网关访问；
	// JiraTokenClassic 普通令牌，直接访问站点地址
	JiraTokenScoped  = "scoped"
	JiraTokenClassic = "classic"

	JiraAuthPAT   = "pat"
	JiraAuthBasic = "basic"
)

// JiraRequestConfig Jira 工单媒介：只放站点和账号，项目、工作类型等「发到哪」的配置在通知规则里
type JiraRequestConfig struct {
	DeploymentType string `json:"deployment_type"` // cloud | datacenter，空按 cloud
	SiteURL        string `json:"site_url"`        // 浏览器里打开 Jira 的地址，不带 /rest/api
	TokenType      string `json:"token_type"`      // cloud：scoped | classic，空按 scoped
	Email          string `json:"email"`           // cloud：令牌所属账号（或服务账号）的邮箱
	APIToken       string `json:"api_token"`       // cloud
	CloudID        string `json:"cloud_id"`        // cloud + scoped：选填，留空按站点地址自动获取
	AuthType       string `json:"auth_type"`       // datacenter：pat | basic
	PersonalToken  string `json:"personal_token"`  // datacenter + pat
	Username       string `json:"username"`        // datacenter + basic
	Password       string `json:"password"`        // datacenter + basic
	NativeNetworkConfig
}

// DiscordRequestConfig Discord 媒介：Webhook 地址在通知规则里填（一个地址对应一个频道），
// 媒介里只有所有规则共用的外观默认值和网络设置，整个配置可以为空。
type DiscordRequestConfig struct {
	Username  string `json:"username"`   // 覆盖 Webhook 显示的名字
	AvatarURL string `json:"avatar_url"` // 覆盖 Webhook 的头像
	Silent    bool   `json:"silent"`     // 静默推送：消息照发，但不触发推送和桌面通知
	NativeNetworkConfig
}

// JSMAlertRequestConfig JSM（Jira Service Management）告警媒介：API 集成的 key 决定告警归哪个团队，
// 所以跟 Discord 的 Webhook 地址一样填在通知规则里；媒介里只有接口地址和网络设置，整个配置可以为空。
type JSMAlertRequestConfig struct {
	APIURL string `json:"api_url"` // 空按 https://api.atlassian.com
	// PriorityMap 夜莺告警级别（"1"/"2"/"3"）对应的 JSM 优先级（P1–P5）。JSM 的优先级全站固定，
	// 这是组织级约定，所以放在媒介里而不是每条规则各配一遍；缺某个级别时按 S1→P1、S2→P2、S3→P3
	PriorityMap map[string]string `json:"priority_map"`
	NativeNetworkConfig
}

// jsmDefaultPriority 与旧版通用 HTTP 媒介的 P{{$event.Severity}} 一致
var jsmDefaultPriority = map[int]string{1: "P1", 2: "P2", 3: "P3"}

// Priority 返回告警级别对应的 JSM 优先级
func (c *JSMAlertRequestConfig) Priority(severity int) string {
	if c != nil {
		if v := strings.ToUpper(strings.TrimSpace(c.PriorityMap[strconv.Itoa(severity)])); v != "" {
			return v
		}
	}
	return jsmDefaultPriority[severity]
}

// NativeNetwork 返回原生媒介的网络设置；非原生媒介或未配置时返回 nil，调用方按默认值处理。
func (rc *RequestConfig) NativeNetwork(requestType string) *NativeNetworkConfig {
	if rc == nil {
		return nil
	}
	switch requestType {
	case RequestTypeJira:
		if rc.JiraRequestConfig != nil {
			return &rc.JiraRequestConfig.NativeNetworkConfig
		}
	case RequestTypeDiscord:
		if rc.DiscordRequestConfig != nil {
			return &rc.DiscordRequestConfig.NativeNetworkConfig
		}
	case RequestTypeJSMAlert:
		if rc.JSMAlertRequestConfig != nil {
			return &rc.JSMAlertRequestConfig.NativeNetworkConfig
		}
	}
	return nil
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

// NotifyChannelIdentsGet 按 id 批量查出 channelID -> ident 映射，供读接口回填展示。
// 不过滤 enable，避免通知规则引用了已停用媒介时展示为空。
func NotifyChannelIdentsGet(ctx *ctx.Context, ids []int64) (map[int64]string, error) {
	ret := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return ret, nil
	}

	var channels []*NotifyChannelConfig
	err := DB(ctx).Select("id", "ident").Where("id in ?", ids).Find(&channels).Error
	if err != nil {
		return nil, err
	}

	for _, c := range channels {
		ret[c.ID] = c.Ident
	}
	return ret, nil
}

func GetHTTPClient(nc *NotifyChannelConfig) (*http.Client, error) {
	rc := nc.RequestConfig
	if rc == nil {
		// 原生媒介的配置可能整体为空（如 Webhook 类媒介什么都不用填，前端会剔掉空壳），按全默认处理
		if !IsNativeRequestType(nc.RequestType) {
			return nil, fmt.Errorf("%+v request config not found", nc)
		}
		rc = &RequestConfig{}
	}

	httpConfig := rc.HTTPRequestConfig
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
	if nc.RequestType == "flashduty" && rc.FlashDutyRequestConfig != nil {
		flashDutyTimeout := rc.FlashDutyRequestConfig.Timeout
		if flashDutyTimeout > 0 {
			timeout = flashDutyTimeout
		}
		if rc.FlashDutyRequestConfig.Proxy != "" {
			proxy = rc.FlashDutyRequestConfig.Proxy
		}
	}

	// 对于 PagerDuty 类型，优先使用 PagerDuty 配置中的代理
	if nc.RequestType == "pagerduty" && rc.PagerDutyRequestConfig != nil && rc.PagerDutyRequestConfig.Proxy != "" {
		proxy = rc.PagerDutyRequestConfig.Proxy
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
	if nc.RequestType == "feishuapp" && rc.FeishuAppRequestConfig != nil {
		feishuAppTimeout := rc.FeishuAppRequestConfig.Timeout
		if feishuAppTimeout > 0 {
			timeout = feishuAppTimeout
		}
		if rc.FeishuAppRequestConfig.Proxy != "" {
			proxy = rc.FeishuAppRequestConfig.Proxy
		}
	}

	// 对于 WecomApp 类型，优先使用 WecomApp 配置中的超时时间和代理
	if nc.RequestType == "wecomapp" && rc.WecomAppRequestConfig != nil {
		wecomAppTimeout := rc.WecomAppRequestConfig.Timeout
		if wecomAppTimeout > 0 {
			timeout = wecomAppTimeout
		}
		if rc.WecomAppRequestConfig.Proxy != "" {
			proxy = rc.WecomAppRequestConfig.Proxy
		}
	}

	insecureSkipVerify := httpConfig.TLS != nil && httpConfig.TLS.SkipVerify
	// 原生媒介（Jira 等）优先使用自己配置里的超时、代理与证书校验开关
	if n := rc.NativeNetwork(nc.RequestType); n != nil {
		if n.Timeout > 0 {
			timeout = n.Timeout
		}
		if n.Proxy != "" {
			proxy = n.Proxy
		}
		insecureSkipVerify = insecureSkipVerify || n.InsecureSkipVerify
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
		InsecureSkipVerify: insecureSkipVerify,
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
		ncc.RequestType != "wecomapp" &&
		!IsNativeRequestType(ncc.RequestType) {
		return errors.New("invalid request type, must be one of 'http', 'smtp', 'script', 'flashduty', 'pagerduty', 'feishuapp', 'wecomapp', 'jira', 'discord', 'jsm_alert'")
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

func (ncc *NotifyChannelConfig) ValidateJiraRequestConfig() error {
	if ncc.RequestConfig == nil || ncc.RequestConfig.JiraRequestConfig == nil {
		return errors.New("jira request config cannot be nil")
	}
	return ncc.RequestConfig.JiraRequestConfig.Verify()
}

// Verify 只做本地格式校验（每次发送前 provider.Check 都会调用，不能发请求）。
// 含 {{ 的值是变量配置引用，发送时才展开，这里跳过格式校验。
func (c *JiraRequestConfig) Verify() error {
	site := strings.TrimSpace(c.SiteURL)
	if site == "" {
		return errors.New("jira site url cannot be empty")
	}
	if !strings.Contains(site, "{{") {
		u, err := url.Parse(site)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("jira site url must be like https://your-domain.atlassian.net")
		}
	}

	switch c.DeploymentType {
	case "", JiraDeploymentCloud:
		if c.TokenType != "" && c.TokenType != JiraTokenScoped && c.TokenType != JiraTokenClassic {
			return fmt.Errorf("jira token type must be %s or %s", JiraTokenScoped, JiraTokenClassic)
		}
		if strings.TrimSpace(c.Email) == "" {
			return errors.New("jira email cannot be empty")
		}
		if strings.TrimSpace(c.APIToken) == "" {
			return errors.New("jira api token cannot be empty")
		}
	default:
		return fmt.Errorf("jira deployment type must be %s", JiraDeploymentCloud)
	}
	return nil
}

// ValidateDiscordRequestConfig Discord 媒介的配置可以为空（全部用默认值），只校验填了的外观字段
func (ncc *NotifyChannelConfig) ValidateDiscordRequestConfig() error {
	if ncc.RequestConfig == nil || ncc.RequestConfig.DiscordRequestConfig == nil {
		return nil
	}
	avatar := strings.TrimSpace(ncc.RequestConfig.DiscordRequestConfig.AvatarURL)
	if avatar != "" && !strings.Contains(avatar, "{{") && !strings.HasPrefix(avatar, "http://") && !strings.HasPrefix(avatar, "https://") {
		return errors.New("discord avatar url must start with http:// or https://")
	}
	return nil
}

// ValidateJSMAlertRequestConfig JSM 告警媒介的配置可以为空（接口地址用默认值），填了地址只校验协议
func (ncc *NotifyChannelConfig) ValidateJSMAlertRequestConfig() error {
	if ncc.RequestConfig == nil || ncc.RequestConfig.JSMAlertRequestConfig == nil {
		return nil
	}
	cfg := ncc.RequestConfig.JSMAlertRequestConfig
	u := strings.TrimSpace(cfg.APIURL)
	if u != "" && !strings.Contains(u, "{{") && !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return errors.New("jsm alert api url must start with http:// or https://")
	}
	for sev, pr := range cfg.PriorityMap {
		if sev != "1" && sev != "2" && sev != "3" {
			return fmt.Errorf("invalid severity %q in jsm alert priority_map, must be 1, 2 or 3", sev)
		}
		switch strings.ToUpper(strings.TrimSpace(pr)) {
		case "", "P1", "P2", "P3", "P4", "P5":
		default:
			return fmt.Errorf("invalid priority %q in jsm alert priority_map, must be P1 to P5", pr)
		}
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

// Weight 用于页面元素排序，weight 越大 排序越靠后
var NotiChMap = []*NotifyChannelConfig{
	{
		Name: "FlashDuty", Ident: "flashduty", RequestType: "flashduty", Weight: 1, Enable: true,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
			FlashDutyRequestConfig: &FlashDutyRequestConfig{
				IntegrationUrl: "flashduty integration url",
				Timeout:        5000,
				RetryTimes:     3,
			},
		},
	},
	{
		Name: "Callback", Ident: "callback", RequestType: "http", Weight: 2, Enable: true,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "{{$params.callback_url}}",
				Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Body: `{{ jsonMarshal $events }}`,
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			Custom: Params{
				Params: []ParamItem{
					{Key: "callback_url", CName: "Callback Url", Type: "string"},
					{Key: "note", CName: "Note", Type: "string"},
				},
			},
		},
	},
	{
		Name: "Email", Ident: Email, RequestType: "smtp", Weight: 2, Enable: true,
		RequestConfig: &RequestConfig{
			SMTPRequestConfig: &SMTPRequestConfig{
				Host:               "smtp.host",
				Port:               25,
				Username:           "your-username",
				Password:           "your-password",
				From:               "your-email",
				InsecureSkipVerify: true,
			},
		},
		ParamConfig: &NotifyParamConfig{
			UserInfo: &UserInfo{
				ContactKey: "email",
			},
		},
	},
	{
		Name: "Dingtalk", Ident: Dingtalk, RequestType: "http", Weight: 3, Enable: true,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL: "https://oapi.dingtalk.com/robot/send", Method: "POST",
				Headers: map[string]string{"Content-Type": "application/json"},
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Parameters: map[string]string{"access_token": "{{$params.access_token}}"},
					Body:       `{"msgtype": "markdown", "markdown": {"title": "{{$tpl.title}}", "text": "{{$tpl.content}}\n{{batchContactsAts $sendtos}}"}, "at": {"atMobiles": {{batchContactsJsonMarshal $sendtos}} }}`,
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			Custom: Params{
				Params: []ParamItem{
					{Key: "access_token", CName: "Access Token", Type: "string"},
					{Key: "bot_name", CName: "Bot Name", Type: "string"},
				},
			},
		},
	},
	{
		Name: "Wecom", Ident: Wecom, RequestType: "http", Weight: 4, Enable: true,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "https://qyapi.weixin.qq.com/cgi-bin/webhook/send",
				Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Parameters: map[string]string{"key": "{{$params.key}}"},
					Body:       `{"msgtype": "markdown", "markdown": {"content": "{{$tpl.content}}"}}`,
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			Custom: Params{
				Params: []ParamItem{
					{Key: "key", CName: "Key", Type: "string"},
					{Key: "bot_name", CName: "Bot Name", Type: "string"},
				},
			},
		},
	},
	{
		Name: "Feishu Card", Ident: FeishuCard, RequestType: "http", Weight: 5, Enable: true,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "https://open.feishu.cn/open-apis/bot/v2/hook/{{$params.access_token}}",
				Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Body: `{"msg_type": "interactive", "card": {"config": {"wide_screen_mode": true}, "header": {"title": {"content": "{{$tpl.title}}", "tag": "plain_text"}, "template": "{{if $event.IsRecovered}}green{{else}}red{{end}}"}, "elements": [{"tag": "markdown", "content": "{{$tpl.content}}"}]}}`,
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			Custom: Params{
				Params: []ParamItem{
					{Key: "access_token", CName: "Access Token", Type: "string"},
					{Key: "bot_name", CName: "Bot Name", Type: "string"},
				},
			},
		},
	},
	{
		// 原生 Discord：媒介里不需要凭证，Webhook 地址在通知规则里填，所以内置一条开箱即用。
		// ParamConfig 里声明规则侧参数，规则页才能按「历史参数」复用填过的地址。
		Name: "Discord", Ident: Discord, RequestType: RequestTypeDiscord, Weight: 6, Enable: true,
		RequestConfig: &RequestConfig{
			DiscordRequestConfig: &DiscordRequestConfig{
				NativeNetworkConfig: NativeNetworkConfig{Timeout: 10000, RetryTimes: 3, RetrySleep: 1000},
			},
		},
		ParamConfig: &NotifyParamConfig{
			Custom: Params{
				Params: DiscordRuleParams,
			},
		},
	},
	{
		// 原生 JSM 告警：API 集成的 key 在通知规则里填，内置一条开箱即用。名称沿用 #3136 之前
		// 内置的「JSM Alert」，老环境里没被用户改过的那条会原地升级（种子按名称 upsert）。
		Name: "JSM Alert", Ident: JSMAlert, RequestType: RequestTypeJSMAlert, Weight: 7, Enable: true,
		RequestConfig: &RequestConfig{
			JSMAlertRequestConfig: &JSMAlertRequestConfig{
				PriorityMap:         map[string]string{"1": "P1", "2": "P2", "3": "P3"},
				NativeNetworkConfig: NativeNetworkConfig{Timeout: 10000, RetryTimes: 3, RetrySleep: 1000},
			},
		},
		ParamConfig: &NotifyParamConfig{
			Custom: Params{
				Params: JSMAlertRuleParams,
			},
		},
	},
}

// JSMAlertRuleParams 是 JSM 告警通知配置在规则里的参数。api_key 与 #3136 之前内置的
// 「JSM Alert」通用 HTTP 媒介同名，老环境里没被改过的那条原地升级后，已有规则照常可用。
var JSMAlertRuleParams = []ParamItem{
	{Key: "api_key", CName: "API Key", Type: "string"},
	{Key: "bot_name", CName: "Name", Type: "string"},
}

// DiscordRuleParams 是 Discord 通知配置在规则里的参数（历史参数复用按这些 key 回显）
var DiscordRuleParams = []ParamItem{
	{Key: "webhook_url", CName: "Webhook URL", Type: "string"},
	{Key: "bot_name", CName: "Name", Type: "string"},
	{Key: "target", CName: "Send to", Type: "string"},
	{Key: "thread_name", CName: "Post title", Type: "string"},
	{Key: "thread_id", CName: "Thread ID", Type: "string"},
	{Key: "mentions", CName: "Mentions", Type: "string"},
}

func InitNotifyChannel(ctx *ctx.Context) {
	if !ctx.IsCenter {
		return
	}

	for _, notiCh := range NotiChMap {
		notiCh.CreateBy = "system"
		notiCh.CreateAt = time.Now().Unix()
		notiCh.UpdateBy = "system"
		notiCh.UpdateAt = time.Now().Unix()
		err := notiCh.Upsert(ctx)
		if err != nil {
			logger.Warningf("notify channel init failed to upsert notify channels %v", err)
		}
	}
}
