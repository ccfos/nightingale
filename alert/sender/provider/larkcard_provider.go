package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

type LarkCardProvider struct {
}

const (
	larkAppTokenURL = "https://open.larksuite.com/open-apis/auth/v3/tenant_access_token/internal"
	larkImageURL    = "https://open.larksuite.com/open-apis/im/v1/images"
)

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

	// 与 feishucard 一致：事件里有截图，且传入 app_id/app_secret 时，先上传并注入 shot_image_key。
	imageBase64 := pickImageBase64(req.Events)
	var appID, appSecret string
	if req.Config.RequestConfig.FeishuRequestConfig != nil {
		appID = strings.TrimSpace(req.Config.RequestConfig.FeishuRequestConfig.AppID)
		appSecret = strings.TrimSpace(req.Config.RequestConfig.FeishuRequestConfig.AppSecret)
	}
	if imageBase64 != "" && appID != "" && appSecret != "" {
		token, err := getLarkTenantAccessToken(ctx, req.HttpClient, appID, appSecret)
		if err != nil {
			logger.Warningf("get lark tenant access token failed: %s", err.Error())
		}
		if token != "" {
			imageKey, err := uploadLarkImage(ctx, req.HttpClient, token, imageBase64)
			if err != nil {
				logger.Warningf("upload lark image failed: %s", err.Error())
			}
			if imageKey != "" {
				if req.CustomParams == nil {
					req.CustomParams = make(map[string]string, 1)
				}
				req.CustomParams["shot_image_key"] = imageKey
			}
		}
	}

	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}

func getLarkTenantAccessToken(ctx context.Context, client *http.Client, appID, appSecret string) (string, error) {
	return getOpenPlatformTenantAccessToken(ctx, client, appID, appSecret, larkAppTokenURL)
}

func uploadLarkImage(ctx context.Context, client *http.Client, token, imageBase64 string) (string, error) {
	return uploadOpenPlatformImage(ctx, client, token, imageBase64, larkImageURL)
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
