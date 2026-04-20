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
