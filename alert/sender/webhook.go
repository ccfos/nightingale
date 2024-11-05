package sender

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
)

func sendWebhook(webhook *models.Webhook, event interface{}, stats *astats.Stats) (bool, string, error) {
	channel := "webhook"
	if webhook.Type == models.RuleCallback {
		channel = "callback"
	}

	conf := webhook
	if conf.Url == "" || !conf.Enable {
		return false, "", nil
	}
	bs, err := json.Marshal(event)
	if err != nil {
		logger.Errorf("%s alertingWebhook failed to marshal event:%+v err:%v", channel, event, err)
		return false, "", err
	}

	bf := bytes.NewBuffer(bs)

	req, err := http.NewRequest("POST", conf.Url, bf)
	if err != nil {
		logger.Warningf("%s alertingWebhook failed to new reques event:%s err:%v", channel, string(bs), err)
		return true, "", err
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

	stats.AlertNotifyTotal.WithLabelValues(channel).Inc()
	var resp *http.Response
	var body []byte
	resp, err = client.Do(req)

	if err != nil {
		stats.AlertNotifyErrorTotal.WithLabelValues(channel).Inc()
		logger.Errorf("event_%s_fail, event:%s, url: [%s], error: [%s]", channel, string(bs), conf.Url, err)
		return true, "", err
	}

	if resp.Body != nil {
		defer resp.Body.Close()
		body, _ = io.ReadAll(resp.Body)
	}

	if resp.StatusCode == 429 {
		logger.Errorf("event_%s_fail, url: %s, response code: %d, body: %s event:%s", channel, conf.Url, resp.StatusCode, string(body), string(bs))
		return true, string(body), fmt.Errorf("status code is 429")
	}

	logger.Debugf("event_%s_succ, url: %s, response code: %d, body: %s event:%s", channel, conf.Url, resp.StatusCode, string(body), string(bs))
	return false, string(body), nil
}

func SingleSendWebhooks(ctx *ctx.Context, webhooks []*models.Webhook, event *models.AlertCurEvent, stats *astats.Stats) {
	for _, conf := range webhooks {
		retryCount := 0
		for retryCount < 3 {
			needRetry, res, err := sendWebhook(conf, event, stats)
			NotifyRecord(ctx, event, "webhook", conf.Url, res, err)
			if !needRetry {
				break
			}
			retryCount++
			time.Sleep(time.Minute * 1 * time.Duration(retryCount))
		}
	}
}

func BatchSendWebhooks(ctx *ctx.Context, webhooks []*models.Webhook, event *models.AlertCurEvent, stats *astats.Stats) {
	for _, conf := range webhooks {
		logger.Infof("push event:%+v to queue:%v", event, conf)
		PushEvent(ctx, conf, event, stats)
	}
}

var EventQueue = make(map[string]*WebhookQueue)
var CallbackEventQueue = make(map[string]*WebhookQueue)
var CallbackEventQueueLock sync.RWMutex
var EventQueueLock sync.RWMutex

const QueueMaxSize = 100000

type WebhookQueue struct {
	list    *SafeListLimited
	closeCh chan struct{}
}

func PushEvent(ctx *ctx.Context, webhook *models.Webhook, event *models.AlertCurEvent, stats *astats.Stats) {
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

		StartConsumer(ctx, queue, webhook.Batch, webhook, stats)
	}

	succ := queue.list.PushFront(event)
	if !succ {
		stats.AlertNotifyErrorTotal.WithLabelValues("push_event_queue").Inc()
		logger.Warningf("Write channel(%s) full, current channel size: %d event:%v", webhook.Url, queue.list.Len(), event)
	}
}

func StartConsumer(ctx *ctx.Context, queue *WebhookQueue, popSize int, webhook *models.Webhook, stats *astats.Stats) {
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
				needRetry, res, err := sendWebhook(webhook, events, stats)
				go RecordEvents(ctx, webhook, events, stats, res, err)
				if !needRetry {
					break
				}
				retryCount++
				time.Sleep(time.Second * time.Duration(webhook.RetryInterval) * time.Duration(retryCount))
			}
		}
	}
}

func RecordEvents(ctx *ctx.Context, webhook *models.Webhook, events []*models.AlertCurEvent, stats *astats.Stats, res string, err error) {
	for _, event := range events {
		time.Sleep(time.Millisecond * 10)
		NotifyRecord(ctx, event, "webhook", webhook.Url, res, err)
	}
}
