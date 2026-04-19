package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

// LarkProvider 飞书国际版 (Lark) 机器人 (text) 通道，保留旧行为：msg_type=text。
// 卡片形态由 LarkCardProvider 处理。
type LarkProvider struct{}

func (p *LarkProvider) Ident() string {
	return models.Lark
}

func (p *LarkProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("lark provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("lark provider requires Content-Type: application/json header")
	}

	if httpConfig.URL == "" {
		return errors.New("lark provider requires URL (e.g. with {{$params.token}})")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("lark provider requires request body")
	}

	return nil
}

func (p *LarkProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}
