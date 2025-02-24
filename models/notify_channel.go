package models

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"html/template"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pkg/str"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/satori/go.uuid"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/sys"
	"gopkg.in/gomail.v2"
)

const AliSortQuery string = "AccessKeyId=%s" +
	"&Action=SendSms" +
	"&Format=JSON" +
	"&OutId=123" +
	"&PhoneNumbers=%s" +
	"&RegionId=cn-hangzhou" +
	"&SignName=%s" +
	"&SignatureMethod=HMAC-SHA1" +
	"&SignatureNonce=%s" +
	"&SignatureVersion=1.0" +
	"&TemplateCode=%s" +
	"&TemplateParam=%s" +
	"&Timestamp=%s" +
	"&Version=2017-05-25"

type EmailContext struct {
	Events []*AlertCurEvent
	Mail   *gomail.Message
}

// NotifyChannelConfig 通知媒介
type NotifyChannelConfig struct {
	ID int64 `json:"id" gorm:"primaryKey"`
	// 基础配置
	Name        string `json:"name"`        // 媒介名称
	Ident       string `json:"ident"`       // 媒介标识
	Description string `json:"description"` // 媒介描述
	Enable      bool   `json:"enable"`      // 是否启用

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
	BatchSend bool           `json:"batch_send"` // 是否批量发送 user_info
}

// UserInfoParam user_info 类型的参数配置
type UserInfoParam struct {
	ContactKey string `json:"contact_key"` // phone, email, dingtalk_robot_token 等
	Batch      bool   `json:"batch"`       // 是否批量发送
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
		channels, err := poster.GetByUrls[[]*NotifyChannelConfig](ctx, "/v1/n9e/notify-channels-v2")
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
	// 设置代理
	var proxyFunc func(*http.Request) (*url.URL, error)
	if nc.HTTPRequestConfig.Proxy != "" {
		proxyURL, err := url.Parse(nc.HTTPRequestConfig.Proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %v", err)
		}
		proxyFunc = http.ProxyURL(proxyURL)
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: nc.HTTPRequestConfig.TLS != nil && nc.HTTPRequestConfig.TLS.SkipVerify,
	}

	transport := &http.Transport{
		Proxy:           proxyFunc,
		TLSClientConfig: tlsConfig,
		DialContext: (&net.Dialer{
			Timeout: time.Duration(nc.HTTPRequestConfig.Timeout) * time.Second,
		}).DialContext,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(nc.HTTPRequestConfig.Timeout) * time.Second,
	}

	return client, nil
}

func (ncc *NotifyChannelConfig) SendFlashDuty(events []*AlertCurEvent, content map[string]string, flashDutyChannelID int64, client *http.Client) error {

	if client == nil {
		return fmt.Errorf("http client not found")
	}

	// MessageTemplate
	fullTpl := make(map[string]interface{})
	fullTpl["tpl"] = content

	// 将 MessageTemplate 与变量配置的信息渲染进 reqBody
	body, err := ncc.parseRequestBody(fullTpl)
	if err != nil {
		logger.Errorf("failed to parse request body: %v, event: %v", err, events)
		return err
	}

	req, err := http.NewRequest(ncc.HTTPRequestConfig.Method, ncc.ParamConfig.FlashDuty.IntegrationUrl, bytes.NewBuffer(body))
	if err != nil {
		logger.Errorf("failed to create request: %v, event: %v", err, events)
		return err
	}

	// 设置 URL 参数
	query := req.URL.Query()
	query.Add("channel_id", strconv.FormatInt(flashDutyChannelID, 10))
	req.URL.RawQuery = query.Encode()

	// 重试机制
	for i := 0; i <= ncc.HTTPRequestConfig.RetryTimes; i++ {
		resp, err := client.Do(req)
		if err != nil {
			if i < ncc.HTTPRequestConfig.RetryTimes {
				time.Sleep(time.Duration(ncc.HTTPRequestConfig.RetryInterval) * time.Second)
				continue
			}
			return fmt.Errorf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// 读取响应
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error reading response:", err)
		}

		// 打印响应
		fmt.Println("Response:", string(body))

		if resp.StatusCode == http.StatusOK {
			return nil
		}

		if i < ncc.HTTPRequestConfig.RetryTimes {
			time.Sleep(time.Duration(ncc.HTTPRequestConfig.RetryInterval) * time.Second)
		}
	}

	return errors.New("failed to send request")
}

func (ncc *NotifyChannelConfig) SendHTTP(events []*AlertCurEvent, content map[string]string, param map[string]string, userInfos []*User, client *http.Client) error {

	if client == nil {
		return fmt.Errorf("http client not found")
	}

	// MessageTemplate
	fullTpl := make(map[string]interface{})
	fullTpl["tpl"] = content
	// 用户信息
	token := make([]string, 0)
	for _, userInfo := range userInfos {
		var t string
		if ncc.ParamConfig.UserInfo.ContactKey == "phone" {
			t = userInfo.Phone

		} else if ncc.ParamConfig.UserInfo.ContactKey == "email" {
			t = userInfo.Email

		} else {
			t, _ = userInfo.ExtractToken(ncc.ParamConfig.UserInfo.ContactKey)
		}

		if t != "" {
			token = append(token, t)
		}
	}
	u := map[string][]string{
		ncc.ParamConfig.UserInfo.ContactKey: token,
	}
	fullTpl["user_info"] = u
	// 自定义参数
	for key, value := range param {
		fullTpl[key] = value
	}

	// 将 MessageTemplate 与变量配置的信息渲染进 reqBody
	body, err := ncc.parseRequestBody(fullTpl)
	if err != nil {
		logger.Errorf("failed to parse request body: %v, event: %v", err, events)
		return err
	}

	// 替换 URL Header Parameters 中的变量
	ncc.replaceVariables(fullTpl, param)

	req, err := http.NewRequest(ncc.HTTPRequestConfig.Method, ncc.HTTPRequestConfig.URL, bytes.NewBuffer(body))
	if err != nil {
		logger.Errorf("failed to create request: %v, event: %v", err, events)
		return err
	}

	// 设置请求头
	for key, value := range ncc.HTTPRequestConfig.Headers {
		req.Header.Set(key, value)
	}

	// 设置 URL 参数
	query := req.URL.Query()
	for key, value := range ncc.HTTPRequestConfig.Request.Parameters {
		query.Add(key, value)
	}

	// 阿里云短信特殊处理
	if ncc.Ident == "ali-sms" {
		query = ncc.getAliQuery(content)
	}

	req.URL.RawQuery = query.Encode()

	// 重试机制
	for i := 0; i <= ncc.HTTPRequestConfig.RetryTimes; i++ {
		resp, err := client.Do(req)
		if err != nil {
			if i < ncc.HTTPRequestConfig.RetryTimes {
				time.Sleep(time.Duration(ncc.HTTPRequestConfig.RetryInterval) * time.Second)
				continue
			}
			return fmt.Errorf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// 读取响应
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error reading response:", err)
		}

		// 打印响应
		fmt.Println("Response:", string(body))

		if resp.StatusCode == http.StatusOK {
			return nil
		}

		if i < ncc.HTTPRequestConfig.RetryTimes {
			time.Sleep(time.Duration(ncc.HTTPRequestConfig.RetryInterval) * time.Second)
		}
	}

	return errors.New("failed to send request")
}

func (ncc *NotifyChannelConfig) getAliQuery(content map[string]string) url.Values {

	query := url.Values{}
	query.Add("Action", "SendSms")
	query.Add("Format", "JSON")
	query.Add("OutId", "123")
	query.Add("Version", "2017-05-25")
	query.Add("RegionId", "cn-hangzhou")
	query.Add("SignatureMethod", "HMAC-SHA1")
	query.Add("SignatureVersion", "1.0")

	Timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	query.Add("Timestamp", Timestamp)

	AccessKeyId := ncc.HTTPRequestConfig.Request.Parameters["access_key_id"]
	query.Add("AccessKeyId", AccessKeyId)

	PhoneNumbers := ncc.HTTPRequestConfig.Request.Parameters["phone_numbers"]
	query.Add("PhoneNumbers", PhoneNumbers)

	SignName := ncc.HTTPRequestConfig.Request.Parameters["sign_name"]
	query.Add("SignName", SignName)

	SignatureNonce := uuid.NewV4().String()
	query.Add("SignatureNonce", SignatureNonce)

	TemplateCode := ncc.HTTPRequestConfig.Request.Parameters["template_code"]
	query.Add("TemplateCode", TemplateCode)

	bodyTpl := make(map[string]interface{})
	bodyTpl["tpl"] = content
	for _, param := range ncc.ParamConfig.Custom.Params {
		bodyTpl[param.Key] = param.CName
	}
	tpl, _ := template.New("template_parma").Funcs(tplx.TemplateFuncMap).Parse(ncc.HTTPRequestConfig.Request.Parameters["template_param"])

	var body bytes.Buffer
	_ = tpl.Execute(&body, bodyTpl)

	TemplateParam := body.String()
	query.Add("TemplateParam", TemplateParam)

	AccessKeySecret := ncc.HTTPRequestConfig.Request.Parameters["access_key_secret"]
	signature := aliSignature(ncc.HTTPRequestConfig.Method, AccessKeyId, AccessKeySecret, PhoneNumbers, SignName, SignatureNonce, TemplateCode, TemplateParam, Timestamp)
	query.Add("Signature", signature)
	return query
}

func aliSignature(method, accessKeyId, accessKeySecret, phoneNumbers, signName, signatureNonce, templateCode, templateParam, timestamp string) string {
	sortQueryString := fmt.Sprintf(AliSortQuery,
		accessKeyId,
		url.QueryEscape(phoneNumbers),
		url.QueryEscape(signName),
		url.QueryEscape(signatureNonce),
		templateCode,
		url.QueryEscape(templateParam),
		url.QueryEscape(timestamp),
	)

	urlencode := encode_local(sortQueryString)
	sign_str := fmt.Sprintf("%s&%%2F&%s", method, urlencode)

	key := []byte(accessKeySecret + "&")
	mac := hmac.New(sha1.New, key)
	mac.Write([]byte(sign_str))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return signature
}

func encode_local(encode_str string) string {
	urlencode := url.QueryEscape(encode_str)
	urlencode = strings.Replace(urlencode, "+", "%%20", -1)
	urlencode = strings.Replace(urlencode, "*", "%2A", -1)
	urlencode = strings.Replace(urlencode, "%%7E", "~", -1)
	urlencode = strings.Replace(urlencode, "/", "%%2F", -1)
	return urlencode
}

func (ncc *NotifyChannelConfig) parseRequestBody(bodyTpl map[string]interface{}) ([]byte, error) {
	tpl, err := template.New("requestBody").Funcs(tplx.TemplateFuncMap).Parse(ncc.HTTPRequestConfig.Request.Body)
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	err = tpl.Execute(&body, bodyTpl)
	return body.Bytes(), err
}

func getParsedString(name, tplStr string, tplData map[string]interface{}) string {
	tpl, err := template.New(name).Funcs(tplx.TemplateFuncMap).Parse(tplStr)
	if err != nil {
		return ""
	}
	var body bytes.Buffer
	err = tpl.Execute(&body, tplData)

	return body.String()
}

func (ncc *NotifyChannelConfig) replaceVariables(tpl map[string]interface{}, param map[string]string) {
	if needsTemplateRendering(ncc.HTTPRequestConfig.URL) {
		ncc.HTTPRequestConfig.URL = getParsedString("url", ncc.HTTPRequestConfig.URL, tpl)
	}

	for key, value := range ncc.HTTPRequestConfig.Headers {
		if needsTemplateRendering(value) {
			ncc.HTTPRequestConfig.Headers[key] = getParsedString(key, value, tpl)
		}
	}

	for key, value := range ncc.HTTPRequestConfig.Request.Parameters {
		if needsTemplateRendering(value) {
			ncc.HTTPRequestConfig.Request.Parameters[key] = getParsedString(key, value, tpl)
		}
	}
}

// needsTemplateRendering 检查字符串是否包含模板语法
func needsTemplateRendering(s string) bool {
	return strings.Contains(s, "{{") && strings.Contains(s, "}}")
}

func (ncc *NotifyChannelConfig) SendEmail(events []*AlertCurEvent, content map[string]string, userInfos []*User, ch chan *EmailContext) error {

	var to []string
	for _, userInfo := range userInfos {
		if userInfo.Email != "" {
			to = append(to, userInfo.Email)
		}
	}
	m := gomail.NewMessage()
	m.SetHeader("From", ncc.SMTPRequestConfig.From)
	m.SetHeader("To", strings.Join(to, ","))
	m.SetHeader("Subject", content["subject"])
	m.SetBody("text/html", content["content"])

	ch <- &EmailContext{events, m}

	return nil
}

func getMessageTpl(events []*AlertCurEvent, content map[string]string) string {
	// 创建一个 map 来存储所有数据
	data := map[string]interface{}{
		"events":  events,
		"content": content,
	}

	// 将数据序列化为 JSON 字节数组
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return ""
	}

	return string(jsonBytes)
}

func (ncc *NotifyChannelConfig) SendScript(events []*AlertCurEvent, content map[string]string, param map[string]string, userInfos []*User) error {

	config := ncc.ScriptRequestConfig
	if config.Script == "" && config.Path == "" {
		return nil
	}

	fpath := ".notify_scriptt"
	if ncc.ScriptRequestConfig.Path != "" {
		fpath = ncc.ScriptRequestConfig.Path
	} else {
		rewrite := true
		if file.IsExist(fpath) {
			oldContent, err := file.ToString(fpath)
			if err != nil {
				return err
			}

			if oldContent == ncc.ScriptRequestConfig.Script {
				rewrite = false
			}
		}

		if rewrite {
			_, err := file.WriteString(fpath, ncc.ScriptRequestConfig.Script)
			if err != nil {
				return err
			}

			err = os.Chmod(fpath, 0777)
			if err != nil {
				return err
			}
		}

		cur, _ := os.Getwd()
		fpath = path.Join(cur, fpath)
	}

	// 用户信息
	token := make([]string, 0)
	for _, userInfo := range userInfos {
		var t string
		if ncc.ParamConfig.UserInfo.ContactKey == "phone" {
			t = userInfo.Phone

		} else if ncc.ParamConfig.UserInfo.ContactKey == "email" {
			t = userInfo.Email

		} else {
			t, _ = userInfo.ExtractToken(ncc.ParamConfig.UserInfo.ContactKey)
		}

		if t != "" {
			token = append(token, t)
		}
	}

	cmd := exec.Command(fpath)
	cmd.Stdin = bytes.NewReader(getStdinBytes(events, content, param, ncc.ParamConfig.UserInfo.ContactKey, token))

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := startCmd(cmd)
	if err != nil {
		return err
	}

	err, isTimeout := sys.WrapTimeout(cmd, time.Duration(config.Timeout)*time.Second)

	if isTimeout {
		if err == nil {
			return errors.New("timeout and killed process")
		}

		return err
	}

	if err != nil {
		return err
	}
	fmt.Printf("event_script_notify_ok: exec %s output: %s", fpath, buf.String())

	return nil
}

func getStdinBytes(events []*AlertCurEvent, content map[string]string, param map[string]string, contactKey string, token []string) []byte {
	// 创建一个 map 来存储所有数据
	data := map[string]interface{}{
		"events":  events,
		"content": content,
		"param":   param,
		"contact": token,
	}

	// 将数据序列化为 JSON 字节数组
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil
	}

	return jsonBytes
}

func startCmd(c *exec.Cmd) error {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c.Start()
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
			if !(str.IsValidURL(c.ParamConfig.FlashDuty.IntegrationUrl) && strings.Contains(
				c.ParamConfig.FlashDuty.IntegrationUrl, "?integration_key=")) {
				return errors.New("flashduty param must have valid integration_url")
			}

			// duty 不校验 http 相关配置
			return nil
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
		}},
}

func InitNotifyChannel(ctx *ctx.Context) {
	if !ctx.IsCenter {
		return
	}

	for channel, notiCh := range NotiChMap {
		notiCh.Enable = true
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
