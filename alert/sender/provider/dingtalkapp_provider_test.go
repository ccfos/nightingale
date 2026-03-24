package provider

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

var (
	appCfg = &models.DingtalkAppRequestConfig{
		AppKey:     "dingosbj1hmokzniirku",
		AppSecret:  "_VmS_fchCwx2_qpszmdOuBMBivMrIDJJj9kGaFYbQfLYb0huQgiEWSdJZ3RLlsbJ",
		ContactKey: "phone",
		Timeout:    10000,
		RetryTimes: 1,
		RetrySleep: 1,
	}
)

func TestDingtalkAppProviderNotify(t *testing.T) {
	p := &DingtalkAppProvider{}
	cfg := &models.NotifyChannelConfig{
		RequestType: "dingtalkapp",
		RequestConfig: &models.RequestConfig{
			DingtalkAppRequestConfig: appCfg,
			HTTPRequestConfig: &models.HTTPRequestConfig{
				Timeout:       10000,
				RetryTimes:    1,
				RetryInterval: 1,
			},
		},
	}
	client, err := models.GetHTTPClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create HTTP client: %v", err)
	}
	req := &NotifyRequest{
		Config: cfg,
		Events: []*models.AlertCurEvent{{
			Hash: "hash-test",
			AnnotationsJSON: map[string]string{
				"alert_image_base64": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
			},
		}},
		TplContent: map[string]interface{}{
			"title":   "test alert",
			"content": "{{ $event.Hash }}\n\n- item 1\n- item 2\n\n`code`",
		},
		Sendtos: []string{"18291906071"},
		CustomParams: map[string]string{
			"card_template_id": "423abee6-e7ca-4d64-8c61-7a4597976d4b.schema",
		},
		HttpClient: client,
	}

	result := p.Notify(context.Background(), req)
	t.Logf("result: %+v", result)
}
