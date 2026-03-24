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

func TestDingtalkAppProviderNotifyWithoutClient(t *testing.T) {
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
		Config:     cfg,
		Events:     []*models.AlertCurEvent{{Hash: "hash-test"}},
		TplContent: map[string]interface{}{"title": "x", "content": "y"},
		Sendtos:    []string{"18291906071"},
		CustomParams: map[string]string{
			"card_template_id": "b38b7af0-6707-4572-9270-0ea477f01f56.schema",
		},
		HttpClient: client,
	}

	result := p.Notify(context.Background(), req)
	t.Logf("result: %+v", result)
}
