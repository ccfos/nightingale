package provider

import (
	"context"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

type DingtalkProvider struct{}

func (p *DingtalkProvider) Ident() string {
	return models.Dingtalk
}

func (p *DingtalkProvider) Check(config *models.NotifyChannelConfig) error {
	return validateSimpleHTTPConfig(p.Ident(), []string{"access_token"}, config)
}

func (p *DingtalkProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	// 内部使用 http_common.SendHTTPRequest 发送
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig

	// 与 feishucard/larkcard 一致：事件里有截图、且确实填了 app_key/app_secret 时，才走钉钉应用接口
	// 上传图片并注入 shot_image_key。只判断结构体非 nil 是不够的：提交上来的 request_config 里可能
	// 带着一份字段全空的 dingtalk_request_config，那会把群机器人通知整条拐进应用模式并失败。
	// 上传链路的任何失败都只告警：可选的截图能力不该把通知本身掐掉。
	imageBase64 := pickImageBase64(req.Events)
	var appKey, appSecret string
	if c := req.Config.RequestConfig.DingtalkRequestConfig; c != nil {
		appKey = strings.TrimSpace(c.AppKey)
		appSecret = strings.TrimSpace(c.AppSecret)
	}
	if imageBase64 != "" && appKey != "" && appSecret != "" {
		accessToken, err := GetAccessToken(ctx, req.HttpClient, appKey, appSecret)
		if err != nil {
			logger.Warningf("get dingtalk access token failed: %s", err.Error())
		}
		if accessToken != "" {
			imageMediaID, err := UploadMedia(ctx, req.HttpClient, accessToken, "image", imageBase64)
			if err != nil {
				logger.Warningf("upload dingtalk image failed: %s", err.Error())
			}
			if imageMediaID != "" {
				if req.CustomParams == nil {
					req.CustomParams = make(map[string]string, 1)
				}
				req.CustomParams["shot_image_key"] = imageMediaID
			}
		}
	}

	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}
