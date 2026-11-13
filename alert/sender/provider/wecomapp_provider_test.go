package provider

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

func TestTruncateWecomMarkdown(t *testing.T) {
	s := strings.Repeat("a", wecomMarkdownMaxLen+10)
	out := truncateWecomMarkdown(s)
	if len(out) > wecomMarkdownMaxLen {
		t.Fatalf("len=%d want <= %d", len(out), wecomMarkdownMaxLen)
	}
}

func TestWecomAppProviderCheck(t *testing.T) {
	p := &WecomAppProvider{}
	base := &models.NotifyChannelConfig{
		RequestType: "wecomapp",
		ParamConfig: &models.NotifyParamConfig{
			UserInfo: &models.UserInfo{ContactKey: "wecom_userid"},
		},
		RequestConfig: &models.RequestConfig{
			WecomAppRequestConfig: &models.WecomAppRequestConfig{
				CorpID:     "c",
				CorpSecret: "s",
				AgentID:    1,
			},
		},
	}
	if err := p.Check(cloneWecomCfg(base)); err != nil {
		t.Fatalf("valid: %v", err)
	}
	noAgent := cloneWecomCfg(base)
	noAgent.RequestConfig.WecomAppRequestConfig.AgentID = 0
	if err := p.Check(noAgent); err == nil {
		t.Fatal("want error for agent_id=0")
	}
	wrongType := cloneWecomCfg(base)
	wrongType.RequestType = "http"
	if err := p.Check(wrongType); err == nil {
		t.Fatal("want error for request_type=http")
	}
}

func TestIsEmailContactKey(t *testing.T) {
	if !isEmailContactKey("email") {
		t.Fatal("email should be recognized")
	}
	if !isEmailContactKey("user_email") {
		t.Fatal("user_email should be recognized")
	}
	if isEmailContactKey("wecom_userid") {
		t.Fatal("wecom_userid should not match email key")
	}
}

func cloneWecomCfg(src *models.NotifyChannelConfig) *models.NotifyChannelConfig {
	c := *src
	rc := *src.RequestConfig
	wc := *src.RequestConfig.WecomAppRequestConfig
	rc.WecomAppRequestConfig = &wc
	c.RequestConfig = &rc
	return &c
}

// TestWecomAppProviderNotifyLive 真实发送企业微信应用消息；账号从 alert/sender/.env.json 读取。
// 需要键：WecomCorpID、WecomCorpSecret、WecomAgentID、WecomSendto；可选 WecomContactKey（默认 wecom_userid）。
func TestWecomAppProviderNotifyLive(t *testing.T) {
	env := readSenderDotEnv(t)
	corpID := senderEnvString(env, "WecomCorpID")
	secret := senderEnvString(env, "WecomCorpSecret")
	agentStr := senderEnvString(env, "WecomAgentID")
	sendto := senderEnvString(env, "WecomSendto")
	contactKey := senderEnvString(env, "WecomContactKey")
	if contactKey == "" {
		contactKey = "wecom_userid"
	}

	if corpID == "" || secret == "" || agentStr == "" || sendto == "" {
		t.Skip("跳过：在 alert/sender/.env.json 中填写 WecomCorpID、WecomCorpSecret、WecomAgentID、WecomSendto")
	}

	agentID, err := strconv.Atoi(agentStr)
	if err != nil || agentID <= 0 {
		t.Fatalf("WecomAgentID 无效: %q", agentStr)
	}

	cfg := &models.NotifyChannelConfig{
		RequestType: "wecomapp",
		ParamConfig: &models.NotifyParamConfig{
			UserInfo: &models.UserInfo{ContactKey: contactKey},
		},
		RequestConfig: &models.RequestConfig{
			HTTPRequestConfig: &models.HTTPRequestConfig{
				URL:           "https://qyapi.weixin.qq.com",
				Method:        "POST",
				Headers:       map[string]string{"Content-Type": "application/json"},
				Timeout:       15000,
				RetryTimes:    1,
				RetryInterval: 100,
			},
			WecomAppRequestConfig: &models.WecomAppRequestConfig{
				CorpID:     corpID,
				CorpSecret: secret,
				AgentID:    agentID,
				Timeout:    15000,
				RetryTimes: 2,
				RetrySleep: 500,
			},
		},
	}

	client, err := models.GetHTTPClient(cfg)
	if err != nil {
		t.Fatalf("GetHTTPClient: %v", err)
	}

	p := &WecomAppProvider{}
	req := &NotifyRequest{
		Config: cfg,
		Events: []*models.AlertCurEvent{{
			Hash: "wecomapp-live-test-" + strconv.FormatInt(time.Now().Unix(), 10),
		}},
		TplContent: map[string]interface{}{
			"title":   "Nightingale WecomApp 集成测试",
			"content": "本条由 `TestWecomAppProviderNotifyLive` 发送，时间：" + time.Now().Format(time.RFC3339),
		},
		Sendtos:    []string{sendto},
		HttpClient: client,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res := p.Notify(ctx, req)
	if res.Err != nil {
		t.Fatalf("Notify 失败: %v, response=%s", res.Err, res.Response)
	}
	t.Logf("target=%s response=%s", res.Target, res.Response)
}
