package provider

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ccfos/nightingale/v6/models"
)

type WecomProvider struct{}

func (p *WecomProvider) Ident() string {
	return models.Wecom
}

func (p *WecomProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("wecom provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("wecom provider requires Content-Type: application/json header")
	}

	if httpConfig.Request.Parameters == nil || httpConfig.Request.Parameters["key"] == "" {
		return errors.New("wecom provider requires key parameter")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("wecom provider requires request body")
	}

	return nil
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
