package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

type CallbackProvider struct{}

func (p *CallbackProvider) Ident() string { return "callback" }

func (p *CallbackProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}
	httpConfig := config.RequestConfig.HTTPRequestConfig
	if httpConfig.URL == "" {
		return errors.New("callback provider requires URL")
	}
	return nil
}

func (p *CallbackProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}
