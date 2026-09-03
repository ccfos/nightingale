//go:build ignore
// +build ignore

// TODO(dingtalkapp): 钉钉应用本次不上线，测试文件一同 build tag 屏蔽；上线时删除顶部两行即可恢复。

package provider

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// TestDingtalkAppProviderNotify 从 alert/sender/.env.json 读取钉钉应用参数后真实发送。
// 需要：DingtalkAppKey、DingtalkAppSecret、DingtalkRobotCode、Phone（接收手机号，与 ContactKey=phone 一致）。
func TestDingtalkAppProviderNotify(t *testing.T) {
	env := readSenderDotEnv(t)
	appKey := senderEnvString(env, "DingtalkAppKey")
	appSecret := senderEnvString(env, "DingtalkAppSecret")
	robotCode := senderEnvString(env, "DingtalkRobotCode")
	phone := senderEnvString(env, "Phone")

	if appKey == "" || appSecret == "" || robotCode == "" || phone == "" {
		t.Skip("跳过：在 .env.json 填写 DingtalkAppKey、DingtalkAppSecret、DingtalkRobotCode、Phone")
	}

	appCfg := &models.DingtalkAppRequestConfig{
		AppKey:     appKey,
		AppSecret:  appSecret,
		Timeout:    10000,
		RetryTimes: 1,
		RetrySleep: 1,
	}

	p := &DingtalkAppProvider{}
	cfg := &models.NotifyChannelConfig{
		RequestType: "dingtalkapp",
		ParamConfig: &models.NotifyParamConfig{
			UserInfo: &models.UserInfo{ContactKey: "phone"},
		},
		RequestConfig: &models.RequestConfig{
			DingtalkAppRequestConfig: appCfg,
			HTTPRequestConfig: &models.HTTPRequestConfig{
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
				"shot_1": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAGQAAABkCAIAAAD/gAIDAAAA80lEQVR4nO3QQQ3AIADAQEA5zhAxxfOwx5ElrYLOs+/4s6UDXjErMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCswKzArMCvwA6d8AZXhrmFoAAAAAElFTkSuQmCC",
			},
		}},
		TplContent: map[string]interface{}{
			"title":   "test alert",
			"content": "{{ $event.Hash }}\n\n- item 1\n- item 2\n\n`code`",
		},
		Sendtos: []string{phone},
		CustomParams: map[string]string{
			"robot_code":   robotCode,
			"single_title": "查看详情",
			"single_url":   "https://www.dingtalk.com/",
		},
		HttpClient: client,
	}

	result := p.Notify(context.Background(), req)
	t.Logf("result: %+v", result)
}

// TestDingtalkAppProviderNotifyGroup 真实发送群聊机器人消息。
// 需要：DingtalkAppKey、DingtalkAppSecret、DingtalkRobotCode、DingtalkOpenConversationID。
func TestDingtalkAppProviderNotifyGroup(t *testing.T) {
	env := readSenderDotEnv(t)
	appKey := senderEnvString(env, "DingtalkAppKey")
	appSecret := senderEnvString(env, "DingtalkAppSecret")
	robotCode := senderEnvString(env, "DingtalkRobotCode")
	openConversationID := senderEnvString(env, "DingtalkOpenConversationID")

	if appKey == "" || appSecret == "" || robotCode == "" || openConversationID == "" {
		t.Skip("跳过：在 .env.json 填写 DingtalkAppKey、DingtalkAppSecret、DingtalkRobotCode、DingtalkOpenConversationID")
	}

	appCfg := &models.DingtalkAppRequestConfig{
		AppKey:     appKey,
		AppSecret:  appSecret,
		Timeout:    10000,
		RetryTimes: 1,
		RetrySleep: 1,
	}

	p := &DingtalkAppProvider{}
	cfg := &models.NotifyChannelConfig{
		RequestType: "dingtalkapp",
		ParamConfig: &models.NotifyParamConfig{
			UserInfo: &models.UserInfo{ContactKey: "dingtalk_userid"},
		},
		RequestConfig: &models.RequestConfig{
			DingtalkAppRequestConfig: appCfg,
			HTTPRequestConfig: &models.HTTPRequestConfig{
				URL:           "https://api.dingtalk.com",
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
		Config:     cfg,
		Events:     []*models.AlertCurEvent{{Hash: "hash-group-test"}},
		TplContent: map[string]interface{}{"title": "test group alert", "content": "group content from n9e"},
		ImGroupIDs: []string{openConversationID},
		// 群聊 robotCode 在真实路径由 BuildNotifyContext 预取自 dingtalk_group 表，
		// 单测里手工填，模拟酷应用已安装场景。
		ImGroupRobotCodes: map[string]string{openConversationID: robotCode},
		CustomParams: map[string]string{
			"robot_code":   robotCode,
			"single_title": "查看详情",
			"single_url":   "https://www.dingtalk.com/",
		},
		HttpClient: client,
	}

	result := p.Notify(context.Background(), req)
	t.Logf("group result: %+v", result)
}

func TestGetScenarioGroupInfo(t *testing.T) {
	env := readSenderDotEnv(t)
	appKey := senderEnvString(env, "DingtalkAppKey")
	appSecret := senderEnvString(env, "DingtalkAppSecret")
	openConversationID := senderEnvString(env, "DingtalkOpenConversationID")

	if appKey == "" || appSecret == "" || openConversationID == "" {
		t.Skip("跳过：在 .env.json 填写 DingtalkAppKey、DingtalkAppSecret、DingtalkOpenConversationID")
	}
	cfg := &models.NotifyChannelConfig{
		RequestType: "dingtalkapp",
		RequestConfig: &models.RequestConfig{
			DingtalkAppRequestConfig: &models.DingtalkAppRequestConfig{
				AppKey:    appKey,
				AppSecret: appSecret,
			},
		},
	}
	client, err := models.GetHTTPClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create HTTP client: %v", err)
	}
	accessToken, err := GetAccessToken(context.Background(), client, appKey, appSecret)
	if err != nil {
		t.Fatalf("Failed to get access token: %v", err)
	}
	info, err := GetScenarioGroupInfo(context.Background(), client, accessToken, openConversationID)
	if err != nil {
		t.Fatalf("Failed to get scenario group info: %v", err)
	}
	t.Logf("scenario group info: %+v", info)
}
