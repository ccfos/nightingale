package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

type SlackWebhookProvider struct{}

func (p *SlackWebhookProvider) Ident() string {
	return models.SlackWebhook
}

func (p *SlackWebhookProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("slackwebhook provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("slackwebhook provider requires Content-Type: application/json header")
	}

	if httpConfig.URL == "" {
		return errors.New("slackwebhook provider requires URL (e.g. {{$params.webhook_url}})")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("slackwebhook provider requires request body")
	}

	return nil
}

func (p *SlackWebhookProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}
