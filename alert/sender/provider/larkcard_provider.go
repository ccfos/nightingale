package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

type LarkCardProvider struct{}

func (p *LarkCardProvider) Ident() string {
	return models.LarkCard
}

func (p *LarkCardProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("lark card provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("lark card provider requires Content-Type: application/json header")
	}

	if httpConfig.URL == "" {
		return errors.New("lark card provider requires URL (e.g. with {{$params.access_token}})")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("lark card provider requires request body")
	}

	return nil
}

func (p *LarkCardProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: "todo: todo", Response: resp, Err: err}
}

func (p *LarkCardProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "Lark Card", Ident: models.LarkCard, RequestType: "http", Weight: 6, Enable: true,
			RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{
					URL:    "https://open.larksuite.com/open-apis/bot/v2/hook/{{$params.token}}",
					Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
					Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
					Request: models.RequestDetail{
						Body: `{"msg_type": "interactive", "card": {"config": {"wide_screen_mode": true}, "header": {"title": {"content": "{{$tpl.title}}", "tag": "plain_text"}, "template": "{{if $event.IsRecovered}}green{{else}}red{{end}}"}, "elements": [{"tag": "markdown", "content": "{{$tpl.content}}"}]}}`,
					},
				},
			},
			ParamConfig: &models.NotifyParamConfig{
				Custom: models.Params{
					Params: []models.ParamItem{
						{Key: "token", CName: "Token", Type: "string"},
						{Key: "bot_name", CName: "Bot Name", Type: "string"},
					},
				},
			},
		},
	}
}
