package provider

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// TestDingtalkAppProviderNotify 从 alert/sender/.env.json 读取钉钉应用参数后真实发送。
// 需要：DingtalkAppKey、DingtalkAppSecret、DingtalkCardTemplateId、Phone（接收手机号，与 ContactKey=phone 一致）。
func TestDingtalkAppProviderNotify(t *testing.T) {
	env := readSenderDotEnv(t)
	appKey := senderEnvString(env, "DingtalkAppKey")
	appSecret := senderEnvString(env, "DingtalkAppSecret")
	cardTpl := senderEnvString(env, "DingtalkCardTemplateId")
	phone := senderEnvString(env, "Phone")

	if appKey == "" || appSecret == "" || cardTpl == "" || phone == "" {
		t.Skip("跳过：在 .env.json 填写 DingtalkAppKey、DingtalkAppSecret、DingtalkCardTemplateId、Phone")
	}

	appCfg := &models.DingtalkAppRequestConfig{
		AppKey:     appKey,
		AppSecret:  appSecret,
		ContactKey: "phone",
		Timeout:    10000,
		RetryTimes: 1,
		RetrySleep: 1,
	}

	p := &DingtalkAppProvider{}
	cfg := &models.NotifyChannelConfig{
		RequestType: "dingtalkapp",
		RequestConfig: &models.RequestConfig{
			DingtalkAppRequestConfig: appCfg,
			HTTPRequestConfig: &models.HTTPRequestConfig{
				URL:           "https://oapi.dingtalk.com",
				Method:        "POST",
				Headers:       map[string]string{"Content-Type": "application/json"},
				Timeout:       10000,
				RetryTimes:    1,
				RetryInterval: 1,
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
			Hash: "hash-test",
			ShotImageBase64: map[string]string{
				"shot_1": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
			},
		}},
		TplContent: map[string]interface{}{
			"title":   "test alert",
			"content": "{{ $event.Hash }}\n\n- item 1\n- item 2\n\n`code`",
		},
		Sendtos: []string{phone},
		CustomParams: map[string]string{
			"card_template_id": cardTpl,
		},
		HttpClient: client,
	}

	result := p.Notify(context.Background(), req)
	t.Logf("result: %+v", result)
}
