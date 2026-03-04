package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

const TencentSmsIdent = "tx-sms"

type TencentSmsProvider struct{}

func (p *TencentSmsProvider) Ident() string {
	return TencentSmsIdent
}

func (p *TencentSmsProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("tencent sms provider requires POST method")
	}

	if httpConfig.URL == "" {
		return errors.New("tencent sms provider requires URL")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("tencent sms provider requires Content-Type: application/json header")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("tencent sms provider requires request body")
	}

	return nil
}

func (p *TencentSmsProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: "todo: todo", Response: resp, Err: err}
}

func (p *TencentSmsProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "Tencent SMS", Ident: TencentSmsIdent, RequestType: "http", Weight: 11, Enable: true,
			RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{
					Method:  "POST",
					URL:     "https://sms.tencentcloudapi.com",
					Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
					Request: models.RequestDetail{
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
			ParamConfig: &models.NotifyParamConfig{
				UserInfo: &models.UserInfo{
					ContactKey: "phone",
				},
			},
		},
	}
}

func setTxHeader(config *models.NotifyChannelConfig, headers map[string]string, payloadBytes []byte) map[string]string {
	timestamp := time.Now().Unix()

	authorization := getTxSignature(config, string(payloadBytes), timestamp)
	headers["X-TC-Timestamp"] = fmt.Sprintf("%d", timestamp)
	headers["Authorization"] = authorization

	return headers
}

func getTxSignature(config *models.NotifyChannelConfig, payloadStr string, timestamp int64) string {
	httpConfig := config.RequestConfig.HTTPRequestConfig

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
