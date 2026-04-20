package provider

import (
	"context"
	"net/http"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

type FeishuCardProvider struct{}

func (p *FeishuCardProvider) Ident() string {
	return models.FeishuCard
}

func (p *FeishuCardProvider) Check(config *models.NotifyChannelConfig) error {
	return validateSimpleHTTPConfig(p.Ident(), nil, config)
}

func (p *FeishuCardProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig

	// 当事件包含截图、且显式提供 app_id/app_secret 时，先上传图片并注入 shot_image_key，供卡片模板引用。
	imageBase64 := pickImageBase64(req.Events)
	appConfig := req.Config.RequestConfig.FeishuRequestConfig

	var appID, appSecret string
	if appConfig != nil {
		appID = strings.TrimSpace(appConfig.AppID)
		appSecret = strings.TrimSpace(appConfig.AppSecret)
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
