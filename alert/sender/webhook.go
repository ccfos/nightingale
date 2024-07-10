package sender

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/toolkits/pkg/logger"
)

func sendWebhook(webhook *models.Webhook, event *models.AlertCurEvent, stats *astats.Stats) bool {
	conf := webhook
	if conf.Url == "" || !conf.Enable {
		return false
	}
	bs, err := json.Marshal(event)
	if err != nil {
		logger.Errorf("alertingWebhook failed to marshal event:%+v err:%v", event, err)
		return false
	}

	bf := bytes.NewBuffer(bs)

	req, err := http.NewRequest("POST", conf.Url, bf)
	if err != nil {
		logger.Warningf("alertingWebhook failed to new reques event:%+v err:%v", event, err)
		return true
	}

	req.Header.Set("Content-Type", "application/json")
	if conf.BasicAuthUser != "" && conf.BasicAuthPass != "" {
		req.SetBasicAuth(conf.BasicAuthUser, conf.BasicAuthPass)
	}

	if len(conf.Headers) > 0 && len(conf.Headers)%2 == 0 {
		for i := 0; i < len(conf.Headers); i += 2 {
			if conf.Headers[i] == "host" || conf.Headers[i] == "Host" {
				req.Host = conf.Headers[i+1]
				continue
			}
			req.Header.Set(conf.Headers[i], conf.Headers[i+1])
		}
	}

	// todo add skip verify
	client := http.Client{
		Timeout: time.Duration(conf.Timeout) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	stats.AlertNotifyTotal.WithLabelValues("webhook").Inc()
	var resp *http.Response
	resp, err = client.Do(req)
	if err != nil {
		stats.AlertNotifyErrorTotal.WithLabelValues("webhook").Inc()
		logger.Errorf("event_webhook_fail, ruleId: [%d], eventId: [%d], event:%+v, url: [%s], error: [%s]", event.RuleId, event.Id, event, conf.Url, err)
		return true
	}

	var body []byte
	if resp.Body != nil {
		defer resp.Body.Close()
		body, _ = io.ReadAll(resp.Body)
	}

	if resp.StatusCode == 429 {
		logger.Errorf("event_webhook_fail, url: %s, response code: %d, body: %s event:%+v", conf.Url, resp.StatusCode, string(body), event)
		return true
	}

	logger.Debugf("event_webhook_succ, url: %s, response code: %d, body: %s event:%+v", conf.Url, resp.StatusCode, string(body), event)
	return false
}

func SendWebhooks(webhooks []*models.Webhook, event *models.AlertCurEvent, stats *astats.Stats) {
	for _, conf := range webhooks {
		retryCount := 0
		for retryCount < 3 {
			needRetry := sendWebhook(conf, event, stats)
			if !needRetry {
				break
			}
			retryCount++
			time.Sleep(time.Second * 10 * time.Duration(retryCount))
		}
	}
}
