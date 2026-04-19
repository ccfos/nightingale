package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

type TelegramProvider struct{}

func (p *TelegramProvider) Ident() string {
	return models.Telegram
}

func (p *TelegramProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("telegram provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("telegram provider requires Content-Type: application/json header")
	}

	if httpConfig.URL == "" {
		return errors.New("telegram provider requires URL (e.g. with {{$params.token}})")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("telegram provider requires request body")
	}

	return nil
}

func (p *TelegramProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}
