package dispatch

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

func TestNotifyConfigForLogHidesCredentials(t *testing.T) {
	c := &models.NotifyConfig{ChannelID: 8, TemplateID: 8, Params: map[string]interface{}{
		"webhook_url":  "https://discord.com/api/webhooks/1/secret-token",
		"api_key":      "jsm-key",
		"access_token": "dingtalk-token",
		"bot_name":     "#alerts",
		"user_ids":     []interface{}{1, 2},
	}}

	s := fmt.Sprintf("%+v", notifyConfigForLog(c))
	for _, secret := range []string{"secret-token", "jsm-key", "dingtalk-token"} {
		if strings.Contains(s, secret) {
			t.Fatalf("log line leaks %q: %s", secret, s)
		}
	}
	if !strings.Contains(s, "bot_name:#alerts") || !strings.Contains(s, "ChannelID:8") {
		t.Fatalf("non-sensitive fields should stay readable: %s", s)
	}
	if c.Params["webhook_url"] != "https://discord.com/api/webhooks/1/secret-token" {
		t.Fatalf("the original config must not be modified")
	}
}
