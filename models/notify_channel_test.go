package models

import (
	"testing"
	"time"
)

func TestScriptProxyTimeout(t *testing.T) {
	for _, tc := range []struct {
		name        string
		proxy       *ScriptProxy
		httpTimeout int
		want        time.Duration
	}{
		{name: "seconds without proxy URL", proxy: &ScriptProxy{Timeout: 2}, want: 2 * time.Second},
		{name: "seconds with proxy URL", proxy: &ScriptProxy{Url: "http://localhost:8080", Timeout: 3}, want: 3 * time.Second},
		{name: "overrides HTTP timeout", proxy: &ScriptProxy{Timeout: 2}, httpTimeout: 500, want: 2 * time.Second},
		{name: "zero preserves HTTP timeout", proxy: &ScriptProxy{}, httpTimeout: 500, want: 500 * time.Millisecond},
		{name: "negative preserves default", proxy: &ScriptProxy{Timeout: -1}, want: 10 * time.Second},
		{name: "unset preserves default", want: 10 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &NotifyChannelConfig{RequestType: "script", RequestConfig: &RequestConfig{
				ScriptRequestConfig: &ScriptRequestConfig{Proxy: tc.proxy},
			}}
			if tc.httpTimeout > 0 {
				config.RequestConfig.HTTPRequestConfig = &HTTPRequestConfig{Timeout: tc.httpTimeout}
			}
			client, err := GetHTTPClient(config)
			if err != nil {
				t.Fatal(err)
			}
			defer client.CloseIdleConnections()
			if client.Timeout != tc.want {
				t.Fatalf("timeout = %v, want %v", client.Timeout, tc.want)
			}
		})
	}
}
