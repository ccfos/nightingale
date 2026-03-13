package sender

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/toolkits/pkg/logger"
)

var staticGlobalWebhookClient *http.Client
var staticGlobalWebhookConf aconf.GlobalWebhook

const staticGlobalWebhookChannel = "static_global_webhook"

func InitStaticGlobalWebhook(conf aconf.GlobalWebhook) {
	staticGlobalWebhookConf = conf
	if !conf.Enable || conf.Url == "" {
		return
	}

	if len(conf.Headers) > 0 && len(conf.Headers)%2 != 0 {
		logger.Warningf("static_global_webhook headers count is odd(%d), headers will be ignored", len(conf.Headers))
	}

	timeout := conf.Timeout
	if timeout <= 0 {
		timeout = 10
	}

	transport := &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: conf.SkipVerify},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	if poster.UseProxy(conf.Url) {
		transport.Proxy = http.ProxyFromEnvironment
	}

	staticGlobalWebhookClient = &http.Client{
		Timeout:   time.Duration(timeout) * time.Second,
		Transport: transport,
	}

	logger.Infof("static_global_webhook initialized, url:%s", conf.Url)
}

func SendStaticGlobalWebhook(ctx *ctx.Context, event *models.AlertCurEvent, stats *astats.Stats) {
	if staticGlobalWebhookClient == nil {
		return
	}

	bs, err := json.Marshal(event)
	if err != nil {
		logger.Errorf("%s failed to marshal event err:%v", staticGlobalWebhookChannel, err)
		NotifyRecord(ctx, []*models.AlertCurEvent{event}, 0, staticGlobalWebhookChannel, staticGlobalWebhookConf.Url, "", err)
		return
	}

	req, err := http.NewRequest("POST", staticGlobalWebhookConf.Url, bytes.NewBuffer(bs))
	if err != nil {
		logger.Warningf("%s failed to new request event:%s err:%v", staticGlobalWebhookChannel, string(bs), err)
		NotifyRecord(ctx, []*models.AlertCurEvent{event}, 0, staticGlobalWebhookChannel, staticGlobalWebhookConf.Url, "", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if staticGlobalWebhookConf.BasicAuthUser != "" && staticGlobalWebhookConf.BasicAuthPass != "" {
		req.SetBasicAuth(staticGlobalWebhookConf.BasicAuthUser, staticGlobalWebhookConf.BasicAuthPass)
	}

	if len(staticGlobalWebhookConf.Headers) > 0 && len(staticGlobalWebhookConf.Headers)%2 == 0 {
		for i := 0; i < len(staticGlobalWebhookConf.Headers); i += 2 {
			if staticGlobalWebhookConf.Headers[i] == "Host" || staticGlobalWebhookConf.Headers[i] == "host" {
				req.Host = staticGlobalWebhookConf.Headers[i+1]
				continue
			}
			req.Header.Set(staticGlobalWebhookConf.Headers[i], staticGlobalWebhookConf.Headers[i+1])
		}
	}

	stats.AlertNotifyTotal.WithLabelValues(staticGlobalWebhookChannel).Inc()
	resp, err := staticGlobalWebhookClient.Do(req)
	if err != nil {
		stats.AlertNotifyErrorTotal.WithLabelValues(staticGlobalWebhookChannel).Inc()
		logger.Errorf("%s_fail url:%s event:%s error:%v", staticGlobalWebhookChannel, staticGlobalWebhookConf.Url, event.Hash, err)
		NotifyRecord(ctx, []*models.AlertCurEvent{event}, 0, staticGlobalWebhookChannel, staticGlobalWebhookConf.Url, "", err)
		return
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	res := fmt.Sprintf("status_code:%d, response:%s", resp.StatusCode, string(body))
	if resp.StatusCode >= 400 {
		stats.AlertNotifyErrorTotal.WithLabelValues(staticGlobalWebhookChannel).Inc()
		logger.Errorf("%s_fail url:%s status:%d body:%s event:%s", staticGlobalWebhookChannel, staticGlobalWebhookConf.Url, resp.StatusCode, string(body), event.Hash)
		NotifyRecord(ctx, []*models.AlertCurEvent{event}, 0, staticGlobalWebhookChannel, staticGlobalWebhookConf.Url, res, fmt.Errorf("status code %d", resp.StatusCode))
		return
	}

	logger.Debugf("%s_succ url:%s status:%d body:%s event:%s", staticGlobalWebhookChannel, staticGlobalWebhookConf.Url, resp.StatusCode, string(body), event.Hash)
	NotifyRecord(ctx, []*models.AlertCurEvent{event}, 0, staticGlobalWebhookChannel, staticGlobalWebhookConf.Url, res, nil)
}
