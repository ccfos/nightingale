package provider

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// TestFeishuAppProviderNotify 从 alert/sender/.env.json 读取飞书应用参数后真实发送。
// 需要：FeishuAppID、FeishuAppSecret、FeishuImGroupID；可选 FeishuContactKey（默认 user_id）、FeishuReceiveIDType（默认与 ContactKey 一致时可填 user_id）。
func TestFeishuAppProviderNotify(t *testing.T) {
	env := readSenderDotEnv(t)
	appID := senderEnvString(env, "FeishuAppID")
	appSecret := senderEnvString(env, "FeishuAppSecret")
	groupID := senderEnvString(env, "FeishuImGroupID")
	contactKey := senderEnvString(env, "FeishuContactKey")
	if contactKey == "" {
		contactKey = "user_id"
	}
	receiveIDType := senderEnvString(env, "FeishuReceiveIDType")
	if receiveIDType == "" {
		receiveIDType = contactKey
	}

	if appID == "" || appSecret == "" || groupID == "" {
		t.Skip("跳过：在 .env.json 填写 FeishuAppID、FeishuAppSecret、FeishuImGroupID")
	}

	cfg := &models.NotifyChannelConfig{
		RequestType: "feishuapp",
		RequestConfig: &models.RequestConfig{
			FeishuAppRequestConfig: &models.FeishuAppRequestConfig{
				AppID:         appID,
				AppSecret:     appSecret,
				ContactKey:    contactKey,
				ReceiveIDType: receiveIDType,
				Timeout:       10000,
				RetryTimes:    1,
				RetrySleep:    10,
			},
			HTTPRequestConfig: &models.HTTPRequestConfig{
				URL:           "https://open.feishu.cn",
				Method:        "POST",
				Headers:       map[string]string{"Content-Type": "application/json"},
				Timeout:       10000,
				RetryTimes:    1,
				RetryInterval: 10,
			},
		},
	}
	client, err := models.GetHTTPClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create HTTP client: %v", err)
	}

	p := &FeishuAppProvider{}
	req := &NotifyRequest{
		Config: cfg,
		Events: []*models.AlertCurEvent{{
			Hash: "hash-test",
			AnnotationsJSON: map[string]string{
				"alert_image_base64": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
			},
		}},
		TplContent: map[string]interface{}{
			"title":   "Test Title",
			"content": "## test markdown content",
		},
		Sendtos:    []string{},
		ImGroupIDs: []string{groupID},
		HttpClient: client,
	}

	result := p.Notify(context.Background(), req)
	if result.Err != nil {
		t.Fatalf("Notify 返回错误: %v", result.Err)
	}
	t.Logf("result: %+v", result)
}
