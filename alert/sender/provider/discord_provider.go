package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

type DiscordProvider struct{}

func (p *DiscordProvider) Ident() string {
	return models.Discord
}

func (p *DiscordProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("discord provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("discord provider requires Content-Type: application/json header")
	}

	if httpConfig.URL == "" {
		return errors.New("discord provider requires URL (e.g. {{$params.webhook_url}})")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("discord provider requires request body")
	}

	return nil
}

func (p *DiscordProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: "todo: todo", Response: resp, Err: err}
}

func (p *DiscordProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "Discord", Ident: models.Discord, RequestType: "http", Weight: 16, Enable: false,
			RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{
					URL:    "{{$params.webhook_url}}",
					Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
					Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
					Request: models.RequestDetail{
						Body: `{"content": "{{$tpl.content}}"}`,
					},
				},
			},
			ParamConfig: &models.NotifyParamConfig{
				Custom: models.Params{
					Params: []models.ParamItem{
						{Key: "webhook_url", CName: "Webhook Url", Type: "string"},
					},
				},
			},
		},
	}
}
