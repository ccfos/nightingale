package provider

import (
	"context"
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
	return validateSimpleHTTPConfig(p.Ident(), nil, config)
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
