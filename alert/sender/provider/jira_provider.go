package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

type JiraProvider struct{}

func (p *JiraProvider) Ident() string {
	return models.Jira
}

func (p *JiraProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("jira provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("jira provider requires Content-Type: application/json header")
	}

	if httpConfig.URL == "" {
		return errors.New("jira provider requires URL")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("jira provider requires request body")
	}

	return nil
}

func (p *JiraProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}
