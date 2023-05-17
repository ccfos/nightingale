package memsto

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
)

type NotifyConfigCacheType struct {
	ctx      *ctx.Context
	webhooks []*models.Webhook
	smtp     aconf.SMTPConfig
	script   models.NotifyScript
	ibex     aconf.Ibex

	sync.RWMutex
}

const DefaultSMTP = `
Host = ""
Port = 994
User = "username"
Pass = "password"
From = "username@163.com"
InsecureSkipVerify = true
Batch = 5
`

const DefaultIbex = `
Address = "http://127.0.0.1:10090"
BasicAuthUser = "ibex"
BasicAuthPass = "ibex"
Timeout = 3000
`

func NewNotifyConfigCache(ctx *ctx.Context) *NotifyConfigCacheType {
	w := &NotifyConfigCacheType{
		ctx: ctx,
	}
	w.SyncNotifyConfigs()
	return w
}

func (w *NotifyConfigCacheType) SyncNotifyConfigs() {
	err := w.syncNotifyConfigs()
	if err != nil {
		logger.Errorf("failed to sync webhooks:", err)
	}

	go w.loopSyncNotifyConfigs()
}

func (w *NotifyConfigCacheType) loopSyncNotifyConfigs() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := w.syncNotifyConfigs(); err != nil {
			logger.Warning("failed to sync webhooks:", err)
		}
	}
}

func (w *NotifyConfigCacheType) syncNotifyConfigs() error {
	w.RWMutex.Lock()
	defer w.RWMutex.Unlock()

	cval, err := models.ConfigsGet(w.ctx, models.WEBHOOKKEY)
	if err != nil {
		return err
	}

	if strings.TrimSpace(cval) != "" {
		err = json.Unmarshal([]byte(cval), &w.webhooks)
		if err != nil {
			logger.Errorf("failed to unmarshal webhooks:%s config:", cval, err)
		}
	}

	logger.Infof("timer: sync wbhooks done number: %d", len(w.webhooks))

	cval, err = models.ConfigsGet(w.ctx, models.SMTP)
	if err != nil {
		return err
	}

	if strings.TrimSpace(cval) != "" {
		err = toml.Unmarshal([]byte(cval), &w.smtp)
		if err != nil {
			logger.Errorf("failed to unmarshal smtp:%s config:", cval, err)
		}
	}

	logger.Infof("timer: sync smtp:%+v done", w.smtp)

	cval, err = models.ConfigsGet(w.ctx, models.NOTIFYSCRIPT)
	if err != nil {
		return err
	}

	if strings.TrimSpace(cval) != "" {
		err = json.Unmarshal([]byte(cval), &w.script)
		if err != nil {
			logger.Errorf("failed to unmarshal notify script:%s config:", cval, err)
		}
	}

	logger.Infof("timer: sync notify script done")

	cval, err = models.ConfigsGet(w.ctx, models.IBEX)
	if err != nil {
		return err
	}

	if strings.TrimSpace(cval) != "" {
		err = toml.Unmarshal([]byte(cval), &w.ibex)
		if err != nil {
			logger.Errorf("failed to unmarshal ibex:%s config:", cval, err)
		}
	} else {
		err = toml.Unmarshal([]byte(DefaultIbex), &w.ibex)
		if err != nil {
			logger.Errorf("failed to unmarshal ibex:%s config:", cval, err)
		}
	}

	logger.Infof("timer: sync ibex done")

	return nil
}

func (w *NotifyConfigCacheType) GetWebhooks() []*models.Webhook {
	w.RWMutex.RLock()
	defer w.RWMutex.RUnlock()
	return w.webhooks
}

func (w *NotifyConfigCacheType) GetSMTP() aconf.SMTPConfig {
	w.RWMutex.RLock()
	defer w.RWMutex.RUnlock()
	return w.smtp
}

func (w *NotifyConfigCacheType) GetNotifyScript() models.NotifyScript {
	w.RWMutex.RLock()
	defer w.RWMutex.RUnlock()
	if w.script.Timeout == 0 {
		w.script.Timeout = 10
	}

	return w.script
}

func (w *NotifyConfigCacheType) GetIbex() aconf.Ibex {
	w.RWMutex.RLock()
	defer w.RWMutex.RUnlock()
	return w.ibex
}
