package provider

import (
	"context"
	"errors"

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
	return &NotifyResult{Target: "todo: todo", Response: resp, Err: err}
}

func (p *WecomProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "Wecom", Ident: models.Wecom, RequestType: "http", Weight: 4, Enable: true,
			RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{
					URL:    "https://qyapi.weixin.qq.com/cgi-bin/webhook/send",
					Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
					Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
					Request: models.RequestDetail{
						Parameters: map[string]string{"key": "{{$params.key}}"},
						Body:       `{"msgtype": "markdown", "markdown": {"content": "{{$tpl.content}}"}}`,
					},
				},
			},
			ParamConfig: &models.NotifyParamConfig{
				Custom: models.Params{
					Params: []models.ParamItem{
						{Key: "key", CName: "Key", Type: "string"},
						{Key: "bot_name", CName: "Bot Name", Type: "string"},
					},
				},
			},
		},
	}
}
