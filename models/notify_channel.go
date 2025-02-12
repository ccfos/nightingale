package models

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/sys"
	"gopkg.in/gomail.v2"
	"html/template"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/satori/go.uuid"
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
	BatchSend bool           `json:"batch_send"` // 是否批量发送 user_info
}

// UserInfoParam user_info 类型的参数配置
type UserInfoParam struct {
	ContactKey string `json:"contact_key"` // phone, email, dingtalk_robot_token 等
}

// FlashDutyParam flashduty 类型的参数配置
type FlashDutyParam struct {
	ChannelIds []int64 `json:"channel_ids"`
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

	// 设置 TLS 配置
	tlsConfig := &tls.Config{
		//InsecureSkipVerify: nc.HTTPRequestConfig.TLS != nil && nc.HTTPRequestConfig.TLS.SkipVerify,
		InsecureSkipVerify: true,
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

func GetSMTPClient(nc *NotifyChannelConfig) (*smtp.Client, error) {
	// 连接到 SMTP 服务器
	addr := fmt.Sprintf("%s:%d", nc.SMTPRequestConfig.Host, nc.SMTPRequestConfig.Port)
	client, err := smtp.Dial(addr)
	if err != nil {
		fmt.Println(err)
		return nil, fmt.Errorf("failed to connect to SMTP server: %v", err)
	}

	// 如果服务器支持 STARTTLS，则升级到 TLS
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: nc.SMTPRequestConfig.InsecureSkipVerify,
			ServerName:         nc.SMTPRequestConfig.Host,
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			return nil, fmt.Errorf("failed to start TLS: %v", err)
		}
	}

	// 进行身份验证
	auth := smtp.PlainAuth("", nc.SMTPRequestConfig.Username, nc.SMTPRequestConfig.Password, nc.SMTPRequestConfig.Host)
	if err := client.Auth(auth); err != nil {
		fmt.Println(err)
		return nil, fmt.Errorf("failed to authenticate: %v", err)
	}

	return client, nil
}

func (ncc *NotifyChannelConfig) SendHTTP(content map[string]string, params []map[string]string, client *http.Client) error {

	if client == nil {
		return fmt.Errorf("http client not found")
	}

	for _, param := range params {

		// 将 MessageTemplate 与变量配置的信息渲染进 reqBody
		body, err := ncc.parseRequestBody(content, param)
		if err != nil {
			continue
		}

		// 替换 URL Header Parameters 中的变量
		ncc.replaceVariables(param)

		req, err := http.NewRequest(ncc.HTTPRequestConfig.Method, ncc.HTTPRequestConfig.URL, bytes.NewBuffer(body))
		if err != nil {
			continue
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

			//// 读取响应
			//body, err := ioutil.ReadAll(resp.Body)
			//if err != nil {
			//	fmt.Println("Error reading response:", err)
			//}
			//
			//// 打印响应
			//fmt.Println("Response:", string(body))

			if resp.StatusCode == http.StatusOK {
				continue
			}

			if i < ncc.HTTPRequestConfig.RetryTimes {
				time.Sleep(time.Duration(ncc.HTTPRequestConfig.RetryInterval) * time.Second)
			}
		}

		//return fmt.Errorf("failed to receive a successful response after %d retries", ncc.HTTPRequestConfig.RetryTimes)
	}

	return nil
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

func (ncc *NotifyChannelConfig) parseRequestBody(content map[string]string, params map[string]string) ([]byte, error) {
	// 将 MessageTemplate 与变量配置的信息渲染进 reqBody
	bodyTpl := make(map[string]interface{})
	bodyTpl["tpl"] = content
	for key, value := range params {
		bodyTpl[key] = value
	}
	tpl, err := template.New("requestBody").Funcs(tplx.TemplateFuncMap).Parse(ncc.HTTPRequestConfig.Request.Body)
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	err = tpl.Execute(&body, bodyTpl)
	return body.Bytes(), err
}

func (ncc *NotifyChannelConfig) replaceVariables(param map[string]string) {
	for k, v := range param {
		ncc.HTTPRequestConfig.URL = strings.Replace(ncc.HTTPRequestConfig.URL, "$"+k, v, -1)
	}

	for key, value := range ncc.HTTPRequestConfig.Headers {
		if !strings.HasPrefix(value, "$") {
			continue
		}
		ncc.HTTPRequestConfig.Headers[key] = param[value[1:]]
	}

	for key, value := range ncc.HTTPRequestConfig.Request.Parameters {
		if !strings.HasPrefix(value, "$") {
			continue
		}
		ncc.HTTPRequestConfig.Request.Parameters[key] = param[value[1:]]
	}
}

func (ncc *NotifyChannelConfig) SendEmail(events []*AlertCurEvent, content map[string]string, params []map[string]string, ch chan *EmailContext) error {

	for i := range params {
		param := params[i]
		var to []string
		if ncc.ParamConfig.BatchSend {
			to = strings.Split(param[ncc.ParamConfig.UserInfo.ContactKey], ",")
		} else {
			to = []string{param[ncc.ParamConfig.UserInfo.ContactKey]}
		}

		m := gomail.NewMessage()

		m.SetHeader("From", ncc.SMTPRequestConfig.From)
		m.SetHeader("To", strings.Join(to, ","))
		m.SetHeader("Subject", "Test Email")
		m.SetBody("text/html", getMessageTpl(events, content))

		ch <- &EmailContext{events, m}

	}
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

func (ncc *NotifyChannelConfig) SendScript(events []*AlertCurEvent, content map[string]string, params []map[string]string) error {

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

	for i := range params {

		cmd := exec.Command(fpath)
		cmd.Stdin = bytes.NewReader(getStdinBytes(events, content, params[i]))

		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf

		err := startCmd(cmd)
		if err != nil {
			fmt.Printf("event_script_notify_fail: run cmd err: %v\n", err)
			continue
		}

		err, isTimeout := sys.WrapTimeout(cmd, time.Duration(config.Timeout)*time.Second)

		if isTimeout {
			if err == nil {
				fmt.Printf("event_script_notify_fail: timeout and killed process %s", fpath)
			}

			if err != nil {
				fmt.Printf("event_script_notify_fail: kill process %s occur error %v", fpath, err)
			}
			continue
		}

		if err != nil {
			fmt.Printf("event_script_notify_fail: exec script %s occur error: %v, output: %s", fpath, err, buf.String())

			continue
		}
		fmt.Printf("event_script_notify_ok: exec %s output: %s", fpath, buf.String())
	}

	return nil
}

func getStdinBytes(events []*AlertCurEvent, content map[string]string, param map[string]string) []byte {
	// 创建一个 map 来存储所有数据
	data := map[string]interface{}{
		"events":  events,
		"content": content,
		"param":   param,
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
