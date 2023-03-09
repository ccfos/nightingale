package memsto

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
)

type WebhookCacheType struct {
	ctx      *ctx.Context
	webhooks []*models.Webhook

	sync.RWMutex
}

func NewWebhookCache(ctx *ctx.Context) *WebhookCacheType {
	w := &WebhookCacheType{
		ctx: ctx,
	}
	w.SyncWebhooks()
	return w
}

func (w *WebhookCacheType) SyncWebhooks() {
	err := w.syncWebhooks()
	if err != nil {
		logger.Errorf("failed to sync webhooks:", err)
	}

	go w.loopSyncWebhooks()
}

func (w *WebhookCacheType) loopSyncWebhooks() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := w.syncWebhooks(); err != nil {
			logger.Warning("failed to sync webhooks:", err)
		}
	}
}

func (w *WebhookCacheType) syncWebhooks() error {
	cval, err := models.ConfigsGet(w.ctx, models.WEBHOOKKEY)
	if err != nil {
		return err
	}
	w.RWMutex.Lock()
	json.Unmarshal([]byte(cval), &w.webhooks)
	w.RWMutex.Unlock()
	logger.Infof("timer: sync wbhooks done number: %d", len(w.webhooks))
	return nil
}

func (w *WebhookCacheType) GetWebhooks() []*models.Webhook {
	w.RWMutex.RLock()
	defer w.RWMutex.RUnlock()
	return w.webhooks
}
