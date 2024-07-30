package sender

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/toolkits/pkg/logger"
)

func sendWebhook(webhook *models.Webhook, event interface{}, stats *astats.Stats) bool {
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
		logger.Warningf("alertingWebhook failed to new reques event:%s err:%v", string(bs), err)
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
	insecureSkipVerify := false
	if webhook != nil {
		insecureSkipVerify = webhook.SkipVerify
	}
	client := http.Client{
		Timeout: time.Duration(conf.Timeout) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify},
		},
	}

	stats.AlertNotifyTotal.WithLabelValues("webhook").Inc()
	var resp *http.Response
	resp, err = client.Do(req)
	if err != nil {
		stats.AlertNotifyErrorTotal.WithLabelValues("webhook").Inc()
		logger.Errorf("event_webhook_fail, event:%s, url: [%s], error: [%s]", string(bs), conf.Url, err)
		return true
	}

	var body []byte
	if resp.Body != nil {
		defer resp.Body.Close()
		body, _ = io.ReadAll(resp.Body)
	}

	if resp.StatusCode == 429 {
		logger.Errorf("event_webhook_fail, url: %s, response code: %d, body: %s event:%s", conf.Url, resp.StatusCode, string(body), string(bs))
		return true
	}

	logger.Debugf("event_webhook_succ, url: %s, response code: %d, body: %s event:%s", conf.Url, resp.StatusCode, string(body), string(bs))
	return false
}

func SingleSendWebhooks(webhooks []*models.Webhook, event *models.AlertCurEvent, stats *astats.Stats) {
	for _, conf := range webhooks {
		retryCount := 0
		for retryCount < 3 {
			needRetry := sendWebhook(conf, event, stats)
			if !needRetry {
				break
			}
			retryCount++
			time.Sleep(time.Minute * 1 * time.Duration(retryCount))
		}
	}
}

func BatchSendWebhooks(webhooks []*models.Webhook, event *models.AlertCurEvent, stats *astats.Stats) {
	for _, conf := range webhooks {
		logger.Infof("push event:%+v to queue:%v", event, conf)
		PushEvent(conf, event, stats)
	}
}

var EventQueue = make(map[string]*WebhookQueue)
var EventQueueLock sync.RWMutex

const QueueMaxSize = 100000

type WebhookQueue struct {
	list    *SafeListLimited
	closeCh chan struct{}
}

func PushEvent(webhook *models.Webhook, event *models.AlertCurEvent, stats *astats.Stats) {
	EventQueueLock.RLock()
	queue := EventQueue[webhook.Url]
	EventQueueLock.RUnlock()

	if queue == nil {
		queue = &WebhookQueue{
			list:    NewSafeListLimited(QueueMaxSize),
			closeCh: make(chan struct{}),
		}

		EventQueueLock.Lock()
		EventQueue[webhook.Url] = queue
		EventQueueLock.Unlock()

		StartConsumer(queue, webhook.Batch, webhook, stats)
	}

	succ := queue.list.PushFront(event)
	if !succ {
		logger.Warningf("Write channel(%s) full, current channel size: %d event:%v", webhook.Url, queue.list.Len(), event)
	}
}

func StartConsumer(queue *WebhookQueue, popSize int, webhook *models.Webhook, stats *astats.Stats) {
	for {
		select {
		case <-queue.closeCh:
			logger.Infof("event queue:%v closed", queue)
			return
		default:
			events := queue.list.PopBack(popSize)
			if len(events) == 0 {
				time.Sleep(time.Millisecond * 400)
				continue
			}

			retryCount := 0
			for retryCount < webhook.RetryCount {
				needRetry := sendWebhook(webhook, events, stats)
				if !needRetry {
					break
				}
				retryCount++
				time.Sleep(time.Second * time.Duration(webhook.RetryInterval) * time.Duration(retryCount))
			}
		}
	}
}
