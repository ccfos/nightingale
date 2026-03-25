package provider

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

func TestFeishuAppProviderNotify(t *testing.T) {
	cfg := &models.NotifyChannelConfig{
		RequestType: "feishuapp",
		RequestConfig: &models.RequestConfig{
			FeishuAppRequestConfig: &models.FeishuAppRequestConfig{
				AppID:         "cli_a9303433f8f8dcc4",
				AppSecret:     "qPDd0wwxyqykI9FhrlQCLbNmSamRyA1k",
				ContactKey:    "user_id",
				ReceiveIDType: "user_id",
				Timeout:       10000,
				RetryTimes:    1,
				RetrySleep:    10,
			},
			HTTPRequestConfig: &models.HTTPRequestConfig{
				Timeout:       10000,
				RetryTimes:    1,
				RetryInterval: 10,
			},
		},
	}
	client, err := models.GetHTTPClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create HTTP client: %v", err)
	}

	p := &FeishuAppProvider{}
	req := &NotifyRequest{
		Config: cfg,
		Events: []*models.AlertCurEvent{{
			Hash: "hash-test",
			AnnotationsJSON: map[string]string{
				"alert_image_base64": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
			},
		}},
		TplContent: map[string]interface{}{
			"title":   "Test Title",
			"content": "## test markdown content",
		},
		Sendtos:    []string{},
		ImGroupIDs: []string{"oc_a44a1c1fee24e3b3d985a358d26067ea"},
		HttpClient: client,
	}

	result := p.Notify(context.Background(), req)
	if result.Err != nil {
		t.Fatalf("Notify 返回错误: %v", result.Err)
	}
	t.Logf("result: %+v", result)
}
