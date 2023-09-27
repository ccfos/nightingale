package memsto

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/BurntSushi/toml"
	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type NotifyConfigCacheType struct {
	ctx         *ctx.Context
	configCache *ConfigCache
	webhooks    []*models.Webhook
	smtp        aconf.SMTPConfig
	script      models.NotifyScript
	ibex        aconf.Ibex

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

func NewNotifyConfigCache(ctx *ctx.Context, configCache *ConfigCache) *NotifyConfigCacheType {
	w := &NotifyConfigCacheType{
		ctx:         ctx,
		configCache: configCache,
	}
	w.SyncNotifyConfigs()
	return w
}

func (w *NotifyConfigCacheType) SyncNotifyConfigs() {
	err := w.syncNotifyConfigs()
	if err != nil {
		logger.Error("failed to sync webhooks:", err)
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
	start := time.Now()
	userVariableMap := make(map[string]string)

	if w.configCache == nil { //for edge and alert
		var err error
		userVariableMap, err = poster.GetByUrls[map[string]string](w.ctx, "/v1/n9e/user-variable/decrypt")
		if err != nil {
			return errors.WithMessage(err, "failed to get configs.")
		}
	} else {
		userVariableMap = w.configCache.Get()
	}

	w.RWMutex.Lock()
	defer w.RWMutex.Unlock()

	cval, err := models.ConfigsGet(w.ctx, models.WEBHOOKKEY)
	if err != nil {
		dumper.PutSyncRecord("webhooks", start.Unix(), -1, -1, "failed to query configs.webhook: "+err.Error())
		return err
	}

	if strings.TrimSpace(cval) != "" {
		err = json.Unmarshal([]byte(cval), &w.webhooks)
		if err != nil {
			dumper.PutSyncRecord("webhooks", start.Unix(), -1, -1, "failed to unmarshal configs.webhook: "+err.Error())
			logger.Errorf("failed to unmarshal webhooks:%s error:%v", cval, err)
		}
	}

	dumper.PutSyncRecord("webhooks", start.Unix(), time.Since(start).Milliseconds(), len(w.webhooks), "success, webhooks:\n"+cval)

	start = time.Now()
	cval, err = models.ConfigsGet(w.ctx, models.SMTP)
	if err != nil {
		dumper.PutSyncRecord("smtp", start.Unix(), -1, -1, "failed to query configs.smtp_config: "+err.Error())
		return err
	}

	tplxBuffer, replaceErr := tplx.ReplaceMacroVariables(models.SMTP, cval, userVariableMap)
	if replaceErr != nil {
		return fmt.Errorf("failed to ReplaceMacroVariables %q:%q. error:%v", models.SMTP, cval, replaceErr)
	}
	if tplxBuffer == nil {
		return fmt.Errorf("unexpected error. %q:%q, userVariableMap:%+v,tplxBuffer(pointer):%v", models.SMTP, cval, userVariableMap, tplxBuffer)
	}
	cval = tplxBuffer.String()

	if strings.TrimSpace(cval) != "" {
		err = toml.Unmarshal([]byte(cval), &w.smtp)
		if err != nil {
			dumper.PutSyncRecord("smtp", start.Unix(), -1, -1, "failed to unmarshal configs.smtp_config: "+err.Error())
			logger.Errorf("failed to unmarshal smtp:%s error:%v", cval, err)
		}
	}

	dumper.PutSyncRecord("smtp", start.Unix(), time.Since(start).Milliseconds(), 1, "success, smtp_config:\n"+cval)

	start = time.Now()
	cval, err = models.ConfigsGet(w.ctx, models.NOTIFYSCRIPT)
	if err != nil {
		dumper.PutSyncRecord("notify_script", start.Unix(), -1, -1, "failed to query configs.notify_script: "+err.Error())
		return err
	}

	if strings.TrimSpace(cval) != "" {
		err = json.Unmarshal([]byte(cval), &w.script)
		if err != nil {
			dumper.PutSyncRecord("notify_script", start.Unix(), -1, -1, "failed to unmarshal configs.notify_script: "+err.Error())
			logger.Errorf("failed to unmarshal notify script:%s error:%v", cval, err)
		}
	}

	dumper.PutSyncRecord("notify_script", start.Unix(), time.Since(start).Milliseconds(), 1, "success, notify_script:\n"+cval)

	start = time.Now()
	cval, err = models.ConfigsGet(w.ctx, models.IBEX)
	if err != nil {
		dumper.PutSyncRecord("ibex", start.Unix(), -1, -1, "failed to query configs.ibex_server: "+err.Error())
		return err
	}

	if strings.TrimSpace(cval) != "" {
		err = toml.Unmarshal([]byte(cval), &w.ibex)
		if err != nil {
			dumper.PutSyncRecord("ibex", start.Unix(), -1, -1, "failed to unmarshal configs.ibex_server: "+err.Error())
			logger.Errorf("failed to unmarshal ibex:%s error:%v", cval, err)
		}
	} else {
		err = toml.Unmarshal([]byte(DefaultIbex), &w.ibex)
		if err != nil {
			dumper.PutSyncRecord("ibex", start.Unix(), -1, -1, "failed to unmarshal configs.ibex_server: "+err.Error())
			logger.Errorf("failed to unmarshal ibex:%s error:%v", cval, err)
		}
	}

	dumper.PutSyncRecord("ibex", start.Unix(), time.Since(start).Milliseconds(), 1, "success, ibex_server config:\n"+cval)

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

func (w *NotifyConfigCacheType) GetComfigCache() *ConfigCache {
	w.RWMutex.RLock()
	defer w.RWMutex.RUnlock()
	return w.configCache
}
