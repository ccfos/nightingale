package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

type SlackBotProvider struct{}

func (p *SlackBotProvider) Ident() string {
	return models.SlackBot
}

func (p *SlackBotProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("slackbot provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("slackbot provider requires Content-Type: application/json header")
	}

	if httpConfig.URL == "" {
		return errors.New("slackbot provider requires URL")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("slackbot provider requires request body")
	}

	return nil
}

func (p *SlackBotProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: "todo: todo", Response: resp, Err: err}
}

func (p *SlackBotProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "SlackBot", Ident: models.SlackBot, RequestType: "http", Weight: 12, Enable: false,
			RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{
					URL:    "https://slack.com/api/chat.postMessage",
					Method: "POST", Headers: map[string]string{"Content-Type": "application/json", "Authorization": "Bearer <you slack bot token>"},
					Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
					Request: models.RequestDetail{
						Body: `{"channel": "#{{$params.channel}}", "text":  "{{$tpl.content}}", "mrkdwn": true}`,
					},
				},
			},
			ParamConfig: &models.NotifyParamConfig{
				Custom: models.Params{
					Params: []models.ParamItem{
						{Key: "channel", CName: "channel", Type: "string"},
						{Key: "channel_name", CName: "Channel Name", Type: "string"},
					},
				},
			},
		},
	}
}
