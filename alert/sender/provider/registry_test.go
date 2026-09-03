package provider

import (
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// init.go 在包初始化时已经把 callback/script/email/flashduty/pagerduty
// 等通用 provider 注册到 DefaultRegistry，这里直接复用。
func TestVerifyChannelConfig(t *testing.T) {
	validHTTP := &models.NotifyChannelConfig{
		RequestConfig: &models.RequestConfig{
			HTTPRequestConfig: &models.HTTPRequestConfig{
				URL:    "https://example.com/hook",
				Method: "POST",
			},
		},
	}

	cases := []struct {
		name       string
		ident      string
		reqType    string
		wantErr    bool
		errContain string
	}{
		{
			name:    "registered ident callback",
			ident:   "callback",
			reqType: "http",
		},
		{
			name:    "custom ident falls back to callback by request_type=http",
			ident:   "my-webhook",
			reqType: "http",
		},
		{
			name:       "unknown request_type rejected",
			ident:      "my-webhook",
			reqType:    "frobnicate",
			wantErr:    true,
			errContain: "unsupported channel",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := *validHTTP
			cfg.Ident = tc.ident
			cfg.RequestType = tc.reqType

			err := VerifyChannelConfig(&cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errContain != "" && !strings.Contains(err.Error(), tc.errContain) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.errContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestVerifyChannelConfig_Nil(t *testing.T) {
	if err := VerifyChannelConfig(nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

// 「测试尚未保存的媒介配置」这条路径把一个 ID 为 0、从未落库的 config 直接送进投递链路。
// Resolve 一旦改成按 ID 查缓存，该功能会在运行时静默失效（Resolve 返回 !ok，
// 报的是 "unknown channel ident"，与真正的配置错误无法区分），因此把不变量钉在这里。
func TestResolveWorksOnUnsavedConfig(t *testing.T) {
	cases := []struct {
		name    string
		ident   string
		reqType string
	}{
		{name: "callback by ident", ident: "callback", reqType: "http"},
		{name: "custom ident falls back by request_type", ident: "my-webhook", reqType: "http"},
		{name: "smtp", ident: "email", reqType: "smtp"},
		{name: "flashduty", ident: "flashduty", reqType: "flashduty"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ncc := &models.NotifyChannelConfig{
				// ID 刻意留零值：这正是未保存配置的形态
				Ident:       tc.ident,
				RequestType: tc.reqType,
				RequestConfig: &models.RequestConfig{
					HTTPRequestConfig: &models.HTTPRequestConfig{URL: "https://example.com/hook", Method: "POST"},
				},
			}

			if ncc.ID != 0 {
				t.Fatalf("precondition failed: ID = %d, want 0", ncc.ID)
			}
			if _, ok := DefaultRegistry.Resolve(ncc); !ok {
				t.Fatalf("Resolve failed for unsaved config ident=%s request_type=%s", tc.ident, tc.reqType)
			}
		})
	}
}
