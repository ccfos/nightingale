package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// dingtalkRecordingTransport 记录所有出站请求并返回固定响应，测试不会真的访问钉钉。
type dingtalkRecordingTransport struct {
	mu   sync.Mutex
	urls []string
}

func (t *dingtalkRecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.urls = append(t.urls, req.URL.String())
	t.mu.Unlock()

	body := `{"errcode":0,"errmsg":"ok"}`
	if strings.HasSuffix(req.URL.Host, "dingtalk.com") {
		// 万一真的走进应用模式，也给一份能继续跑下去的响应，让断言落在「有没有调用」上
		body = `{"accessToken":"fake-token","errcode":0,"media_id":"fake-media-id"}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func (t *dingtalkRecordingTransport) count(substr string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for _, u := range t.urls {
		if strings.Contains(u, substr) {
			n++
		}
	}
	return n
}

const dingtalkTestWebhookURL = "https://oapi.example.com/robot/send?access_token=token"

func newDingtalkNotifyRequest(appCfg *models.DingtalkRequestConfig, withImage bool, client *http.Client) *NotifyRequest {
	event := &models.AlertCurEvent{Hash: "dingtalk-test"}
	if withImage {
		event.ShotImageBase64 = map[string]string{"shot_1": "data:image/png;base64,iVBORw0KGgo="}
	}

	return &NotifyRequest{
		Config: &models.NotifyChannelConfig{
			Ident:       models.Dingtalk,
			RequestType: "http",
			RequestConfig: &models.RequestConfig{
				DingtalkRequestConfig: appCfg,
				HTTPRequestConfig: &models.HTTPRequestConfig{
					URL:           dingtalkTestWebhookURL,
					Method:        http.MethodPost,
					Headers:       map[string]string{"Content-Type": "application/json"},
					Timeout:       10000,
					RetryTimes:    1,
					RetryInterval: 10,
					Request: models.RequestDetail{
						Body: `{"msgtype":"markdown","markdown":{"title":"{{$tpl.title}}","text":"{{$tpl.content}}"}}`,
					},
				},
			},
		},
		Events:     []*models.AlertCurEvent{event},
		TplContent: map[string]interface{}{"title": "t", "content": "c"},
		HttpClient: client,
	}
}

// TestDingtalkProviderNotifyWithBlankAppConfig 回归：前端提交的 request_config 里可能带一份字段全空的
// dingtalk_request_config，此时不能把群机器人通知拐进钉钉应用模式（会以 app key cannot be empty 整条失败）。
func TestDingtalkProviderNotifyWithBlankAppConfig(t *testing.T) {
	cases := []struct {
		name   string
		appCfg *models.DingtalkRequestConfig
	}{
		{name: "nil config", appCfg: nil},
		{name: "empty config", appCfg: &models.DingtalkRequestConfig{}},
		{name: "blank fields", appCfg: &models.DingtalkRequestConfig{AppKey: "  ", AppSecret: "  "}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := &dingtalkRecordingTransport{}
			req := newDingtalkNotifyRequest(c.appCfg, true, &http.Client{Transport: tr})

			result := (&DingtalkProvider{}).Notify(context.Background(), req)
			if result.Err != nil {
				t.Fatalf("notify failed: %v, response: %s", result.Err, result.Response)
			}
			if n := tr.count("api.dingtalk.com"); n != 0 {
				t.Fatalf("should not call dingtalk open api, got %d request(s): %v", n, tr.urls)
			}
			if n := tr.count("oapi.example.com"); n != 1 {
				t.Fatalf("webhook should be called exactly once, got %d: %v", n, tr.urls)
			}
			if _, ok := req.CustomParams["shot_image_key"]; ok {
				t.Fatal("shot_image_key should not be injected without app key")
			}
		})
	}
}

// TestDingtalkProviderNotifyWithAppConfig 填了 app_key/app_secret 且事件带截图时，才走上传链路。
func TestDingtalkProviderNotifyWithAppConfig(t *testing.T) {
	appCfg := &models.DingtalkRequestConfig{AppKey: "key", AppSecret: "secret"}

	t.Run("with image", func(t *testing.T) {
		tr := &dingtalkRecordingTransport{}
		req := newDingtalkNotifyRequest(appCfg, true, &http.Client{Transport: tr})

		result := (&DingtalkProvider{}).Notify(context.Background(), req)
		if result.Err != nil {
			t.Fatalf("notify failed: %v, response: %s", result.Err, result.Response)
		}
		if n := tr.count("api.dingtalk.com/v1.0/oauth2/accessToken"); n != 1 {
			t.Fatalf("access token should be requested once, got %d: %v", n, tr.urls)
		}
		if n := tr.count("oapi.example.com"); n != 1 {
			t.Fatalf("webhook should be called exactly once, got %d: %v", n, tr.urls)
		}
	})

	t.Run("without image", func(t *testing.T) {
		tr := &dingtalkRecordingTransport{}
		req := newDingtalkNotifyRequest(appCfg, false, &http.Client{Transport: tr})

		result := (&DingtalkProvider{}).Notify(context.Background(), req)
		if result.Err != nil {
			t.Fatalf("notify failed: %v, response: %s", result.Err, result.Response)
		}
		if n := tr.count("dingtalk.com"); n != 0 {
			t.Fatalf("should not touch dingtalk open api without image, got %d: %v", n, tr.urls)
		}
	})
}
