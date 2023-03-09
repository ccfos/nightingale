package memsto

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
)

type NotifyScriptCacheType struct {
	ctx    *ctx.Context
	config models.NotifyScript
	sync.RWMutex
}

func NewNotifyScript(ctx *ctx.Context) *NotifyScriptCacheType {
	w := &NotifyScriptCacheType{
		ctx: ctx,
	}
	w.SyncNotifyScript()
	return w
}

func (w *NotifyScriptCacheType) SyncNotifyScript() {
	err := w.syncNotifyScript()
	if err != nil {
		logger.Errorf("failed to sync notify config:", err)
	}

	go w.loopSyncNotifyScript()
}

func (w *NotifyScriptCacheType) loopSyncNotifyScript() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := w.syncNotifyScript(); err != nil {
			logger.Warning("failed to sync notify config:", err)
		}
	}
}

func (w *NotifyScriptCacheType) syncNotifyScript() error {
	cval, err := models.ConfigsGet(w.ctx, models.NOTIFYSCRIPT)
	if err != nil {
		return err
	}
	w.RWMutex.Lock()
	json.Unmarshal([]byte(cval), &w.config)
	w.RWMutex.Unlock()
	logger.Infof("timer: sync notify done")
	return nil
}

func (w *NotifyScriptCacheType) GetNotifyScript() models.NotifyScript {
	w.RWMutex.RLock()
	defer w.RWMutex.RUnlock()
	return w.config
}
