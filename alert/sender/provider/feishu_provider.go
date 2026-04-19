package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

// FeishuProvider 飞书机器人 (text) 通道，保留旧行为：msg_type=text。
// 卡片形态由 FeishuCardProvider 处理。
type FeishuProvider struct{}

func (p *FeishuProvider) Ident() string {
	return models.Feishu
}

func (p *FeishuProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("feishu provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("feishu provider requires Content-Type: application/json header")
	}

	if httpConfig.URL == "" {
		return errors.New("feishu provider requires URL (e.g. with {{$params.access_token}})")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("feishu provider requires request body")
	}

	return nil
}

func (p *FeishuProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}
