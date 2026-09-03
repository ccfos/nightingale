package provider

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/ccfos/nightingale/v6/models"
)

type WecomProvider struct{}

func (p *WecomProvider) Ident() string {
	return models.Wecom
}

func (p *WecomProvider) Check(config *models.NotifyChannelConfig) error {
	return validateSimpleHTTPConfig(p.Ident(), []string{"key"}, config)
}

func (p *WecomProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	if err != nil {
		return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
	}

	// 告警事件包含截图时，文本发送成功后再补发一条 image 消息。
	imageBase64 := pickImageBase64(req.Events)
	if imageBase64 == "" {
		return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: nil}
	}
	imageResp, imageErr := sendWecomImageMessage(httpConfig, req, imageBase64)
	if imageErr != nil {
		return &NotifyResult{
			Target:   getNotifyTarget(req.CustomParams, req.Sendtos),
			Response: resp + "; image_send: " + imageResp,
			Err:      imageErr,
		}
	}
	return &NotifyResult{
		Target:   getNotifyTarget(req.CustomParams, req.Sendtos),
		Response: resp + "; image_send: " + imageResp,
		Err:      nil,
	}
}

func sendWecomImageMessage(httpConfig *models.HTTPRequestConfig, req *NotifyRequest, imagePayload string) (string, error) {
	raw, err := decodeBase64Payload(imagePayload)
	if err != nil {
		return "", fmt.Errorf("decode wecom image payload failed: %w", err)
	}
	hash := md5.Sum(raw)
	bodyObj := map[string]interface{}{
		"msgtype": "image",
		"image": map[string]string{
			"base64": base64.StdEncoding.EncodeToString(raw),
			"md5":    hex.EncodeToString(hash[:]),
		},
	}
	bodyBytes, err := json.Marshal(bodyObj)
	if err != nil {
		return "", fmt.Errorf("marshal wecom image body failed: %w", err)
	}

	cfg := *httpConfig
	cfg.Request = httpConfig.Request
	cfg.Request.Body = string(bodyBytes)

	return SendHTTPRequest(&cfg, req.Events, req.TplContent, req.CustomParams, req.Sendtos, req.HttpClient)
}
