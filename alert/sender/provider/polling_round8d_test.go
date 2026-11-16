package provider

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestR8dJSMPollErrors(t *testing.T) {
	old := jsmPollGap
	jsmPollGap = 0
	t.Cleanup(func() { jsmPollGap = old })
	for _, status := range []int{400, 401, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, got := newCaptureServer(t, always(status, `{"message":"denied"}`))
			result, err := (&JSMAlertProvider{}).waitRequest(context.Background(), srv.Client(), srv.URL, "key", "request", time.Second)
			require.Nil(t, result)
			require.ErrorContains(t, err, "unexpected status code")
			require.Len(t, got(), 1)
			require.Equal(t, "/requests/request", got()[0].Path)
			require.Equal(t, "GenieKey key", got()[0].Header.Get("Authorization"))
		})
	}
	c, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (&JSMAlertProvider{}).waitRequest(c, &http.Client{}, "http://local.invalid", "key", "request", time.Second)
	require.ErrorIs(t, err, context.Canceled)
	_, err = (&JSMAlertProvider{}).waitRequest(context.Background(), &http.Client{}, ":bad", "key", "request", time.Second)
	require.Error(t, err)
}
func TestR8dFeishuNotifyStopsOnSendFailure(t *testing.T) {
	for _, stage := range []string{"image", "personal", "group"} {
		t.Run(stage, func(t *testing.T) {
			srv, got := newCaptureServer(t, func(_ int, r capturedRequest) (int, string) {
				if r.Path == "/token" {
					return 200, `{"code":0,"tenant_access_token":"token"}`
				}
				return 200, `{"code":400,"msg":"denied"}`
			})
			localFeishuEndpoints(t, srv.URL)
			req := webhookReq("feishuapp", &models.RequestConfig{FeishuAppRequestConfig: &models.FeishuAppRequestConfig{AppID: "app", AppSecret: "secret", ReceiveIDType: "open_id"}}, nil)
			req.HttpClient = srv.Client()
			switch stage {
			case "image":
				req.Events[0].ShotImageBase64 = map[string]string{"shot": "aW1hZ2U="}
			case "personal":
				req.Sendtos = []string{"alice", "bob"}
			case "group":
				req.ImGroupIDs = []string{"group1", "group2"}
			}
			result := (&FeishuAppProvider{}).Notify(context.Background(), req)
			require.ErrorContains(t, result.Err, "denied")
			require.Len(t, got(), 2)
			require.Equal(t, "Bearer token", got()[1].Header.Get("Authorization"))
		})
	}
}
