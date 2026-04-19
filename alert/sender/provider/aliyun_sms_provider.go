package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/logger"
)

const AliyunSmsIdent = "ali-sms"

type AliyunSmsProvider struct{}

func (p *AliyunSmsProvider) Ident() string {
	return AliyunSmsIdent
}

func (p *AliyunSmsProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("aliyun sms provider requires POST method")
	}

	if httpConfig.URL == "" {
		return errors.New("aliyun sms provider requires URL")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("aliyun sms provider requires Content-Type: application/json header")
	}

	if httpConfig.Request.Body == "" && len(httpConfig.Request.Parameters) == 0 {
		return errors.New("aliyun sms provider requires request body or parameters")
	}

	return nil
}

func (p *AliyunSmsProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := p.sendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}

// 从原 NotifyChannelConfig.SendHTTP 提取，供各 HTTP 类 Provider 复用
func (p *AliyunSmsProvider) sendHTTPRequest(httpConfig *models.HTTPRequestConfig, events []*models.AlertCurEvent,
	tpl map[string]interface{}, params map[string]string, sendtos []string,
	client *http.Client) (string, error) {

	if client == nil {
		return "", fmt.Errorf("http client not found")
	}

	if len(events) == 0 {
		return "", fmt.Errorf("events is empty")
	}

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
	body, err := parseRequestBody(httpConfig, fullTpl)
	if err != nil {
		logger.Errorf("failed to parse request body: %v, event: %v", err, events)
		return "", err
	}

	// 替换 URL Header Parameters 中的变量
	url, headers, parameters := replaceVariables(httpConfig, fullTpl)
	logger.Infof("url: %v, headers: %v, parameters: %v", url, headers, parameters)

	// 重试机制
	var lastErrorMessage string
	for i := 0; i < httpConfig.RetryTimes; i++ {
		var resp *http.Response
		req, err := p.makeHTTPRequest(httpConfig, url, headers, parameters, body)
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
			return fmt.Sprintf("status_code:%d, response:%s", resp.StatusCode, string(body)), nil
		}

		return fmt.Sprintf("status_code:%d, response:%s", resp.StatusCode, string(body)), fmt.Errorf("failed to send request, status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return lastErrorMessage, errors.New("all retries failed, last error: " + lastErrorMessage)
}

func (p *AliyunSmsProvider) makeHTTPRequest(httpConfig *models.HTTPRequestConfig, url string, headers map[string]string, parameters map[string]string, body []byte) (*http.Request, error) {
	// 设置签名
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

// getAliQuery 获取阿里云API的查询参数和请求头
func getAliQuery(ident string, query url.Values, httpConfig *models.HTTPRequestConfig, params map[string]string) (url.Values, map[string]string) {
	// 获取基础配置
	ak := httpConfig.Request.Parameters["AccessKeyId"]
	sk := httpConfig.Request.Parameters["AccessKeySecret"]
	// httpConfig := ncc.RequestConfig.HTTPRequestConfig

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
	if ident == "ali-sms" {
		headers["x-acs-action"] = "SendSms"
	} else if ident == "ali-voice" {
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
