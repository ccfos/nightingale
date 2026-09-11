package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

const AliyunVoiceIdent = "ali-voice"

type AliyunVoiceProvider struct{}

func (p *AliyunVoiceProvider) Ident() string {
	return AliyunVoiceIdent
}

func (p *AliyunVoiceProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("aliyun voice provider requires POST method")
	}

	if httpConfig.URL == "" {
		return errors.New("aliyun voice provider requires URL")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("aliyun voice provider requires Content-Type: application/json header")
	}

	if httpConfig.Request.Body == "" && len(httpConfig.Request.Parameters) == 0 {
		return errors.New("aliyun voice provider requires request body or parameters")
	}

	return nil
}

func (p *AliyunVoiceProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := p.sendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}

// 从原 NotifyChannelConfig.SendHTTP 提取，供各 HTTP 类 Provider 复用
func (p *AliyunVoiceProvider) sendHTTPRequest(httpConfig *models.HTTPRequestConfig, events []*models.AlertCurEvent,
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
		req, err := p.makeHTTPRequest(httpConfig, url, headers, parameters, body)
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
		body, err := io.ReadAll(resp.Body)
		logger.Debugf("send http request: %+v, response: %+v, body: %+v", req, resp, string(body))
		if err != nil {
			logger.Errorf("send_http: failed to read response. url=%s error=%v", safeURL, err)
		}
		if resp.StatusCode == http.StatusOK {
			return fmt.Sprintf("status_code:%d, response:%s", resp.StatusCode, string(body)), nil
		}

		return fmt.Sprintf("status_code:%d, response:%s", resp.StatusCode, string(body)), fmt.Errorf("failed to send request, status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return lastErrorMessage, errors.New("all retries failed, last error: " + lastErrorMessage)
}

func (p *AliyunVoiceProvider) makeHTTPRequest(httpConfig *models.HTTPRequestConfig, url string, headers map[string]string, parameters map[string]string, body []byte) (*http.Request, error) {
	req, err := http.NewRequest(httpConfig.Method, url, nil)
	if err != nil {
		return nil, err
	}
	query := req.URL.Query()

	query, headers = getAliQuery(p.Ident(), query, httpConfig, parameters)
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	req.URL.RawQuery = query.Encode()
	// 记录完整的请求信息
	logger.Debugf("URL: %v, Method: %s, Headers: %+v, params: %+v, Body: %s", req.URL, req.Method, req.Header, query, string(body))

	return req, nil
}
