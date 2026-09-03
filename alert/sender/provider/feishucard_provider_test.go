package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// TestFeishuCardProviderNotifyWithImage 真实发送飞书卡片并覆盖图片上传场景。
// 需要：FeishuCardAccessToken、FeishuAppID、FeishuAppSecret。
func TestFeishuCardProviderNotifyWithImage(t *testing.T) {
	env := readSenderDotEnv(t)
	cardToken := senderEnvString(env, "FeishuCardAccessToken")
	appID := senderEnvString(env, "FeishuAppID")
	appSecret := senderEnvString(env, "FeishuAppSecret")
	if cardToken == "" || appID == "" || appSecret == "" {
		t.Skip("跳过：在 .env.json 填写 FeishuCardAccessToken、FeishuAppID、FeishuAppSecret")
	}

	cfg := &models.NotifyChannelConfig{
		RequestType: "feishucard",
		RequestConfig: &models.RequestConfig{
			FeishuRequestConfig: &models.FeishuRequestConfig{
				AppID:     appID,
				AppSecret: appSecret,
			},
			HTTPRequestConfig: &models.HTTPRequestConfig{
				URL:           "https://open.feishu.cn/open-apis/bot/v2/hook/{{$params.access_token}}",
				Method:        "POST",
				Headers:       map[string]string{"Content-Type": "application/json"},
				Timeout:       10000,
				RetryTimes:    1,
				RetryInterval: 10,
				Request: models.RequestDetail{
					Body: `{"msg_type":"interactive","card":{"config":{"wide_screen_mode":true},"header":{"title":{"tag":"plain_text","content":"{{$tpl.title}}"},"template":"red"},"elements":[{"tag":"markdown","content":"{{$tpl.content}}"},{"tag":"img","img_key":"{{$params.shot_image_key}}","alt":{"tag":"plain_text","content":"screenshot"}}]}}`,
				},
			},
		},
	}
	client, err := models.GetHTTPClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create HTTP client: %v", err)
	}

	req := &NotifyRequest{
		Config: cfg,
		Events: []*models.AlertCurEvent{{
			Hash: "feishucard-image-test",
			ShotImageBase64: map[string]string{
				"shot_1": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAGQAAABkCAIAAAD/gAIDAAAA80lEQVR4nO3QQQ3AIADAQEA5zhAxxfOwx5ElrYLOs+/4s6UDXjErMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCvwA6d8AZXhrmFoAAAAAElFTkSuQmCC",
			},
		}},
		TplContent: map[string]interface{}{
			"title":   "Feishu Card Image Test",
			"content": "this is a live feishu card image test",
		},
		CustomParams: map[string]string{
			"access_token": cardToken,
		},
		HttpClient: client,
	}

	result := (&FeishuCardProvider{}).Notify(context.Background(), req)
	if result.Err != nil {
		t.Fatalf("Notify 返回错误: %v, response: %s", result.Err, result.Response)
	}
	if !strings.Contains(result.Response, "StatusCode") {
		t.Fatalf("unexpected response: %s", result.Response)
	}
	t.Logf("result: %+v", result)
}
