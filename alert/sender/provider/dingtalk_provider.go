package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

type DingtalkProvider struct{}

func (p *DingtalkProvider) Ident() string {
	return "dingtalk"
}

func (p *DingtalkProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("dingtalk provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("dingtalk provider requires Content-Type: application/json header")
	}

	if httpConfig.Request.Parameters == nil || httpConfig.Request.Parameters["access_token"] == "" {
		return errors.New("dingtalk provider requires access_token parameter")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("dingtalk provider requires request body")
	}

	return nil
}

func (p *DingtalkProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	// 内部使用 http_common.SendHTTPRequest 发送
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: "todo: todo", Response: resp, Err: err}
}

func (p *DingtalkProvider) DefaultChannels() []*models.NotifyChannelConfig {

	return []*models.NotifyChannelConfig{
		{
			Name: "Dingtalk", Ident: models.Dingtalk, RequestType: "http", Weight: 3, Enable: true,
			RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{
					URL: "https://oapi.dingtalk.com/robot/send", Method: "POST",
					Headers: map[string]string{"Content-Type": "application/json"},
					Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
					Request: models.RequestDetail{
						Parameters: map[string]string{"access_token": "{{$params.access_token}}"},
						Body:       `{"msgtype": "markdown", "markdown": {"title": "{{$tpl.title}}", "text": "{{$tpl.content}}\n{{batchContactsAts $sendtos}}"}, "at": {"atMobiles": {{batchContactsJsonMarshal $sendtos}} }}`,
					},
				},
			},
			ParamConfig: &models.NotifyParamConfig{
				Custom: models.Params{
					Params: []models.ParamItem{
						{Key: "access_token", CName: "Access Token", Type: "string"},
						{Key: "bot_name", CName: "Bot Name", Type: "string"},
					},
				},
			},
		},
	}
}
