package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

type MattermostBotProvider struct{}

func (p *MattermostBotProvider) Ident() string {
	return models.MattermostBot
}

func (p *MattermostBotProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("mattermostbot provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("mattermostbot provider requires Content-Type: application/json header")
	}

	if httpConfig.URL == "" {
		return errors.New("mattermostbot provider requires URL")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("mattermostbot provider requires request body")
	}

	return nil
}

func (p *MattermostBotProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: "todo: todo", Response: resp, Err: err}
}

func (p *MattermostBotProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "MattermostBot", Ident: models.MattermostBot, RequestType: "http", Weight: 14, Enable: false,
			RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{
					URL:    "<your mattermost url>/api/v4/posts",
					Method: "POST", Headers: map[string]string{"Content-Type": "application/json", "Authorization": "Bearer <you mattermost bot token>"},
					Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
					Request: models.RequestDetail{
						Body: `{"channel_id": "{{$params.channel_id}}", "message":  "{{$tpl.content}}"}`,
					},
				},
			},
			ParamConfig: &models.NotifyParamConfig{
				Custom: models.Params{
					Params: []models.ParamItem{
						{Key: "channel_id", CName: "Channel ID", Type: "string"},
						{Key: "channel_name", CName: "Channel Name", Type: "string"},
					},
				},
			},
		},
	}
}
