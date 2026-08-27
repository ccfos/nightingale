package provider

import (
	"bytes"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"net/http"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
	"github.com/toolkits/pkg/logger"
)

// SendHTTPRequest 通用 HTTP 发送函数
// 从原 NotifyChannelConfig.SendHTTP 提取，供各 HTTP 类 Provider 复用
func SendHTTPRequest(httpConfig *models.HTTPRequestConfig, events []*models.AlertCurEvent,
	tpl map[string]interface{}, params map[string]string, sendtos []string,
	client *http.Client) (string, error) {

	if client == nil {
		return "", fmt.Errorf("http client not found")
	}

	if len(events) == 0 {
		return "", fmt.Errorf("events is empty")
	}

	// MessageTemplate + 变量配置
	fullTpl := buildNotifyTplData(events, tpl, params, sendtos)

	// 将 MessageTemplate 与变量配置的信息渲染进 reqBody
	body, err := parseRequestBody(httpConfig, fullTpl)
	if err != nil {
		logger.Errorf("failed to parse request body: %v, event: %v", err, events)
		return "", err
	}

	// 替换 URL Header Parameters 中的变量
	url, headers, parameters := replaceVariables(httpConfig, fullTpl)
	safeURL, safeHeaders, safeParams := redactForLog(httpConfig, fullTpl, url, headers, parameters)
	logger.Infof("url: %v, headers: %v, parameters: %v", safeURL, safeHeaders, safeParams)

	// 重试机制
	var lastErrorMessage string
	for i := 0; i < httpConfig.RetryTimes; i++ {
		var resp *http.Response
		req, err := makeHTTPRequest(httpConfig, url, headers, parameters, body)
		if err != nil {
			logger.Errorf("send_http: failed to create request. url=%s error=%v", safeURL, err)
			return fmt.Sprintf("failed to create request. error: %s", redactErrMsg(err, url, safeURL)), err
		}

		resp, err = client.Do(req)
		if err != nil {
			logger.Errorf("send_http: failed to send http notify. url=%s error=%v", safeURL, err)
			lastErrorMessage = redactErrMsg(err, url, safeURL)
			time.Sleep(time.Duration(httpConfig.RetryInterval) * time.Millisecond)
			continue
		}
		defer resp.Body.Close()

		// 读取响应
		respBody, err := io.ReadAll(resp.Body)
		logger.Debugf("send http request: %+v, response: %+v, body: %+v", req, resp, string(respBody))
		if err != nil {
			logger.Errorf("send_http: failed to read response. url=%s error=%v", safeURL, err)
		}
		if resp.StatusCode == http.StatusOK {
			return fmt.Sprintf("status_code:%d, response:%s", resp.StatusCode, string(respBody)), nil
		}

		return fmt.Sprintf("status_code:%d, response:%s", resp.StatusCode, string(respBody)), fmt.Errorf("failed to send request, status code: %d, body: %s", resp.StatusCode, string(respBody))
	}

	return lastErrorMessage, errors.New("all retries failed, last error: " + lastErrorMessage)
}

func parseRequestBody(httpConfig *models.HTTPRequestConfig, bodyTpl map[string]interface{}) ([]byte, error) {
	var defs = []string{
		"{{$tpl := .tpl}}",
		"{{$sendto := .sendto}}",
		"{{$sendtos := .sendtos}}",
		"{{$params := .params}}",
		"{{$events := .events}}",
		"{{$event := .event}}",
	}

	text := strings.Join(append(defs, httpConfig.Request.Body), "")
	tpl, err := htmltemplate.New("requestBody").Funcs(tplx.TemplateFuncMap).Parse(text)
	if err != nil {
		return nil, err
	}

	var body bytes.Buffer
	err = tpl.Execute(&body, bodyTpl)
	return body.Bytes(), err
}

func replaceVariables(httpConfig *models.HTTPRequestConfig, tpl map[string]interface{}) (string, map[string]string, map[string]string) {
	url := ""
	headers := make(map[string]string)
	parameters := make(map[string]string)

	if needsTemplateRendering(httpConfig.URL) {
		// 只打模板原文，不打渲染上下文：tpl 里含「变量配置」解密后的全部变量值
		logger.Infof("replace variables url template: %s", httpConfig.URL)
		warnUndefinedVars("url", httpConfig.URL, tpl)
		url = getParsedHTTPString("url", httpConfig.URL, tpl)
	} else {
		url = httpConfig.URL
	}

	for key, value := range httpConfig.Headers {
		if needsTemplateRendering(value) {
			warnUndefinedVars(key, value, tpl)
			headers[key] = getParsedHTTPString(key, value, tpl)
		} else {
			headers[key] = value
		}
	}

	for key, value := range httpConfig.Request.Parameters {
		if needsTemplateRendering(value) {
			warnUndefinedVars(key, value, tpl)
			parameters[key] = getParsedHTTPString(key, value, tpl)
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
	tpl, err := texttemplate.New(name).Funcs(tplx.TemplateFuncMap).Parse(text)
	if err != nil {
		return ""
	}
	var body bytes.Buffer
	err = tpl.Execute(&body, tplData)
	if err != nil {
		// 不回显 tplData：里面有「变量配置」解密后的变量值，而这个返回值会被当成
		// 渲染结果发给对端、写进日志、落进 notify_record
		return fmt.Sprintf("failed to parse template: %v", err)
	}

	return body.String()
}

// getParsedHTTPString 渲染 URL / 请求头 / 查询参数里的模板。
//
// 纯函数、无副作用：脱敏路径要拿掩码上下文把同一个模板再渲染一遍（见 redactField），
// 未定义变量告警因此挂在 replaceVariables 那一趟真实渲染上，不放在这里，否则告警翻倍。
//
// 这里维持 html/template 不变：events / params / tpl 的转义行为是既有约定
// （由 TestHTTPVariableRenderingKeepsLegacyHTMLEscaping 锁定），改引擎会影响所有存量配置。
// 用户变量不受这层转义影响——buildNotifyTplData 把变量按 template.HTML 注入，
// html/template 对该类型原样输出，凭证里的 + & 等字符不会被改写。
func getParsedHTTPString(name, tplStr string, tplData map[string]interface{}) string {
	var defs = []string{
		"{{$tpl := .tpl}}",
		"{{$sendto := .sendto}}",
		"{{$sendtos := .sendtos}}",
		"{{$params := .params}}",
		"{{$events := .events}}",
		"{{$event := .event}}",
	}

	text := strings.Join(append(defs, tplStr), "")
	tpl, err := htmltemplate.New(name).Funcs(tplx.TemplateFuncMap).Parse(text)
	if err != nil {
		return ""
	}
	var body bytes.Buffer
	err = tpl.Execute(&body, tplData)
	if err != nil {
		// 不回显 tplData：里面有「变量配置」解密后的变量值，而这个返回值会被当成
		// 渲染结果发给对端、写进日志、落进 notify_record
		return fmt.Sprintf("failed to parse template: %v", err)
	}

	return body.String()
}

func makeHTTPRequest(httpConfig *models.HTTPRequestConfig, url string, headers map[string]string, parameters map[string]string, body []byte) (*http.Request, error) {
	req, err := http.NewRequest(httpConfig.Method, url, bytes.NewBuffer(body))
	if err != nil {
		logger.Errorf("failed to create request: %v", err)
		return nil, err
	}

	query := req.URL.Query()
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	for key, value := range parameters {
		query.Add(key, value)
	}
	req.URL.RawQuery = query.Encode()
	// 记录完整的请求信息
	logger.Debugf("URL: %v, Method: %s, Headers: %+v, params: %+v, Body: %s", req.URL, req.Method, req.Header, query, string(body))

	return req, nil
}

func getNotifyTarget(customParams map[string]string, sendtos []string) string {
	if len(customParams) > 0 {
		if u, ok := customParams["callback_url"]; ok && u != "" {
			return u
		}
	}
	if len(sendtos) > 0 {
		return sendtos[0]
	}
	return ""
}
