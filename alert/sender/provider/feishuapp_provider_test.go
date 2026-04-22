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
		ParamConfig: &models.NotifyParamConfig{
			UserInfo: &models.UserInfo{ContactKey: contactKey},
		},
		RequestConfig: &models.RequestConfig{
			FeishuAppRequestConfig: &models.FeishuAppRequestConfig{
				AppID:         appID,
				AppSecret:     appSecret,
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
			ShotImageBase64: map[string]string{
				"shot_1": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
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

// Notify 在 ParamConfig 为 nil、只配了 ReceiveIDType 时不应 panic。
// 通过 HttpClient=nil 让 token 获取立刻返回 error，避免真实网络请求；
// 本用例的目的是验证个人发送路径不会因空指针崩溃。
func TestFeishuAppProvider_Notify_NilParamConfig_NoPanic(t *testing.T) {
	p := &FeishuAppProvider{}
	req := &NotifyRequest{
		Config: &models.NotifyChannelConfig{
			RequestConfig: &models.RequestConfig{
				FeishuAppRequestConfig: &models.FeishuAppRequestConfig{
					AppID:         "app",
					AppSecret:     "secret",
					ReceiveIDType: "email",
				},
			},
		},
		Sendtos:    []string{"a@b.com"},
		HttpClient: nil,
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Notify panicked: %v", r)
		}
	}()
	res := p.Notify(context.Background(), req)
	if res == nil || res.Err == nil {
		t.Fatalf("expected error result (nil http client), got %+v", res)
	}
}

// 仅群发（ImGroupIDs 非空、Sendtos 为空、ParamConfig 为 nil）时也不应 panic。
func TestFeishuAppProvider_Notify_GroupOnly_NilParamConfig_NoPanic(t *testing.T) {
	p := &FeishuAppProvider{}
	req := &NotifyRequest{
		Config: &models.NotifyChannelConfig{
			RequestConfig: &models.RequestConfig{
				FeishuAppRequestConfig: &models.FeishuAppRequestConfig{
					AppID:     "app",
					AppSecret: "secret",
				},
			},
		},
		ImGroupIDs: []string{"oc_xxx"},
		HttpClient: nil,
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Notify panicked: %v", r)
		}
	}()
	_ = p.Notify(context.Background(), req)
}
