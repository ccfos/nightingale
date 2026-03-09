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
	return &NotifyResult{Target: "todo: todo", Response: resp, Err: err}
}

func (p *TelegramProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "Telegram", Ident: models.Telegram, RequestType: "http", Weight: 7, Enable: true,
			RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{
					URL:    "https://api.telegram.org/bot{{$params.token}}/sendMessage",
					Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
					Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
					Request: models.RequestDetail{
						Parameters: map[string]string{"chat_id": "{{$params.chat_id}}"},
						Body:       `{"text":"{{$tpl.content}}","parse_mode": "HTML"}`,
					},
				},
			},
			ParamConfig: &models.NotifyParamConfig{
				Custom: models.Params{
					Params: []models.ParamItem{
						{Key: "token", CName: "Token", Type: "string"},
						{Key: "chat_id", CName: "Chat Id", Type: "string"},
						{Key: "bot_name", CName: "Bot Name", Type: "string"},
					},
				},
			},
		},
	}
}
