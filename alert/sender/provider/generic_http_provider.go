package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

type GenericHTTPProvider struct{}

func (p *GenericHTTPProvider) Ident() string { return "callback" }

func (p *GenericHTTPProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}
	httpConfig := config.RequestConfig.HTTPRequestConfig
	if httpConfig.URL == "" {
		return errors.New("callback provider requires URL")
	}
	return nil
}

func (p *GenericHTTPProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}

func getNotifyTarget(customParams map[string]string, sendtos []string) string {
	if len(customParams) > 0 {
		if u, ok := customParams["callback_url"]; ok && u != "" {
			return u
		}
	}
	if len(sendtos) > 0 {
		return sendtos[0]
	}
	return ""
}

func (p *GenericHTTPProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "Callback", Ident: "callback", RequestType: "http", Weight: 2, Enable: true,
			RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{
					URL:    "{{$params.callback_url}}",
					Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
					Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
					Request: models.RequestDetail{
						Body: `{{ jsonMarshal $events }}`,
					},
				},
			},
			ParamConfig: &models.NotifyParamConfig{
				Custom: models.Params{
					Params: []models.ParamItem{
						{Key: "callback_url", CName: "Callback Url", Type: "string"},
						{Key: "note", CName: "Note", Type: "string"},
					},
				},
			},
		},
	}
}
