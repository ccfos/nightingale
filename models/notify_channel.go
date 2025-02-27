package models

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pkg/str"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/sys"
	"gopkg.in/gomail.v2"
)

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
	RequestType   string         `json:"request_type"` // http, stmp, script, flashduty
	RequestConfig *RequestConfig `json:"request_config,omitempty" gorm:"serializer:json"`

	CreateAt int64  `json:"create_at"`
	CreateBy string `json:"create_by"`
	UpdateAt int64  `json:"update_at"`
	UpdateBy string `json:"update_by"`
}

func (ncc *NotifyChannelConfig) TableName() string {
	return "notify_channel"
}

type RequestConfig struct {
	HTTPRequestConfig      *HTTPRequestConfig      `json:"http_request_config,omitempty" gorm:"serializer:json"`
	SMTPRequestConfig      *SMTPRequestConfig      `json:"smtp_request_config,omitempty" gorm:"serializer:json"`
	ScriptRequestConfig    *ScriptRequestConfig    `json:"script_request_config,omitempty" gorm:"serializer:json"`
	FlashDutyRequestConfig *FlashDutyRequestConfig `json:"flashduty_request_config,omitempty" gorm:"serializer:json"`
}

// NotifyParamConfig 参数配置
type NotifyParamConfig struct {
	UserContactKey string      `json:"user_contact_key"` // phone, email, dingtalk_robot_token 等
	Params         []ParamItem `json:"params"`           // params 类型的参数配置
}

// FlashDutyParam flashduty 类型的参数配置
type FlashDutyRequestConfig struct {
	Proxy          string `json:"proxy"`
	IntegrationUrl string `json:"integration_url"`
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
	Body       string            `json:"body"`       // 请求体
}

func (ncc *NotifyChannelConfig) SendScript(events []*AlertCurEvent, tpl map[string]string, params map[string]string, userInfos []*User) (string, string, error) {

	config := ncc.RequestConfig.ScriptRequestConfig
	if config.Script == "" && config.Path == "" {
		return "", "", nil
	}

	fpath := ".notify_scriptt"
	if config.Path != "" {
		fpath = config.Path
	} else {
		rewrite := true
		if file.IsExist(fpath) {
			oldContent, err := file.ToString(fpath)
			if err != nil {
				return "", "", nil
			}

			if oldContent == config.Script {
				rewrite = false
			}
		}

		if rewrite {
			_, err := file.WriteString(fpath, config.Script)
			if err != nil {
				return "", "", nil
			}

			err = os.Chmod(fpath, 0777)
			if err != nil {
				return "", "", err
			}
		}

		cur, _ := os.Getwd()
		fpath = path.Join(cur, fpath)
	}

	// 用户信息
	sendtos := make([]string, 0)
	for _, userInfo := range userInfos {
		var sendto string
		if ncc.ParamConfig.UserContactKey == "phone" {
			sendto = userInfo.Phone
		} else if ncc.ParamConfig.UserContactKey == "email" {
			sendto = userInfo.Email
		} else {
			sendto, _ = userInfo.ExtractToken(ncc.ParamConfig.UserContactKey)
		}
		if sendto != "" {
			sendtos = append(sendtos, sendto)
		}
	}

	cmd := exec.Command(fpath)
	cmd.Stdin = bytes.NewReader(getStdinBytes(events, tpl, params, sendtos))

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := startCmd(cmd)
	if err != nil {
		return "", "", err
	}

	res := buf.String()

	// 截断超出长度的输出
	if len(res) > 512 {
		// 确保在有效的UTF-8字符边界处截断
		validLen := 0
		for i := 0; i < 512 && i < len(res); {
			_, size := utf8.DecodeRuneInString(res[i:])
			if i+size > 512 {
				break
			}
			i += size
			validLen = i
		}
		res = res[:validLen] + "..."
	}

	err, isTimeout := sys.WrapTimeout(cmd, time.Duration(config.Timeout)*time.Second)

	if isTimeout {
		if err == nil {
			return cmd.String(), res, errors.New("timeout and killed process")
		}

		return cmd.String(), res, err
	}

	if err != nil {
		return cmd.String(), res, err
	}
	fmt.Printf("event_script_notify_ok: exec %s output: %s", fpath, buf.String())

	return cmd.String(), res, nil
}

func getStdinBytes(events []*AlertCurEvent, tpl map[string]string, params map[string]string, sendtos []string) []byte {
	// 创建一个 map 来存储所有数据
	data := map[string]interface{}{
		"events": events,
		"tpl":    tpl,
		"params": params,
		"sendto": sendtos,
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
	httpConfig := nc.RequestConfig.HTTPRequestConfig

	// 设置代理
	var proxyFunc func(*http.Request) (*url.URL, error)
	if httpConfig.Proxy != "" {
		proxyURL, err := url.Parse(httpConfig.Proxy)
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
			Timeout: time.Duration(httpConfig.Timeout) * time.Second,
		}).DialContext,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(httpConfig.Timeout) * time.Second,
	}

	return client, nil
}

func (ncc *NotifyChannelConfig) SendFlashDuty(events []*AlertCurEvent, flashDutyChannelID int64, client *http.Client) (string, error) {
	// todo 每一个 channel 批量发送事件
	if client == nil {
		return "", fmt.Errorf("http client not found")
	}

	body, err := json.Marshal(events)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", ncc.RequestConfig.FlashDutyRequestConfig.IntegrationUrl, bytes.NewBuffer(body))
	if err != nil {
		logger.Errorf("failed to create request: %v, event: %v", err, events)
		return "", err
	}

	// 设置 URL 参数
	query := req.URL.Query()
	query.Add("channel_id", strconv.FormatInt(flashDutyChannelID, 10))
	req.URL.RawQuery = query.Encode()

	// 重试机制
	for i := 0; i <= 3; i++ {
		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(time.Duration(100) * time.Millisecond)
			continue
		}
		defer resp.Body.Close()

		// 读取响应
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Errorf("failed to read response: %v, event: %v", err, events)
		}

		if resp.StatusCode == http.StatusOK {
			return string(body), nil
		}
		time.Sleep(time.Duration(100) * time.Millisecond)
	}

	return "", errors.New("failed to send request")
}

func (ncc *NotifyChannelConfig) SendHTTP(events []*AlertCurEvent, tpl map[string]string, params map[string]string, userInfo *User, client *http.Client) (string, error) {
	if client == nil {
		return "", fmt.Errorf("http client not found")
	}

	httpConfig := ncc.RequestConfig.HTTPRequestConfig

	// MessageTemplate
	fullTpl := make(map[string]interface{})

	// 用户信息
	var sendto string
	if userInfo != nil {
		if ncc.ParamConfig.UserContactKey == "phone" {
			sendto = userInfo.Phone
		} else if ncc.ParamConfig.UserContactKey == "email" {
			sendto = userInfo.Email
		} else {
			sendto, _ = userInfo.ExtractToken(ncc.ParamConfig.UserContactKey)
		}
	}

	fullTpl["sendto"] = sendto // 发送对象
	fullTpl["params"] = params // 自定义参数
	fullTpl["tpl"] = tpl
	fullTpl["events"] = events

	// 将 MessageTemplate 与变量配置的信息渲染进 reqBody
	body, err := ncc.parseRequestBody(fullTpl)
	if err != nil {
		logger.Errorf("failed to parse request body: %v, event: %v", err, events)
		return "", err
	}

	// 替换 URL Header Parameters 中的变量
	ncc.replaceVariables(fullTpl)

	req, err := http.NewRequest(httpConfig.Method, httpConfig.URL, bytes.NewBuffer(body))
	if err != nil {
		logger.Errorf("failed to create request: %v, event: %v", err, events)
		return "", err
	}

	// 设置请求头 腾讯云短信、语音特殊处理
	if ncc.Ident == "tx-sms" || ncc.Ident == "tx-voice" {
		ncc.setTxHeader(req, body)
	} else {
		for key, value := range httpConfig.Headers {
			req.Header.Set(key, value)
		}
	}

	// 设置 URL 参数 阿里云短信、语音特殊处理
	query := req.URL.Query()
	if ncc.Ident == "ali-sms" || ncc.Ident == "ali-voice" {
		query = ncc.getAliQuery(tpl)
	} else {
		for key, value := range httpConfig.Request.Parameters {
			query.Add(key, value)
		}
	}

	req.URL.RawQuery = query.Encode()

	// 重试机制
	for i := 0; i <= httpConfig.RetryTimes; i++ {
		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(time.Duration(httpConfig.RetryInterval) * time.Second)
			continue
		}
		defer resp.Body.Close()

		// 读取响应
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error reading response:", err)
		}

		if resp.StatusCode == http.StatusOK {
			return string(body), nil
		}

		time.Sleep(time.Duration(httpConfig.RetryInterval) * time.Second)
	}

	return "", errors.New("failed to send request")
}

func (ncc *NotifyChannelConfig) getAliQuery(tplContent map[string]string) url.Values {
	// 渲染 param
	var paramKey string
	if ncc.Ident == "ali-sms" {
		paramKey = "TemplateParam"
	} else {
		paramKey = "TtsParam"
	}

	bodyTpl := make(map[string]interface{})
	bodyTpl["tpl"] = tplContent
	for _, param := range ncc.ParamConfig.Params {
		bodyTpl[param.Key] = param.CName
	}

	httpConfig := ncc.RequestConfig.HTTPRequestConfig

	tpl, _ := template.New(paramKey).Funcs(tplx.TemplateFuncMap).Parse(httpConfig.Request.Parameters[paramKey])
	var body bytes.Buffer
	_ = tpl.Execute(&body, bodyTpl)
	param := body.String()

	query := url.Values{}
	for k, v := range httpConfig.Request.Parameters {
		if k == "AccessKeySecret" || k == paramKey {
			continue
		}
		query.Add(k, v)
	}

	query.Add(paramKey, param)

	Timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	query.Add("Timestamp", Timestamp)

	SignatureNonce := uuid.NewV4().String()
	query.Add("SignatureNonce", SignatureNonce)

	signature := aliSignature(httpConfig.Method, httpConfig.Request.Parameters["AccessKeySecret"], query)
	query.Add("Signature", signature)
	return query
}

func (ncc *NotifyChannelConfig) setTxHeader(req *http.Request, payloadBytes []byte) {
	httpConfig := ncc.RequestConfig.HTTPRequestConfig
	timestamp := time.Now().Unix()

	authorization := ncc.getTxSignature(string(payloadBytes), timestamp)

	for key, value := range httpConfig.Headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("X-TC-Timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("Authorization", authorization)
}

func (ncc *NotifyChannelConfig) getTxSignature(payloadStr string, timestamp int64) string {
	httpConfig := ncc.RequestConfig.HTTPRequestConfig

	canonicalHeaders := fmt.Sprintf("content-type:application/json\nhost:%s\nx-tc-action:%s\n",
		httpConfig.Headers["Host"], strings.ToLower(httpConfig.Headers["X-TC-Action"]))

	hasher := sha256.New()
	hasher.Write([]byte(payloadStr))
	hashedRequestPayload := hex.EncodeToString(hasher.Sum(nil))
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		httpConfig.Method,
		"/",
		"",
		canonicalHeaders,
		"content-type;host;x-tc-action",
		hashedRequestPayload)

	// 1. 生成日期
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	// 2. 拼接待签名字符串
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, httpConfig.Headers["Service"])
	hasher = sha256.New()
	hasher.Write([]byte(canonicalRequest))
	hashedCanonicalRequest := hex.EncodeToString(hasher.Sum(nil))
	stringToSign := fmt.Sprintf("TC3-HMAC-SHA256\n%d\n%s\n%s",
		timestamp,
		credentialScope,
		hashedCanonicalRequest)
	// 3. 计算签名
	secretDate := sign([]byte("TC3"+httpConfig.Headers["Secret_Key"]), date)
	secretService := sign(secretDate, httpConfig.Headers["Service"])
	secretSigning := sign(secretService, "tc3_request")
	signature := hex.EncodeToString(sign(secretSigning, stringToSign))
	// 4. 组织Authorization
	authorization := fmt.Sprintf("TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		httpConfig.Headers["Secret_ID"], credentialScope, "content-type;host;x-tc-action", signature)
	return authorization
}

func sign(key []byte, msg string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(msg))
	return h.Sum(nil)
}

func aliSignature(method, accessSecret string, query url.Values) string {
	queryStrs := make([]string, 0, len(query))
	for k, v := range query {
		queryStrs = append(queryStrs, fmt.Sprintf("%s=%s", url.QueryEscape(k), url.QueryEscape(v[0])))
	}

	sort.Strings(queryStrs)

	urlencode := encode_local(strings.Join(queryStrs, "&"))
	sign_str := fmt.Sprintf("%s&%%2F&%s", method, urlencode)

	key := []byte(accessSecret + "&")
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
	var defs = []string{
		"{{$tpl := .tpl}}",
		"{{$sendto := .sendto}}",
		"{{$params := .params}}",
		"{{$events := .events}}",
	}

	text := strings.Join(append(defs, ncc.RequestConfig.HTTPRequestConfig.Request.Body), "")
	tpl, err := template.New("requestBody").Funcs(tplx.TemplateFuncMap).Parse(text)
	if err != nil {
		return nil, err
	}

	var body bytes.Buffer
	err = tpl.Execute(&body, bodyTpl)
	return body.Bytes(), err
}

func getParsedString(name, tplStr string, tplData map[string]interface{}) string {
	var defs = []string{
		"{{$tpl := .tpl}}",
		"{{$sendto := .sendto}}",
		"{{$params := .params}}",
		"{{$events := .events}}",
	}

	text := strings.Join(append(defs, tplStr), "")
	tpl, err := template.New(name).Funcs(tplx.TemplateFuncMap).Parse(text)
	if err != nil {
		return ""
	}
	var body bytes.Buffer
	err = tpl.Execute(&body, tplData)
	if err != nil {
		return fmt.Sprintf("failed to parse template: %v data: %v", err, tplData)
	}

	return body.String()
}

func (ncc *NotifyChannelConfig) replaceVariables(tpl map[string]interface{}) {
	httpConfig := ncc.RequestConfig.HTTPRequestConfig

	if needsTemplateRendering(httpConfig.URL) {
		httpConfig.URL = getParsedString("url", httpConfig.URL, tpl)
	}

	for key, value := range httpConfig.Headers {
		if needsTemplateRendering(value) {
			httpConfig.Headers[key] = getParsedString(key, value, tpl)
		}
	}

	for key, value := range httpConfig.Request.Parameters {
		if needsTemplateRendering(value) {
			httpConfig.Request.Parameters[key] = getParsedString(key, value, tpl)
		}
	}
}

// needsTemplateRendering 检查字符串是否包含模板语法
func needsTemplateRendering(s string) bool {
	return strings.Contains(s, "{{") && strings.Contains(s, "}}")
}

func (ncc *NotifyChannelConfig) SendEmail(events []*AlertCurEvent, tpl map[string]string, userInfos []*User, ch chan *EmailContext) error {

	var to []string
	for _, userInfo := range userInfos {
		if userInfo.Email != "" {
			to = append(to, userInfo.Email)
		}
	}
	m := gomail.NewMessage()
	m.SetHeader("From", ncc.RequestConfig.SMTPRequestConfig.From)
	m.SetHeader("To", strings.Join(to, ","))
	m.SetHeader("Subject", tpl["subject"])
	m.SetBody("text/html", tpl["content"])

	ch <- &EmailContext{events, m}

	return nil
}

func (ncc *NotifyChannelConfig) SendEmail2(events []*AlertCurEvent, tpl map[string]string, userInfos []*User) error {

	d := gomail.NewDialer(ncc.RequestConfig.SMTPRequestConfig.Host, ncc.RequestConfig.SMTPRequestConfig.Port, ncc.RequestConfig.SMTPRequestConfig.Username, ncc.RequestConfig.SMTPRequestConfig.Password)
	if ncc.RequestConfig.SMTPRequestConfig.InsecureSkipVerify {
		d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}
	s, err := d.Dial()
	if err != nil {
		logger.Errorf("email_sender: failed to dial: %s", err)
	}

	var to []string
	for _, userInfo := range userInfos {
		if userInfo.Email != "" {
			to = append(to, userInfo.Email)
		}
	}
	m := gomail.NewMessage()
	m.SetHeader("From", ncc.RequestConfig.SMTPRequestConfig.From)
	m.SetHeader("To", strings.Join(to, ","))
	m.SetHeader("Subject", tpl["subject"])
	m.SetBody("text/html", tpl["content"])
	return gomail.Send(s, m)
}

func (ncc *NotifyChannelConfig) Verify() error {
	if ncc.Name == "" {
		return errors.New("channel name cannot be empty")
	}

	if ncc.Ident == "" {
		return errors.New("channel identifier cannot be empty")
	}

	if ncc.RequestType != "http" && ncc.RequestType != "smtp" && ncc.RequestType != "script" && ncc.RequestType != "flashduty" {
		return errors.New("invalid request type, must be 'http', 'smtp' or 'script'")
	}

	if ncc.ParamConfig != nil {
		for _, param := range ncc.ParamConfig.Params {
			if param.Key != "" && param.CName == "" {
				return errors.New("param items must have valid cname")
			}
		}
	}

	// 校验 Request 配置
	switch ncc.RequestType {
	case "http":
		if err := ncc.ValidateHTTPRequestConfig(); err != nil {
			return err
		}
	case "smtp":
		if err := ncc.ValidateSMTPRequestConfig(); err != nil {
			return err
		}
	case "script":
		if err := ncc.ValidateScriptRequestConfig(); err != nil {
			return err
		}
	case "flashduty":
		if err := ncc.ValidateFlashDutyRequestConfig(); err != nil {
			return err
		}
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

	if !str.IsValidURL(c.URL) {
		return errors.New("invalid URL format")
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

func (ncc *NotifyChannelConfig) Update(ctx *ctx.Context, ref NotifyChannelConfig) error {
	// ref.FE2DB()
	if ncc.Ident != ref.Ident {
		return errors.New("cannot update ident")
	}

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
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL: "https://oapi.dingtalk.com/robot/send", Method: "POST",
				Headers: map[string]string{"Content-Type": "application/json"},
				Timeout: 10, Concurrency: 5, RetryTimes: 3, RetryInterval: 5,
				Request: RequestDetail{
					Parameters: map[string]string{"access_token": "your-access-token"},
					Body:       "This is a Dingtalk notification.",
				},
			},
		},
	},
	Feishu: &NotifyChannelConfig{
		Name: Feishu, Ident: Feishu, RequestType: "http",
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "https://open.feishu.cn/open-apis/bot/v2/hook/your-feishu-token",
				Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
				Timeout: 10, Concurrency: 5, RetryTimes: 3, RetryInterval: 5,
				Request: RequestDetail{
					Body: "This is a FeiShu notification.",
				},
			},
		},
	},
	FeishuCard: &NotifyChannelConfig{
		Name: FeishuCard, Ident: FeishuCard, RequestType: "http",
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "https://open.feishu.cn/open-apis/bot/v2/hook/your-feishu-token",
				Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
				Timeout: 10, Concurrency: 5, RetryTimes: 3, RetryInterval: 5,
				Request: RequestDetail{
					Body: "This is a FeiShuCard notification.",
				},
			},
		},
	},
	Wecom: &NotifyChannelConfig{
		Name: Wecom, Ident: Wecom, RequestType: "http",
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "https://qyapi.weixin.qq.com/cgi-bin/webhook/send",
				Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
				Timeout: 10, Concurrency: 5, RetryTimes: 3, RetryInterval: 5,
				Request: RequestDetail{
					Body: "This is a Wecom notification.",
				},
			},
		},
	},
	Email: &NotifyChannelConfig{
		Name: Email, Ident: Email, RequestType: "smtp",
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
	},
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

func (ncc *NotifyChannelConfig) Upsert(ctx *ctx.Context, ident string) error {
	ch, err := NotifyChannelGet(ctx, "ident = ?", ident)
	if err != nil {
		return errors.WithMessage(err, "failed to get message tpl")
	}
	if ch == nil {
		return Insert(ctx, ncc)
	}

	if ch.UpdateBy != "" && ch.UpdateBy != "system" {
		return nil
	}
	return ch.Update(ctx, *ncc)
}
