package models

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/pkg/cmdx"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
	"github.com/google/uuid"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"gopkg.in/gomail.v2"
)

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

	Weight   int    `json:"weight"` // 权重，根据此字段对内置模板进行排序
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
	PagerDutyRequestConfig *PagerDutyRequestConfig `json:"pagerduty_request_config,omitempty" gorm:"serializer:json"`
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

func (ncc *NotifyChannelConfig) SendScript(events []*AlertCurEvent, tpl map[string]interface{}, params map[string]string, sendtos []string) (string, string, error) {
	config := ncc.RequestConfig.ScriptRequestConfig
	if config.Script == "" && config.Path == "" {
		return "", "", fmt.Errorf("script or path is empty")
	}

	fpath := ".notify_script_" + strconv.FormatInt(ncc.ID, 10)
	if config.Path != "" {
		fpath = config.Path
	} else {
		rewrite := true
		if file.IsExist(fpath) {
			oldContent, err := file.ToString(fpath)
			if err != nil {
				return "", "", fmt.Errorf("failed to read script file: %v", err)
			}

			if oldContent == config.Script {
				rewrite = false
			}
		}

		if rewrite {
			_, err := file.WriteString(fpath, config.Script)
			if err != nil {
				return "", "", fmt.Errorf("failed to write script file: %v", err)
			}

			err = os.Chmod(fpath, 0777)
			if err != nil {
				return "", "", fmt.Errorf("failed to chmod script file: %v", err)
			}
		}

		cur, _ := os.Getwd()
		fpath = path.Join(cur, fpath)
	}

	cmd := exec.Command(fpath)
	cmd.Stdin = bytes.NewReader(getStdinBytes(events, tpl, params, sendtos))

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err, isTimeout := cmdx.RunTimeout(cmd, time.Duration(config.Timeout)*time.Millisecond)
	logger.Infof("event_script_notify_result: exec %s output: %s isTimeout: %v err: %v stdin: %s", fpath, buf.String(), isTimeout, err, string(getStdinBytes(events, tpl, params, sendtos)))

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

	if isTimeout {
		if err == nil {
			return cmd.String(), res, errors.New("timeout and killed process")
		}

		return cmd.String(), res, err
	}
	if err != nil {
		return cmd.String(), res, fmt.Errorf("failed to execute script: %v", err)
	}

	return cmd.String(), res, nil
}

func getStdinBytes(events []*AlertCurEvent, tpl map[string]interface{}, params map[string]string, sendtos []string) []byte {
	if len(events) == 0 {
		return []byte("")
	}

	// 创建一个 map 来存储所有数据
	data := map[string]interface{}{
		"event":   events[0],
		"events":  events,
		"tpl":     tpl,
		"params":  params,
		"sendtos": sendtos,
	}

	// 将数据序列化为 JSON 字节数组
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil
	}

	return jsonBytes
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
	if nc.RequestConfig == nil || nc.RequestConfig.HTTPRequestConfig == nil {
		return nil, fmt.Errorf("%+v http request config not found", nc)
	}

	httpConfig := nc.RequestConfig.HTTPRequestConfig

	// 对于 FlashDuty 类型，优先使用 FlashDuty 配置中的超时时间
	timeout := httpConfig.Timeout
	if nc.RequestType == "flashduty" && nc.RequestConfig.FlashDutyRequestConfig != nil {
		flashDutyTimeout := nc.RequestConfig.FlashDutyRequestConfig.Timeout
		if flashDutyTimeout > 0 {
			timeout = flashDutyTimeout
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
			Timeout: time.Duration(timeout) * time.Millisecond,
		}).DialContext,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(timeout) * time.Millisecond,
	}

	return client, nil
}

func (ncc *NotifyChannelConfig) makeHTTPRequest(httpConfig *HTTPRequestConfig, url string, headers map[string]string, parameters map[string]string, body []byte) (*http.Request, error) {
	req, err := http.NewRequest(httpConfig.Method, url, bytes.NewBuffer(body))
	if err != nil {
		logger.Errorf("failed to create request: %v", err)
		return nil, err
	}

	query := req.URL.Query()
	// 设置请求头 腾讯云短信、语音特殊处理
	if ncc.Ident == "tx-sms" || ncc.Ident == "tx-voice" {
		headers = ncc.setTxHeader(headers, body)
		for key, value := range headers {
			req.Header.Add(key, value)
		}
	} else if ncc.Ident == "ali-sms" || ncc.Ident == "ali-voice" {
		req, err = http.NewRequest(httpConfig.Method, url, nil)
		if err != nil {
			return nil, err
		}

		query, headers = ncc.getAliQuery(ncc.Ident, query, httpConfig.Request.Parameters["AccessKeyId"], httpConfig.Request.Parameters["AccessKeySecret"], parameters)
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	} else {
		for key, value := range headers {
			req.Header.Add(key, value)
		}
	}

	if ncc.Ident != "ali-sms" && ncc.Ident != "ali-voice" {
		for key, value := range parameters {
			query.Add(key, value)
		}
	}

	req.URL.RawQuery = query.Encode()
	// 记录完整的请求信息
	logger.Debugf("URL: %v, Method: %s, Headers: %+v, params: %+v, Body: %s", req.URL, req.Method, req.Header, query, string(body))

	return req, nil
}

func (ncc *NotifyChannelConfig) makeFlashDutyRequest(url string, bodyBytes []byte, flashDutyChannelID int64) (*http.Request, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	// 设置 URL 参数
	query := req.URL.Query()
	if flashDutyChannelID != 0 {
		// 如果 flashduty 有配置协作空间(channel_id)，则传入 channel_id 参数
		query.Add("channel_id", strconv.FormatInt(flashDutyChannelID, 10))
	}
	req.URL.RawQuery = query.Encode()
	req.Header.Add("Content-Type", "application/json")
	return req, nil
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

	url := ncc.RequestConfig.FlashDutyRequestConfig.IntegrationUrl

	retrySleep := time.Second
	if ncc.RequestConfig.FlashDutyRequestConfig.RetrySleep > 0 {
		retrySleep = time.Duration(ncc.RequestConfig.FlashDutyRequestConfig.RetrySleep) * time.Millisecond
	}

	retryTimes := 3
	if ncc.RequestConfig.FlashDutyRequestConfig.RetryTimes > 0 {
		retryTimes = ncc.RequestConfig.FlashDutyRequestConfig.RetryTimes
	}

	// 把最后一次错误保存下来，后面返回，让用户在页面上也可以看到
	var lastErrorMessage string
	for i := 0; i <= retryTimes; i++ {
		req, err := ncc.makeFlashDutyRequest(url, body, flashDutyChannelID)
		if err != nil {
			logger.Errorf("send_flashduty: failed to create request. url=%s request_body=%s error=%v", url, string(body), err)
			return fmt.Sprintf("failed to create request. error: %v", err), err
		}

		// 直接使用客户端发送请求，超时时间已经在 client 中设置
		resp, err := client.Do(req)
		if err != nil {
			logger.Errorf("send_flashduty: http_call=fail url=%s request_body=%s error=%v times=%d", url, string(body), err, i+1)
			if i < retryTimes {
				// 重试等待时间，后面要放到页面上配置
				time.Sleep(retrySleep)
			}
			lastErrorMessage = err.Error()
			continue
		}

		// 走到这里，说明请求 Flashduty 成功，不管 Flashduty 返回了什么结果，都不判断，仅保存，给用户查看即可
		// 比如服务端返回 5xx，也不要重试，重试可能会导致服务端数据有问题。告警事件这样的东西，没有那么关键，只要最终能在 UI 上看到调用结果就行
		var resBody []byte
		if resp.Body != nil {
			defer resp.Body.Close()

			resBody, err = io.ReadAll(resp.Body)
			if err != nil {
				logger.Errorf("send_flashduty: failed to read response. request_body=%s, error=%v", string(body), err)
				resBody = []byte("failed to read response. error: " + err.Error())
			}
		}

		logger.Infof("send_flashduty: http_call=succ url=%s request_body=%s response_code=%d response_body=%s times=%d", url, string(body), resp.StatusCode, string(resBody), i+1)
		return fmt.Sprintf("status_code:%d, response:%s", resp.StatusCode, string(resBody)), nil
	}

	return lastErrorMessage, errors.New("failed to send request")
}

func (ncc *NotifyChannelConfig) SendPagerDuty(events []*AlertCurEvent, routingKey, siteUrl string, client *http.Client) (string, error) {
	if client == nil {
		return "", fmt.Errorf("http client not found")
	}
	if ncc.RequestConfig == nil || ncc.RequestConfig.PagerDutyRequestConfig == nil {
		return "", fmt.Errorf("pagerduty request config not found")
	}

	retrySleep := time.Second
	if ncc.RequestConfig.PagerDutyRequestConfig.RetrySleep > 0 {
		retrySleep = time.Duration(ncc.RequestConfig.PagerDutyRequestConfig.RetrySleep) * time.Millisecond
	}

	retryTimes := 3
	if ncc.RequestConfig.PagerDutyRequestConfig.RetryTimes > 0 {
		retryTimes = ncc.RequestConfig.PagerDutyRequestConfig.RetryTimes
	}

	endpoint := "https://events.pagerduty.com/v2/enqueue"
	var failedMsgs []string
	var responses []string

	for _, event := range events {
		action := "trigger"
		if event.IsRecovered {
			action = "resolve"
		}

		severity := "critical"
		switch event.Severity {
		case 2:
			severity = "error"
		case 3:
			severity = "warning"
		}

		jsonBody := map[string]interface{}{
			"routing_key":  routingKey,
			"event_action": action,
			"dedup_key":    event.Hash,
			"payload": map[string]interface{}{
				"summary":   event.RuleName,
				"source":    event.Cluster,
				"severity":  severity,
				"group":     event.GroupName,
				"component": event.Target,
				"timestamp": time.Unix(event.TriggerTime, 0).Format(time.RFC3339),
				"custom_details": map[string]interface{}{
					"tags":               event.TagsJSON,
					"annotations":        event.AnnotationsJSON,
					"cluster":            event.Cluster,
					"rule_id":            event.RuleId,
					"rule_note":          event.RuleNote,
					"rule_prod":          event.RuleProd,
					"prom_ql":            event.PromQl,
					"target_ident":       event.TargetIdent,
					"target_note":        event.TargetNote,
					"datasource_id":      event.DatasourceId,
					"first_trigger_time": time.Unix(event.FirstTriggerTime, 0).Format(time.RFC3339),
					"prom_for_duration":  event.PromForDuration,
					"runbook_url":        event.RunbookUrl,
					"notify_cur_number":  event.NotifyCurNumber,
					"group_id":           event.GroupId,
					"cate":               event.Cate,
				},
			},
			"links": []map[string]string{
				{"href": fmt.Sprintf("%s/alert-his-events/%d", siteUrl, event.Id), "text": "Event Detail"},
				{"href": fmt.Sprintf("%s/alert-mutes/add?__event_id=%d", siteUrl, event.Id), "text": "Mute this alert"},
			},
		}

		body, err := json.Marshal(jsonBody)
		if err != nil {
			logger.Errorf("send_pagerduty: failed to marshal request body. error=%v", err)
			failedMsgs = append(failedMsgs, fmt.Sprintf("event %d marshal error: %v", event.Id, err))
			// 记录一条空响应占位，方便上层区分事件
			responses = append(responses, fmt.Sprintf("event %d: marshal error: %v", event.Id, err))
			continue
		}

		var lastErrorMessage string
		var lastRespSummary string
		attempts := retryTimes + 1
		for i := 0; i < attempts; i++ {
			req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
			if err != nil {
				logger.Errorf("send_pagerduty: failed to create request. url=%s request_body=%s error=%v", endpoint, string(body), err)
				lastErrorMessage = err.Error()
				if i < attempts-1 {
					time.Sleep(retrySleep)
					continue
				}
				break
			}
			req.Header.Add("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				logger.Errorf("send_pagerduty: http_call=fail url=%s request_body=%s error=%v times=%d", endpoint, string(body), err, i+1)
				lastErrorMessage = err.Error()
				if i < attempts-1 {
					time.Sleep(retrySleep)
					continue
				}
				break
			}

			// 确保关闭 body
			var resBody []byte
			if resp.Body != nil {
				resBody, err = io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					logger.Errorf("send_pagerduty: failed to read response. request_body=%s, error=%v", string(body), err)
					resBody = []byte("failed to read response. error: " + err.Error())
				}
			} else {
				resBody = []byte("")
			}

			respSummary := fmt.Sprintf("status_code:%d, response:%s", resp.StatusCode, string(resBody))
			lastRespSummary = respSummary

			logger.Infof("send_pagerduty: http_call=succ url=%s request_body=%s response_code=%d response_body=%s times=%d", endpoint, string(body), resp.StatusCode, string(resBody), i+1)

			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
				// 当前事件发送成功
				lastErrorMessage = ""
				break
			}

			lastErrorMessage = respSummary
			if i < attempts-1 {
				time.Sleep(retrySleep)
				continue
			}
			break
		}

		// 保存本次事件的响应摘要（无论成功或失败），便于上层记录 traceId 等信息
		if lastRespSummary == "" && lastErrorMessage != "" {
			lastRespSummary = lastErrorMessage
		}
		responses = append(responses, fmt.Sprintf("event %d: %s", event.Id, lastRespSummary))

		if lastErrorMessage != "" {
			failedMsgs = append(failedMsgs, fmt.Sprintf("event %d: %s", event.Id, lastErrorMessage))
		}
	}

	// 将每个 event 的响应摘要返回给上层，便于记录 pagerduty 返回的 traceId 等信息
	if len(failedMsgs) > 0 {
		return strings.Join(responses, " | "), errors.New(strings.Join(failedMsgs, " | "))
	}
	return strings.Join(responses, " | "), nil
}

func (ncc *NotifyChannelConfig) SendHTTP(events []*AlertCurEvent, tpl map[string]interface{}, params map[string]string, sendtos []string, client *http.Client) (string, error) {
	if client == nil {
		return "", fmt.Errorf("http client not found")
	}

	if len(events) == 0 {
		return "", fmt.Errorf("events is empty")
	}

	httpConfig := ncc.RequestConfig.HTTPRequestConfig

	// MessageTemplate
	fullTpl := make(map[string]interface{})

	fullTpl["sendtos"] = sendtos // 发送对象
	fullTpl["params"] = params   // 自定义参数
	fullTpl["tpl"] = tpl
	fullTpl["events"] = events
	fullTpl["event"] = events[0]

	if len(sendtos) > 0 {
		fullTpl["sendto"] = sendtos[0]
	}

	// 将 MessageTemplate 与变量配置的信息渲染进 reqBody
	body, err := ncc.parseRequestBody(fullTpl)
	if err != nil {
		logger.Errorf("failed to parse request body: %v, event: %v", err, events)
		return "", err
	}

	// 替换 URL Header Parameters 中的变量
	url, headers, parameters := ncc.replaceVariables(fullTpl)
	logger.Infof("url: %v, headers: %v, parameters: %v", url, headers, parameters)

	// 重试机制
	var lastErrorMessage string
	for i := 0; i < httpConfig.RetryTimes; i++ {
		var resp *http.Response
		req, err := ncc.makeHTTPRequest(httpConfig, url, headers, parameters, body)
		if err != nil {
			logger.Errorf("send_http: failed to create request. url=%s request_body=%s error=%v", url, string(body), err)
			return fmt.Sprintf("failed to create request. error: %v", err), err
		}

		resp, err = client.Do(req)
		if err != nil {
			logger.Errorf("send_http: failed to send http notify. url=%s request_body=%s error=%v", url, string(body), err)
			lastErrorMessage = err.Error()
			time.Sleep(time.Duration(httpConfig.RetryInterval) * time.Millisecond)
			continue
		}
		defer resp.Body.Close()

		// 读取响应
		body, err := io.ReadAll(resp.Body)
		logger.Debugf("send http request: %+v, response: %+v, body: %+v", req, resp, string(body))
		if err != nil {
			logger.Errorf("send_http: failed to read response. url=%s request_body=%s error=%v", url, string(body), err)
		}
		if resp.StatusCode == http.StatusOK {
			return string(body), nil
		}

		return "", fmt.Errorf("failed to send request, status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return lastErrorMessage, errors.New("all retries failed, last error: " + lastErrorMessage)
}

// getAliQuery 获取阿里云API的查询参数和请求头
func (ncc *NotifyChannelConfig) getAliQuery(ident string, query url.Values, ak, sk string, params map[string]string) (url.Values, map[string]string) {
	// 获取基础配置
	httpConfig := ncc.RequestConfig.HTTPRequestConfig

	httpMethod := "POST"
	canonicalURI := "/"

	var queryParams map[string]string
	if ident == "ali-sms" {
		queryParams = map[string]string{
			"PhoneNumbers":  params["PhoneNumbers"],
			"SignName":      params["SignName"],
			"TemplateCode":  params["TemplateCode"],
			"TemplateParam": params["TemplateParam"],
		}
	} else if ident == "ali-voice" {
		queryParams = map[string]string{
			"CalledNumber":     params["CalledNumber"],
			"TtsCode":          params["TtsCode"],
			"TtsParam":         params["TtsParam"],
			"CalledShowNumber": params["CalledShowNumber"],
		}
	}

	// 设置基础headers
	headers := map[string]string{
		"host":                  httpConfig.Headers["Host"],
		"x-acs-version":         "2017-05-25",
		"x-acs-date":            time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"x-acs-signature-nonce": uuid.New().String(),
		"x-acs-content-sha256":  fmt.Sprintf("%x", sha256.Sum256([]byte(""))),
	}

	// 根据服务类型设置action
	if ncc.Ident == "ali-sms" {
		headers["x-acs-action"] = "SendSms"
	} else if ncc.Ident == "ali-voice" {
		headers["x-acs-action"] = "SingleCallByTts"
	}

	// 计算签名
	signature, signedHeaders := getSignature(sk, httpMethod, canonicalURI, headers, queryParams, "")

	// 添加授权头
	headers["Authorization"] = fmt.Sprintf("ACS3-HMAC-SHA256 Credential=%s,SignedHeaders=%s,Signature=%s",
		ak, signedHeaders, signature)

	// 业务参数
	for k, v := range queryParams {
		query.Add(k, v)
	}

	query.Del("AccessKeyId")
	query.Del("AccessKeySecret")

	return query, headers
}

// getSignature 计算签名
func getSignature(accessKeySecret string, httpMethod, canonicalURI string, headers map[string]string, queryParams map[string]string, body string) (string, string) {
	// 1. 构造规范化请求
	// 处理查询参数
	var sortedQueryParams []string
	for k, v := range queryParams {
		sortedQueryParams = append(sortedQueryParams, fmt.Sprintf("%s=%s",
			percentEncode(k), percentEncode(v)))
	}
	sort.Strings(sortedQueryParams)
	canonicalQueryString := strings.Join(sortedQueryParams, "&")

	// 处理请求头
	var canonicalHeaders []string
	var signedHeaders []string
	for k, v := range headers {
		lowerK := strings.ToLower(k)
		if lowerK == "host" || lowerK == "content-type" || strings.HasPrefix(lowerK, "x-acs-") {
			canonicalHeaders = append(canonicalHeaders, fmt.Sprintf("%s:%s", lowerK, strings.TrimSpace(v)))
			signedHeaders = append(signedHeaders, lowerK)
		}
	}
	sort.Strings(canonicalHeaders)
	sort.Strings(signedHeaders)

	canonicalHeadersStr := strings.Join(canonicalHeaders, "\n") + "\n"
	signedHeadersStr := strings.Join(signedHeaders, ";")

	// 计算body的hash值
	h := sha256.New()
	h.Write([]byte(body))
	bodyHash := hex.EncodeToString(h.Sum(nil))

	// 构造规范化请求
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		httpMethod, canonicalURI, canonicalQueryString, canonicalHeadersStr,
		signedHeadersStr, bodyHash)

	// 2. 构造待签名字符串
	algorithm := "ACS3-HMAC-SHA256"
	h = sha256.New()
	h.Write([]byte(canonicalRequest))
	canonicalRequestHash := hex.EncodeToString(h.Sum(nil))
	stringToSign := fmt.Sprintf("%s\n%s", algorithm, canonicalRequestHash)

	// 3. 计算签名
	h = hmac.New(sha256.New, []byte(accessKeySecret))
	h.Write([]byte(stringToSign))
	signature := hex.EncodeToString(h.Sum(nil))

	return signature, signedHeadersStr
}

func percentEncode(str string) string {
	encoded := url.QueryEscape(str)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

func (ncc *NotifyChannelConfig) setTxHeader(headers map[string]string, payloadBytes []byte) map[string]string {
	timestamp := time.Now().Unix()

	authorization := ncc.getTxSignature(string(payloadBytes), timestamp)
	headers["X-TC-Timestamp"] = fmt.Sprintf("%d", timestamp)
	headers["Authorization"] = authorization

	return headers
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

func (ncc *NotifyChannelConfig) parseRequestBody(bodyTpl map[string]interface{}) ([]byte, error) {
	var defs = []string{
		"{{$tpl := .tpl}}",
		"{{$sendto := .sendto}}",
		"{{$sendtos := .sendtos}}",
		"{{$params := .params}}",
		"{{$events := .events}}",
		"{{$event := .event}}",
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
		"{{$sendtos := .sendtos}}",
		"{{$params := .params}}",
		"{{$events := .events}}",
		"{{$event := .event}}",
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

func (ncc *NotifyChannelConfig) replaceVariables(tpl map[string]interface{}) (string, map[string]string, map[string]string) {
	httpConfig := ncc.RequestConfig.HTTPRequestConfig
	url := ""
	headers := make(map[string]string)
	parameters := make(map[string]string)

	if needsTemplateRendering(httpConfig.URL) {
		logger.Infof("replace variables url: %s tpl: %+v", httpConfig.URL, tpl)
		url = getParsedString("url", httpConfig.URL, tpl)
	} else {
		url = httpConfig.URL
	}

	for key, value := range httpConfig.Headers {
		if needsTemplateRendering(value) {
			headers[key] = html.UnescapeString(getParsedString(key, value, tpl))
		} else {
			headers[key] = value
		}
	}

	for key, value := range httpConfig.Request.Parameters {
		if needsTemplateRendering(value) {
			parameters[key] = getParsedString(key, value, tpl)
		} else {
			parameters[key] = value
		}
	}

	return url, headers, parameters
}

// needsTemplateRendering 检查字符串是否包含模板语法
func needsTemplateRendering(s string) bool {
	return strings.Contains(s, "{{") && strings.Contains(s, "}}")
}

func (ncc *NotifyChannelConfig) SendEmail(notifyRuleId int64, events []*AlertCurEvent, tpl map[string]interface{}, sendtos []string, ch chan *EmailContext) {
	m := gomail.NewMessage()
	m.SetHeader("From", ncc.RequestConfig.SMTPRequestConfig.From)
	m.SetHeader("To", sendtos...)
	m.SetHeader("Subject", tpl["subject"].(string))
	m.SetBody("text/html", tpl["content"].(string))
	ch <- &EmailContext{notifyRuleId, events, m}
}

func (ncc *NotifyChannelConfig) SendEmailNow(events []*AlertCurEvent, tpl map[string]interface{}, sendtos []string) error {

	d := gomail.NewDialer(ncc.RequestConfig.SMTPRequestConfig.Host, ncc.RequestConfig.SMTPRequestConfig.Port, ncc.RequestConfig.SMTPRequestConfig.Username, ncc.RequestConfig.SMTPRequestConfig.Password)
	if ncc.RequestConfig.SMTPRequestConfig.InsecureSkipVerify {
		d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}
	s, err := d.Dial()
	if err != nil {
		logger.Errorf("email_sender: failed to dial: %s", err)
		return err
	}

	m := gomail.NewMessage()
	m.SetHeader("From", ncc.RequestConfig.SMTPRequestConfig.From)
	m.SetHeader("To", sendtos...)
	m.SetHeader("Subject", tpl["subject"].(string))
	m.SetBody("text/html", tpl["content"].(string))
	return gomail.Send(s, m)
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

	if ncc.RequestType != "http" && ncc.RequestType != "smtp" && ncc.RequestType != "script" && ncc.RequestType != "flashduty" && ncc.RequestType != "pagerduty" {
		return errors.New("invalid request type, must be 'http', 'smtp' or 'script'")
	}

	if ncc.ParamConfig != nil {
		for _, param := range ncc.ParamConfig.Custom.Params {
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
	case "pagerduty":
		if err := ncc.ValidatePagerDutyRequestConfig(); err != nil {
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

var NotiChMap = []*NotifyChannelConfig{
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
		Name: "Discord", Ident: Discord, RequestType: "http", Weight: 16, Enable: false,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "{{$params.webhook_url}}",
				Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Body: `{"content": "{{$tpl.content}}"}`,
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			Custom: Params{
				Params: []ParamItem{
					{Key: "webhook_url", CName: "Webhook Url", Type: "string"},
				},
			},
		},
	},
	{
		Name: "MattermostWebhook", Ident: MattermostWebhook, RequestType: "http", Weight: 15, Enable: false,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "{{$params.webhook_url}}",
				Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Body: `{"text":  "{{$tpl.content}}"}`,
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			Custom: Params{
				Params: []ParamItem{
					{Key: "webhook_url", CName: "Webhook Url", Type: "string"},
					{Key: "bot_name", CName: "Bot Name", Type: "string"},
				},
			},
		},
	},
	{
		Name: "MattermostBot", Ident: MattermostBot, RequestType: "http", Weight: 14, Enable: false,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "<your mattermost url>/api/v4/posts",
				Method: "POST", Headers: map[string]string{"Content-Type": "application/json", "Authorization": "Bearer <you mattermost bot token>"},
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Body: `{"channel_id": "{{$params.channel_id}}", "message":  "{{$tpl.content}}"}`,
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			Custom: Params{
				Params: []ParamItem{
					{Key: "channel_id", CName: "Channel ID", Type: "string"},
					{Key: "channel_name", CName: "Channel Name", Type: "string"},
				},
			},
		},
	},
	{
		Name: "SlackWebhook", Ident: SlackWebhook, RequestType: "http", Weight: 13, Enable: false,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "{{$params.webhook_url}}",
				Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Body: `{"text":  "{{$tpl.content}}", "mrkdwn": true}`,
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			Custom: Params{
				Params: []ParamItem{
					{Key: "webhook_url", CName: "Webhook Url", Type: "string"},
					{Key: "bot_name", CName: "Bot Name", Type: "string"},
				},
			},
		},
	},
	{
		Name: "SlackBot", Ident: SlackBot, RequestType: "http", Weight: 12, Enable: false,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "https://slack.com/api/chat.postMessage",
				Method: "POST", Headers: map[string]string{"Content-Type": "application/json", "Authorization": "Bearer <you slack bot token>"},
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Body: `{"channel": "#{{$params.channel}}", "text":  "{{$tpl.content}}", "mrkdwn": true}`,
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			Custom: Params{
				Params: []ParamItem{
					{Key: "channel", CName: "channel", Type: "string"},
					{Key: "channel_name", CName: "Channel Name", Type: "string"},
				},
			},
		},
	},
	{
		Name: "Tencent SMS", Ident: "tx-sms", RequestType: "http", Weight: 11, Enable: true,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				Method:  "POST",
				URL:     "https://sms.tencentcloudapi.com",
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Body: `{"PhoneNumberSet":["{{ $sendto }}"],"SignName":"需要改为实际的签名","SmsSdkAppId":"需要改为实际的appid","TemplateId":"需要改为实际的模板id","TemplateParamSet":["{{$tpl.content}}"]}`,
				},
				Headers: map[string]string{
					"Content-Type": "application/json",
					"Host":         "sms.tencentcloudapi.com",
					"X-TC-Action":  "SendSms",
					"X-TC-Version": "2021-01-11",
					"X-TC-Region":  "需要改为实际的region",
					"Service":      "sms",
					"Secret_ID":    "需要改为实际的secret_id",
					"Secret_Key":   "需要改为实际的secret_key",
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			UserInfo: &UserInfo{
				ContactKey: "phone",
			},
		},
	},
	{
		Name: "Tencent Voice", Ident: "tx-voice", RequestType: "http", Weight: 10, Enable: true,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				Method:  "POST",
				URL:     "https://vms.tencentcloudapi.com",
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Body: `{"CalledNumber":"+86{{ $sendto }}","TemplateId":"需要改为实际的模板id","TemplateParamSet":["{{$tpl.content}}"],"VoiceSdkAppid":"需要改为实际的appid"}`,
				},
				Headers: map[string]string{
					"Content-Type": "application/json",
					"Host":         "vms.tencentcloudapi.com",
					"X-TC-Action":  "SendTtsVoice",
					"X-TC-Version": "2020-09-02",
					"X-TC-Region":  "ap-beijing",
					"Service":      "vms",
					"Secret_ID":    "需要改为实际的secret_id",
					"Secret_Key":   "需要改为实际的secret_key",
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			UserInfo: &UserInfo{
				ContactKey: "phone",
			},
		},
	},
	{
		Name: "Aliyun SMS", Ident: "ali-sms", RequestType: "http", Weight: 9, Enable: true,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				Method:  "POST",
				URL:     "https://dysmsapi.aliyuncs.com",
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Parameters: map[string]string{
						"PhoneNumbers":    "{{ $sendto }}",
						"SignName":        "需要改为实际的签名",
						"TemplateCode":    "需要改为实际的模板id",
						"TemplateParam":   `{"incident":"故障{{$tpl.incident}}，请及时处理"}`,
						"AccessKeyId":     "需要改为实际的access_key_id",
						"AccessKeySecret": "需要改为实际的access_key_secret",
					},
				},
				Headers: map[string]string{
					"Content-Type": "application/json",
					"Host":         "dysmsapi.aliyuncs.com",
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			UserInfo: &UserInfo{
				ContactKey: "phone",
			},
		},
	},
	{
		Name: "Aliyun Voice", Ident: "ali-voice", RequestType: "http", Weight: 8, Enable: true,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				Method:  "POST",
				URL:     "https://dyvmsapi.aliyuncs.com",
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Parameters: map[string]string{
						"TtsCode":          "需要改为实际的voice_code",
						"TtsParam":         `{"incident":"故障{{$tpl.incident}}，一键认领请按1"}`,
						"CalledNumber":     `{{ $sendto }}`,
						"CalledShowNumber": `需要改为实际的show_number, 如果为空则不显示`,
						"AccessKeyId":      "需要改为实际的access_key_id",
						"AccessKeySecret":  "需要改为实际的access_key_secret",
					},
				},
				Headers: map[string]string{
					"Content-Type": "application/json",
					"Host":         "dyvmsapi.aliyuncs.com",
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			UserInfo: &UserInfo{
				ContactKey: "phone",
			},
		},
	},
	{
		Name: "Telegram", Ident: Telegram, RequestType: "http", Weight: 7, Enable: true,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "https://api.telegram.org/bot{{$params.token}}/sendMessage",
				Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Parameters: map[string]string{"chat_id": "{{$params.chat_id}}"},
					Body:       `{"text":"{{$tpl.content}}","parse_mode": "HTML"}`,
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			Custom: Params{
				Params: []ParamItem{
					{Key: "token", CName: "Token", Type: "string"},
					{Key: "chat_id", CName: "Chat Id", Type: "string"},
					{Key: "bot_name", CName: "Bot Name", Type: "string"},
				},
			},
		},
	},
	{
		Name: "Lark", Ident: Lark, RequestType: "http", Weight: 6, Enable: true,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "https://open.larksuite.com/open-apis/bot/v2/hook/{{$params.token}}",
				Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Body: `{"msg_type": "text", "content": {"text": "{{$tpl.content}}"}}`,
				},
			},
		},
		ParamConfig: &NotifyParamConfig{
			Custom: Params{
				Params: []ParamItem{
					{Key: "token", CName: "Token", Type: "string"},
					{Key: "bot_name", CName: "Bot Name", Type: "string"},
				},
			},
		},
	},

	{
		Name: "Lark Card", Ident: LarkCard, RequestType: "http", Weight: 6, Enable: true,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "https://open.larksuite.com/open-apis/bot/v2/hook/{{$params.token}}",
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
					{Key: "token", CName: "Token", Type: "string"},
					{Key: "bot_name", CName: "Bot Name", Type: "string"},
				},
			},
		},
	},
	{
		Name: "Feishu", Ident: Feishu, RequestType: "http", Weight: 5, Enable: true,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				URL:    "https://open.feishu.cn/open-apis/bot/v2/hook/{{$params.access_token}}",
				Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Body: `{"msg_type": "text", "content": {"text": "{{$tpl.content}}"}}`,
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
				Timeout:        5000, // 默认5秒超时
				RetryTimes:     3,    // 默认重试3次
			},
		},
	},
	{
		Name: "PagerDuty", Ident: "pagerduty", RequestType: "pagerduty", Weight: 1, Enable: true,
		RequestConfig: &RequestConfig{
			PagerDutyRequestConfig: &PagerDutyRequestConfig{
				ApiKey:     "pagerduty api key",
				Timeout:    5000,
				RetryTimes: 3,
			},
		},
	},
	{
		Name: "ServiceNow", Ident: "servicenow", RequestType: "http", Weight: 1, Enable: true,
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				Method:  "POST",
				URL: "https://<your-instance>.service-now.com/em_event.do?JSONv2&sysparm_action=insert",
				Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
				Request: RequestDetail{
					Body: `{
	"source": "nightingale",
	"event_class": "{{$event.Cate}}",
	"severity": "{{if $event.IsRecovered}}0{{else if eq $event.Severity 1}}1{{else if eq $event.Severity 2}}2{{else}}3{{end}}",
	"resolution_state": "{{if $event.IsRecovered}}Closing{{else}}New{{end}}",
	"message_key": "{{$event.Hash}}",
	"node": "{{$event.TargetIdent}}",
	"resource": "{{$event.Cluster}}",
	"type": "{{$event.RuleProd}}",
	"metric_name": "{{$event.RuleName}}",
	"description": "{{if $event.IsRecovered}}[Recovered] {{end}}{{$event.RuleName}} - {{$event.TargetIdent}} {{if not $event.IsRecovered}}Trigger Value: {{$event.TriggerValue}}{{end}}",
 	"time_of_event": "{{timeformat $event.TriggerTime}}",
 	"additional_info": {"n9e_id":{{$event.Id}},"rule_id":{{$event.RuleId}},"rule_name":"{{$event.RuleName}}","rule_note":"{{$event.RuleNote}}","group_id":{{$event.GroupId}},"group_name":"{{$event.GroupName}}","datasource_id":{{$event.DatasourceId}},"cluster":"{{$event.Cluster}}","cate":"{{$event.Cate}}","prom_ql":"{{$event.PromQl}}","trigger_value":"{{$event.TriggerValue}}","runbook_url":"{{$event.RunbookUrl}}","tags":"{{$event.Tags}}","target_note":"{{$event.TargetNote}}","first_trigger_time":{{$event.FirstTriggerTime}},"notify_cur_number":{{$event.NotifyCurNumber}}}
}`,
				},
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Accept":        "application/json",
					"Authorization": "Basic {{ basicAuth \"<username>\" \"<password>\" }}",
				},
			},
		},
	},
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
