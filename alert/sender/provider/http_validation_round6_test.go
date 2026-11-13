package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestSimpleHTTPProvider_ChecksWebhookContract(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*models.NotifyChannelConfig)
		want   string
	}{
		{name: "valid", mutate: func(*models.NotifyChannelConfig) {}},
		{
			name: "wrong method", want: "requires POST method",
			mutate: func(c *models.NotifyChannelConfig) { c.RequestConfig.HTTPRequestConfig.Method = http.MethodGet },
		},
		{
			name: "missing content type", want: "Content-Type: application/json",
			mutate: func(c *models.NotifyChannelConfig) { c.RequestConfig.HTTPRequestConfig.Headers = nil },
		},
		{
			name: "missing URL", want: "URL cannot be empty",
			mutate: func(c *models.NotifyChannelConfig) { c.RequestConfig.HTTPRequestConfig.URL = "" },
		},
		{
			name: "missing body", want: "requires request body",
			mutate: func(c *models.NotifyChannelConfig) { c.RequestConfig.HTTPRequestConfig.Request.Body = "" },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := tencentRound6Request("http://127.0.0.1").Config
			tc.mutate(config)

			err := (&simpleHTTPProvider{ident: "test-webhook"}).Check(config)

			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
}

func TestWebhookValidation_RequiresNamedParameter(t *testing.T) {
	config := tencentRound6Request("http://127.0.0.1").Config

	err := validateSimpleHTTPConfig("test-webhook", []string{"access_token"}, config)

	require.EqualError(t, err, "test-webhook provider requires access_token parameter")
	config.RequestConfig.HTTPRequestConfig.Request.Parameters["access_token"] = "configured"
	require.NoError(t, validateSimpleHTTPConfig("test-webhook", []string{"access_token"}, config))
}

func TestSimpleHTTPProvider_PostsRenderedWebhook(t *testing.T) {
	server, requests := newCaptureServer(t, always(http.StatusOK, `{"accepted":true}`))
	req := tencentRound6Request(server.URL)
	req.Config.RequestConfig.HTTPRequestConfig.Headers = map[string]string{"Content-Type": "application/json"}

	result := (&simpleHTTPProvider{ident: "test-webhook"}).Notify(context.Background(), req)

	require.NoError(t, result.Err)
	require.Equal(t, "12025550123", result.Target)
	require.Contains(t, result.Response, `"accepted":true`)
	require.Len(t, requests(), 1)
	sent := requests()[0]
	require.Equal(t, "/send", sent.Path)
	require.Equal(t, "west", sent.Query.Get("tenant"))
	require.Equal(t, "application/json", sent.Header.Get("Content-Type"))
	require.JSONEq(t, `{"message":"usage 91%","phone":"12025550123"}`, string(sent.Body))
}
