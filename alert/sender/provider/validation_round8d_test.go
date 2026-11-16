package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestR8dNativeMissingVariables(t *testing.T) {
	withUserVariables(t, map[string]string{})
	for _, tc := range []struct {
		name   string
		p      NotifyChannelProvider
		rc     *models.RequestConfig
		params map[string]string
	}{
		{name: "ntfy server", p: &NtfyProvider{}, rc: &models.RequestConfig{NtfyRequestConfig: &models.NtfyRequestConfig{ServerURL: "{{.missing}}"}}, params: map[string]string{"topic": "valid"}},
		{name: "ntfy topic", p: &NtfyProvider{}, rc: &models.RequestConfig{}, params: map[string]string{"topic": "{{.missing}}"}},
		{name: "line token", p: &LineProvider{}, rc: &models.RequestConfig{LineRequestConfig: &models.LineRequestConfig{ChannelAccessToken: "{{.missing}}"}}, params: map[string]string{"to": "valid"}},
		{name: "line to", p: &LineProvider{}, rc: &models.RequestConfig{LineRequestConfig: &models.LineRequestConfig{ChannelAccessToken: "secret"}}, params: map[string]string{"to": "{{.missing}}"}},
		{name: "pushover token", p: &PushoverProvider{}, rc: &models.RequestConfig{PushoverRequestConfig: &models.PushoverRequestConfig{Token: "{{.missing}}"}}, params: map[string]string{"user_key": "valid"}},
		{name: "pushover key", p: &PushoverProvider{}, rc: &models.RequestConfig{PushoverRequestConfig: &models.PushoverRequestConfig{Token: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}, params: map[string]string{"user_key": "{{.missing}}"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := newCaptureServer(t, always(200, `{}`))
			req := webhookReq(tc.p.Ident(), tc.rc, tc.params)
			req.HttpClient = srv.Client()
			res := tc.p.Notify(context.Background(), req)
			require.ErrorContains(t, res.Err, "missing")
			require.Empty(t, got())
		})
	}
}
func TestR8dFeishuLookupValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*http.Client, string) error
	}{
		{"chats", func(c *http.Client, tok string) error {
			_, e := SearchFeishuVisibleChats(context.Background(), c, tok, "ops", 0, "", "")
			return e
		}},
		{"users", func(c *http.Client, tok string) error {
			_, e := GetFeishuUserID(context.Background(), c, tok, []string{"user@example.test"}, nil, false, "")
			return e
		}},
		{"card", func(c *http.Client, tok string) error {
			_, e := SendFeishuCardMessage(context.Background(), c, tok, "open_id", "user", `{}`)
			return e
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.call(nil, "token"))
			if tc.name != "card" {
				require.Error(t, tc.call(&http.Client{}, ""))
			}
			for _, body := range []string{`not json`, `{"code":400,"msg":"denied"}`} {
				srv, got := newCaptureServer(t, always(200, body))
				localFeishuEndpoints(t, srv.URL)
				require.Error(t, tc.call(srv.Client(), "token"))
				require.Len(t, got(), 1)
				require.Equal(t, "Bearer token", got()[0].Header.Get("Authorization"))
			}
			for _, broken := range []bool{false, true} {
				client := &http.Client{Transport: flashDutyTestTransport(func(r *http.Request) (*http.Response, error) {
					if broken {
						return nil, context.Canceled
					}
					return &http.Response{StatusCode: 200, Body: r8dBrokenBody{}, Header: make(http.Header), Request: r}, nil
				})}
				require.Error(t, tc.call(client, "token"))
			}
		})
	}
}
