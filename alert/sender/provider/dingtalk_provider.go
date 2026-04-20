package provider

import (
	"context"

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
	c := req.Config.RequestConfig.DingtalkRequestConfig
	if c != nil {
		accessToken, err := GetAccessToken(ctx, req.HttpClient, c.AppKey, c.AppSecret)
		if err != nil {
			return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: "", Err: err}
		} else {
			// 上传钉钉图片
			imageBase64 := pickImageBase64(req.Events)
			if imageBase64 != "" {
				imageMediaID, err := UploadMedia(ctx, req.HttpClient, accessToken, "image", imageBase64)
				if err != nil {
					logger.Warningf("upload dingtalk image failed: %s", err.Error())
				} else {
					if req.CustomParams == nil {
						req.CustomParams = make(map[string]string, 1)
					}
					req.CustomParams["shot_image_key"] = imageMediaID
				}
			}
		}
	}

	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}

