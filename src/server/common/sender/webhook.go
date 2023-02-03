package sender

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
)

func SendWebhooks(webhooks []config.Webhook, event *models.AlertCurEvent) {
	for _, conf := range webhooks {
		if conf.Url == "" || !conf.Enable {
			continue
		}
		bs, err := json.Marshal(event)
		if err != nil {
			continue
		}

		bf := bytes.NewBuffer(bs)

		req, err := http.NewRequest("POST", conf.Url, bf)
		if err != nil {
			logger.Warning("alertingWebhook failed to new request", err)
			continue
		}

		if conf.BasicAuthUser != "" && conf.BasicAuthPass != "" {
			req.SetBasicAuth(conf.BasicAuthUser, conf.BasicAuthPass)
		}

		if len(conf.Headers) > 0 && len(conf.Headers)%2 == 0 {
			for i := 0; i < len(conf.Headers); i += 2 {
				if conf.Headers[i] == "host" {
					req.Host = conf.Headers[i+1]
					continue
				}
				req.Header.Set(conf.Headers[i], conf.Headers[i+1])
			}
		}

		client := http.Client{
			Timeout: conf.TimeoutDuration,
		}

		var resp *http.Response
		resp, err = client.Do(req)
		if err != nil {
			logger.Warningf("WebhookCallError, ruleId: [%d], eventId: [%d], url: [%s], error: [%s]", event.RuleId, event.Id, conf.Url, err)
			continue
		}

		var body []byte
		if resp.Body != nil {
			defer resp.Body.Close()
			body, _ = ioutil.ReadAll(resp.Body)
		}

		logger.Debugf("alertingWebhook done, url: %s, response code: %d, body: %s", conf.Url, resp.StatusCode, string(body))
	}
}
