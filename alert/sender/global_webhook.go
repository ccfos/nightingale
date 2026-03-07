package sender

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/toolkits/pkg/logger"
)

var globalWebhookClient *http.Client
var globalWebhookConf aconf.GlobalWebhook

func InitGlobalWebhook(conf aconf.GlobalWebhook) {
	globalWebhookConf = conf
	if !conf.Enable || conf.Url == "" {
		return
	}

	if len(conf.Headers) > 0 && len(conf.Headers)%2 != 0 {
		logger.Warningf("global_webhook headers count is odd(%d), headers will be ignored", len(conf.Headers))
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

	globalWebhookClient = &http.Client{
		Timeout:   time.Duration(timeout) * time.Second,
		Transport: transport,
	}

	logger.Infof("global_webhook initialized, url:%s", conf.Url)
}

func SendGlobalWebhook(event *models.AlertCurEvent, stats *astats.Stats) {
	if globalWebhookClient == nil {
		return
	}

	bs, err := json.Marshal(event)
	if err != nil {
		logger.Errorf("global_webhook failed to marshal event err:%v", err)
		return
	}

	req, err := http.NewRequest("POST", globalWebhookConf.Url, bytes.NewBuffer(bs))
	if err != nil {
		logger.Warningf("global_webhook failed to new request event:%s err:%v", string(bs), err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if globalWebhookConf.BasicAuthUser != "" && globalWebhookConf.BasicAuthPass != "" {
		req.SetBasicAuth(globalWebhookConf.BasicAuthUser, globalWebhookConf.BasicAuthPass)
	}

	if len(globalWebhookConf.Headers) > 0 && len(globalWebhookConf.Headers)%2 == 0 {
		for i := 0; i < len(globalWebhookConf.Headers); i += 2 {
			if globalWebhookConf.Headers[i] == "Host" || globalWebhookConf.Headers[i] == "host" {
				req.Host = globalWebhookConf.Headers[i+1]
				continue
			}
			req.Header.Set(globalWebhookConf.Headers[i], globalWebhookConf.Headers[i+1])
		}
	}

	stats.AlertNotifyTotal.WithLabelValues("global_webhook").Inc()
	resp, err := globalWebhookClient.Do(req)
	if err != nil {
		stats.AlertNotifyErrorTotal.WithLabelValues("global_webhook").Inc()
		logger.Errorf("global_webhook_fail url:%s event:%s error:%v", globalWebhookConf.Url, event.Hash, err)
		return
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		stats.AlertNotifyErrorTotal.WithLabelValues("global_webhook").Inc()
		logger.Errorf("global_webhook_fail url:%s status:%d body:%s event:%s", globalWebhookConf.Url, resp.StatusCode, string(body), event.Hash)
		return
	}

	logger.Debugf("global_webhook_succ url:%s status:%d body:%s event:%s", globalWebhookConf.Url, resp.StatusCode, string(body), event.Hash)
}
