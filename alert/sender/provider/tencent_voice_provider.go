package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

const TencentVoiceIdent = "tx-voice"

type TencentVoiceProvider struct{}

func (p *TencentVoiceProvider) Ident() string {
	return TencentVoiceIdent
}

func (p *TencentVoiceProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("tencent voice provider requires POST method")
	}

	if httpConfig.URL == "" {
		return errors.New("tencent voice provider requires URL")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("tencent voice provider requires Content-Type: application/json header")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("tencent voice provider requires request body")
	}

	return nil
}

func (p *TencentVoiceProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: "todo: todo", Response: resp, Err: err}
}

func (p *TencentVoiceProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "Tencent Voice", Ident: TencentVoiceIdent, RequestType: "http", Weight: 10, Enable: true,
			RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{
					Method:  "POST",
					URL:     "https://vms.tencentcloudapi.com",
					Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
					Request: models.RequestDetail{
						Body: `{"CalledNumber":"+86{{ $sendto }}","TemplateId":"需要改为实际的模板id","TemplateParamSet":["{{$tpl.content}}"],"VoiceSdkAppid":"需要改为实际的appid"}`,
					},
					Headers: map[string]string{
						"Content-Type": "application/json",
						"Host":         "vms.tencentcloudapi.com",
						"X-TC-Action":  "SendTtsVoice",
						"X-TC-Version": "2020-09-02",
						"X-TC-Region":  "ap-beijing",
						"Service":      "vms",
						"Secret_ID":    "需要改为实际的secret_id",
						"Secret_Key":   "需要改为实际的secret_key",
					},
				},
			},
			ParamConfig: &models.NotifyParamConfig{
				UserInfo: &models.UserInfo{
					ContactKey: "phone",
				},
			},
		},
	}
}
