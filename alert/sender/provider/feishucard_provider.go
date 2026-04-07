package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

type FeishuCardProvider struct {
	appConfig *models.FeishuRequestConfig
}

func (p *FeishuCardProvider) Ident() string {
	return models.FeishuCard
}

func (p *FeishuCardProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("feishu card provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("feishu card provider requires Content-Type: application/json header")
	}

	if httpConfig.URL == "" {
		return errors.New("feishu card provider requires URL (e.g. with {{$params.access_token}})")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("feishu card provider requires request body")
	}

	return nil
}

func (p *FeishuCardProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig

	// 当事件包含截图、且显式提供 app_id/app_secret 时，先上传图片并注入 shot_image_key，供卡片模板引用。
	imageBase64 := pickImageBase64(req.Events)
	p.appConfig = req.Config.RequestConfig.FeishuRequestConfig

	var appID, appSecret string
	if p.appConfig != nil {
		appID = strings.TrimSpace(p.appConfig.AppID)
		appSecret = strings.TrimSpace(p.appConfig.AppSecret)
	}
	if imageBase64 != "" && appID != "" && appSecret != "" {
		token, err := getFeishuTenantAccessToken(ctx, req.HttpClient, appID, appSecret)
		if err != nil {
			logger.Warningf("get feishu tenant access token failed: %s", err.Error())
		} else {
			imageKey, err := uploadFeishuImage(ctx, req.HttpClient, token, imageBase64)
			if err != nil {
				logger.Warningf("upload feishu image failed: %s", err.Error())
			} else {
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

func getFeishuTenantAccessToken(ctx context.Context, client *http.Client, appID, appSecret string) (string, error) {
	return getOpenPlatformTenantAccessToken(ctx, client, appID, appSecret, feishuAppTokenURL)
}

func uploadFeishuImage(ctx context.Context, client *http.Client, token, imageBase64 string) (string, error) {
	return uploadOpenPlatformImage(ctx, client, token, imageBase64, feishuImageURL)
}

func (p *FeishuCardProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "Feishu Card", Ident: models.FeishuCard, RequestType: "http", Weight: 5, Enable: true,
			RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{
					URL:    "https://open.feishu.cn/open-apis/bot/v2/hook/{{$params.access_token}}",
					Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
					Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
					Request: models.RequestDetail{
						Body: `{"msg_type": "interactive", "card": {"config": {"wide_screen_mode": true}, "header": {"title": {"content": "{{$tpl.title}}", "tag": "plain_text"}, "template": "{{if $event.IsRecovered}}green{{else}}red{{end}}"}, "elements": [{"tag": "markdown", "content": "{{$tpl.content}}"}]}}`,
					},
				},
				FeishuRequestConfig: &models.FeishuRequestConfig{
					AppID:     "",
					AppSecret: "",
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
