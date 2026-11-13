package provider

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func tencentRound6Request(url string) *NotifyRequest {
	return &NotifyRequest{
		Config: &models.NotifyChannelConfig{RequestConfig: &models.RequestConfig{
			HTTPRequestConfig: &models.HTTPRequestConfig{
				Method: http.MethodPost, URL: url + "/send?existing=1", RetryTimes: 1,
				Headers: map[string]string{
					"Content-Type": "application/json", "X-TC-Action": "SendSms",
					"Service": "sms", "Secret_ID": "local-id", "Secret_Key": "local-secret",
				},
				Request: models.RequestDetail{
					Body:       `{"message":"{{$tpl.content}}","phone":"{{$sendto}}"}`,
					Parameters: map[string]string{"tenant": "{{$params.tenant}}"},
				},
			},
		}},
		Events:       []*models.AlertCurEvent{{RuleName: "CPU high"}},
		TplContent:   map[string]interface{}{"content": "usage 91%"},
		CustomParams: map[string]string{"tenant": "west"}, Sendtos: []string{"12025550123"},
		HttpClient: &http.Client{},
	}
}

func tencentRound6Providers() []NotifyChannelProvider {
	return []NotifyChannelProvider{&TencentSmsProvider{}, &TencentVoiceProvider{}}
}

func TestTencentNotify_PostsRenderedSignedRequest(t *testing.T) {
	for _, p := range tencentRound6Providers() {
		t.Run(p.Ident(), func(t *testing.T) {
			server, requests := newCaptureServer(t, always(http.StatusOK, `{"Response":{"RequestId":"local"}}`))
			req := tencentRound6Request(server.URL)

			result := p.Notify(context.Background(), req)

			require.NoError(t, result.Err)
			require.Equal(t, "12025550123", result.Target)
			require.Contains(t, result.Response, `"RequestId":"local"`)
			require.Len(t, requests(), 1)
			sent := requests()[0]
			require.Equal(t, http.MethodPost, sent.Method)
			require.Equal(t, "/send", sent.Path)
			require.Equal(t, "1", sent.Query.Get("existing"))
			require.Equal(t, "west", sent.Query.Get("tenant"))
			require.JSONEq(t, `{"message":"usage 91%","phone":"12025550123"}`, string(sent.Body))
			timestamp, err := strconv.ParseInt(sent.Header.Get("X-TC-Timestamp"), 10, 64)
			require.NoError(t, err)
			// Validate the signature against the transmitted timestamp, without a clock assumption.
			require.Equal(t, getTxSignature(req.Config.RequestConfig.HTTPRequestConfig, string(sent.Body), timestamp),
				sent.Header.Get("Authorization"))
			require.Equal(t, "local-secret", req.Config.RequestConfig.HTTPRequestConfig.Headers["Secret_Key"])
		})
	}
}

func TestTencentNotify_ReportsHTTPRejection(t *testing.T) {
	for _, p := range tencentRound6Providers() {
		t.Run(p.Ident(), func(t *testing.T) {
			server, requests := newCaptureServer(t, always(http.StatusForbidden, `{"error":"denied"}`))

			result := p.Notify(context.Background(), tencentRound6Request(server.URL))

			require.ErrorContains(t, result.Err, "status code: 403")
			require.Contains(t, result.Response, `"error":"denied"`)
			require.Len(t, requests(), 1)
		})
	}
}

func TestTencentNotify_RejectsInvalidInputsBeforeSending(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*NotifyRequest)
		want   string
	}{
		{name: "missing client", mutate: func(r *NotifyRequest) { r.HttpClient = nil }, want: "http client not found"},
		{name: "empty events", mutate: func(r *NotifyRequest) { r.Events = nil }, want: "events is empty"},
		{
			name: "invalid template", want: "unclosed action",
			mutate: func(r *NotifyRequest) { r.Config.RequestConfig.HTTPRequestConfig.Request.Body = "{{" },
		},
		{
			name: "invalid URL", want: "missing protocol scheme",
			mutate: func(r *NotifyRequest) { r.Config.RequestConfig.HTTPRequestConfig.URL = "://invalid" },
		},
	}
	for _, p := range tencentRound6Providers() {
		for _, tc := range cases {
			t.Run(p.Ident()+"/"+tc.name, func(t *testing.T) {
				server, requests := newCaptureServer(t, always(http.StatusOK, `{}`))
				req := tencentRound6Request(server.URL)
				tc.mutate(req)

				result := p.Notify(context.Background(), req)

				require.ErrorContains(t, result.Err, tc.want)
				require.Empty(t, requests())
			})
		}
	}
}
