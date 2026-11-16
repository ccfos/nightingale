package provider

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestR8dPagerDutyContinuesAfterFailedRoutingKey(t *testing.T) {
	srv, got := newCaptureServer(t, func(_ int, r capturedRequest) (int, string) {
		if r.json(t)["routing_key"] == "bad" {
			return 400, `{"message":"rejected"}`
		}
		return 202, `{"status":"success"}`
	})
	req := webhookReq("pagerduty", &models.RequestConfig{PagerDutyRequestConfig: &models.PagerDutyRequestConfig{RetryTimes: 1, RetrySleep: 1}}, nil)
	req.HttpClient = wecomLocalClient(t, srv)
	req.PagerDutyRoutingKeys = []string{"bad", "good"}
	result := (&PagerDutyProvider{}).Notify(context.Background(), req)
	require.ErrorContains(t, result.Err, "routing_key bad")
	require.Contains(t, result.Response, "success")
	require.Equal(t, "bad,good", result.Target)
	calls := got()
	require.Len(t, calls, 3)
	for _, r := range calls {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/v2/enqueue", r.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.Equal(t, "h", r.json(t)["dedup_key"])
	}
}
func TestR8dPagerDutyTransportRetries(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: flashDutyTestTransport(func(r *http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })}
	response, err := SendPagerDuty(&models.PagerDutyRequestConfig{RetryTimes: 1, RetrySleep: 1}, []*models.AlertCurEvent{{Id: 7}}, "key", "", client)
	require.ErrorContains(t, err, "unexpected EOF")
	require.Contains(t, response, "event 7")
	require.Equal(t, 2, calls)
}
func TestR8dFlashDutyContinuesAfterFailedChannel(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, `{"success":true}`))
	count := 0
	transport := srv.Client().Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client := &http.Client{Transport: flashDutyTestTransport(func(r *http.Request) (*http.Response, error) {
		count++
		if count <= 2 {
			return nil, io.ErrUnexpectedEOF
		}
		return transport.RoundTrip(r)
	})}
	req := webhookReq("flashduty", &models.RequestConfig{FlashDutyRequestConfig: &models.FlashDutyRequestConfig{IntegrationUrl: srv.URL, RetryTimes: 1, RetrySleep: 1}}, nil)
	req.HttpClient = client
	req.FlashDutyChannelIDs = []int64{1, 2}
	result := (&FlashDutyProvider{}).Notify(context.Background(), req)
	require.ErrorContains(t, result.Err, "channel 1")
	require.Contains(t, result.Response, "success")
	require.Equal(t, "1,2", result.Target)
	require.Equal(t, 3, count)
	require.Len(t, got(), 1)
	require.Equal(t, "POST", got()[0].Method)
}
func TestR8dHTTPFailures(t *testing.T) {
	cfg := &models.HTTPRequestConfig{URL: "http://local.invalid", Method: "POST", RetryTimes: 1}
	event := []*models.AlertCurEvent{{Id: 1}}
	_, err := SendHTTPRequest(cfg, event, nil, nil, nil, nil)
	require.ErrorContains(t, err, "http client not found")
	_, err = SendHTTPRequest(cfg, nil, nil, nil, nil, &http.Client{})
	require.ErrorContains(t, err, "events is empty")
	cfg.Request.Body = "{{"
	_, err = SendHTTPRequest(cfg, event, nil, nil, nil, &http.Client{})
	require.Error(t, err)
	cfg.Request.Body = `{"test":true}`
	cfg.URL = "://bad"
	_, err = SendHTTPRequest(cfg, event, nil, nil, nil, &http.Client{})
	require.Error(t, err)
	srv, got := newCaptureServer(t, always(400, `rejected`))
	cfg.URL = srv.URL
	response, err := SendHTTPRequest(cfg, event, nil, nil, nil, srv.Client())
	require.ErrorContains(t, err, "status code: 400")
	require.Contains(t, response, "rejected")
	require.Len(t, got(), 1)
	client := &http.Client{Transport: flashDutyTestTransport(func(r *http.Request) (*http.Response, error) { return nil, context.Canceled })}
	_, err = SendHTTPRequest(cfg, event, nil, nil, nil, client)
	require.ErrorContains(t, err, "all retries failed")
}
