package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

type JsmProvider struct{}

func (p *JsmProvider) Ident() string {
	return models.JSMAlert // "jsm_alert"
}

func (p *JsmProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("jsm provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("jsm provider requires Content-Type: application/json header")
	}

	if httpConfig.URL == "" {
		return errors.New("jsm provider requires URL")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("jsm provider requires request body")
	}

	return nil
}

func (p *JsmProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}
