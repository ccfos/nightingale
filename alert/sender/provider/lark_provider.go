package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

// LarkProvider 飞书国际版 (Lark) 机器人 (text) 通道，保留旧行为：msg_type=text。
// 卡片形态由 LarkCardProvider 处理。
type LarkProvider struct{}

func (p *LarkProvider) Ident() string {
	return models.Lark
}

func (p *LarkProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("lark provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("lark provider requires Content-Type: application/json header")
	}

	if httpConfig.URL == "" {
		return errors.New("lark provider requires URL (e.g. with {{$params.token}})")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("lark provider requires request body")
	}

	return nil
}

func (p *LarkProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}

func (p *LarkProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "Lark", Ident: models.Lark, RequestType: "http", Weight: 6, Enable: true,
			RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{
					URL:    "https://open.larksuite.com/open-apis/bot/v2/hook/{{$params.token}}",
					Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
					Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
					Request: models.RequestDetail{
						Body: `{"msg_type": "text", "content": {"text": "{{$tpl.content}}"}}`,
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
