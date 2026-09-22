package provider

import "testing"

func TestMaskNotifyTargetKeepsReadableTargets(t *testing.T) {
	for _, target := range []string{
		"#alerts",               // Discord 名称
		"N9E SRE",               // JSM 名称
		"KAN-7,KAN-8",           // Jira 工单号
		"ops@example.com",       // 邮件
		"13800138000",           // 手机号
		"1234567890123456789",   // FlashDuty 频道 ID
		"id=12",                 // 屏蔽规则
		"值班告警群",                 // 中文名称
		"***d2ab",               // JSM 没填名称时的 key 掩码
		"/opt/scripts/alert.sh", // 脚本路径
		"https://discord.com/api/webhooks/123456789012345678/***",
		"http://cmdb.local/api/v1/alerts?source=n9e",
	} {
		if got := MaskNotifyTarget(target); got != target {
			t.Errorf("%q should be shown as is, got %q", target, got)
		}
	}
}

func TestMaskNotifyTargetHidesSecrets(t *testing.T) {
	cases := map[string]string{
		// 钉钉 access_token、PagerDuty routing key、企微 key（UUID）、Telegram bot token
		"8f2a6c1e9b3d4f5a7c0e2b4d6f8a1c3e5b7d9f0a2c4e6b8d0f1a3c5e7b9d2f4a": "***2f4a",
		"a1b2c3d4e5f60718293a4b5c6d7e8f90":                                 "***8f90",
		"6f1d2c3b-4a5e-4f6a-9b8c-7d6e5f4a3b2c":                             "***3b2c",
		"123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw":                     "***Dsaw",
		// 旧版本写入时遮了后 4 位的 token，以及它和机器人名称拼在一起的形态
		"8f2a6c1e9b3d4f5a7c0e2b4d6f8a1c3e5b7d9f0a2c4e6b8d0f1a3c5e7b9d****,#prod-ale****": "***,#prod-ale****",
		// URL：查询参数里的凭证、路径末尾的 token、Telegram 的 bot<token>、userinfo 里的密码
		"https://oapi.dingtalk.com/robot/send?access_token=8f2a6c1e9b3d4f5a7c0e2b4d6f8a1c3e":         "https://oapi.dingtalk.com/robot/send?access_token=***",
		"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=6f1d2c3b-4a5e-4f6a-9b8c-7d6e5f4a3b2c":  "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=***",
		"https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX1":             "https://hooks.slack.com/services/T00000000/B00000000/***",
		"https://open.feishu.cn/open-apis/bot/v2/hook/6f1d2c3b-4a5e-4f6a-9b8c-7d6e5f4a3b2c":          "https://open.feishu.cn/open-apis/bot/v2/hook/***",
		"https://discord.com/api/webhooks/123456789012345678/FakeWebhookToken-ForTests_abc123xyz789": "https://discord.com/api/webhooks/123456789012345678/***",
		"https://api.telegram.org/bot123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw/sendMessage":       "https://api.telegram.org/***/sendMessage",
		"https://alert:s3cret-pass@callback.example.com/n9e":                                         "https://alert@callback.example.com/n9e",
	}
	for in, want := range cases {
		if got := MaskNotifyTarget(in); got != want {
			t.Errorf("MaskNotifyTarget(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestNotifyTargetFromParams(t *testing.T) {
	token := "8f2a6c1e9b3d4f5a7c0e2b4d6f8a1c3e5b7d9f0a2c4e6b8d0f1a3c5e7b9d2f4a"
	cases := []struct {
		params map[string]string
		want   string
	}{
		{map[string]string{"access_token": token, "bot_name": "#prod-alerts"}, "#prod-alerts"},
		{map[string]string{"access_token": token}, "***2f4a"},
		{map[string]string{"key": "6f1d2c3b-4a5e-4f6a-9b8c-7d6e5f4a3b2c", "__test_nonce": "abc"}, "***3b2c"},
		{map[string]string{"callback_url": "https://oapi.dingtalk.com/robot/send?access_token=" + token}, "https://oapi.dingtalk.com/robot/send?access_token=***"},
		{map[string]string{"chat_id": "-1001234567", "token": token}, "-1001234567,***2f4a"},
		{map[string]string{"__test_nonce": "abc"}, ""},
	}
	for _, c := range cases {
		if got := NotifyTargetFromParams(c.params); got != c.want {
			t.Errorf("NotifyTargetFromParams(%v) = %q, want %q", c.params, got, c.want)
		}
	}
}
