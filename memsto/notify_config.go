package memsto

import (
	"bytes"
	"fmt"

	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type NotifyConfigCacheType struct {
	ctx      *ctx.Context
	webhooks []*models.Webhook
	smtp     aconf.SMTPConfig
	script   models.NotifyScript
	ibex     aconf.Ibex
	macroMap map[string]string
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
		ctx:      ctx,
		macroMap: make(map[string]string),
	}
	w.SyncNotifyConfigs()
	return w
}

func (w *NotifyConfigCacheType) SyncNotifyConfigs() {
	err := w.syncNotifyConfigs()
	if err != nil {
		logger.Warning("failed to sync webhooks:", err)
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
	mvCache := w.ctx.Ctx.Value(MacroVariableKey).(*MacroVariableCache)
	if mvCache == nil { //for edge
		ret, err := poster.GetByUrls[map[string]string](w.ctx, "/v1/n9e/macro-variable")
		if err != nil {
			return errors.WithMessage(err, "failed to sync notify configs(macroMap).")
		}
		w.macroMap = ret
	} else {
		w.macroMap = mvCache.Get()
	}

	webhooksSyncFun := func() error {
		return w.initToml(&w.webhooks, models.WEBHOOKKEY, "webhooks", func() int {
			return len(w.webhooks)
		})
	}

	smtpSyncFun := func() error {
		return w.initToml(&w.smtp, models.SMTP, "smtp", func() int {
			if "" == w.smtp.Host {
				return 0
			}
			return 1
		})
	}

	notifyScriptSyncFun := func() error {
		return w.initToml(&w.script, models.NOTIFYSCRIPT, "notify_script", func() int {
			if "" == w.script.Content {
				return 0
			}
			return 1
		})
	}

	ibexSyncFun := func() error {
		return w.initToml(&w.ibex, models.IBEX, "ibex", func() int {
			if "" == w.ibex.Address {
				return 0
			}
			return 1
		})
	}
	return MutiErrorHook("error:failed to get syncNotifyConfigs", webhooksSyncFun, smtpSyncFun, notifyScriptSyncFun, ibexSyncFun)
}

func MutiErrorHook(errorPrefix string, fs ...func() error) error {
	var errs []error = make([]error, 0, len(fs))
	for i := range fs {
		if err := fs[i](); err != nil {
			errs = append(errs, err)
			err = nil
		}
	}
	if len(errs) < 1 {
		return nil
	} else {
		b := bytes.Buffer{}
		for i := range errs {
			b.WriteString(fmt.Sprintf("error[%v]:", i))
			b.WriteString(errs[i].Error())
		}
		return fmt.Errorf(errorPrefix+" %s", b.String())
	}
}

func (w *NotifyConfigCacheType) initToml(target any, cKey string, dumperKey string, count func() int) error {
	start := time.Now()
	//cval, err := models.ConfigsGet(w.ctx, cKey)
	cvalFun := func() (string, string, error) {
		cval, err := models.ConfigsGet(w.ctx, cKey)
		return cKey, cval, err
	}
	cval, err := models.ConfigsGetDecryption(cvalFun, w.macroMap)
	if err != nil {
		dumper.PutSyncRecord(dumperKey, start.Unix(), -1, -1, "failed to query configs."+cKey+": "+err.Error())
		return errors.WithMessage(err, "failed to get config in initTomal. cKey ('"+cKey+"')value is invalid")
	}

	if strings.TrimSpace(cval) != "" {
		err = toml.Unmarshal([]byte(cval), target)
		if err != nil {
			dumper.PutSyncRecord(dumperKey, start.Unix(), -1, -1, "failed to unmarshal configs."+cKey+": "+err.Error())
			err = fmt.Errorf("failed to unmarshal config. '%s':'%s' error:%v", cKey, cval, err)
			logger.Error(err)
		}
	}
	dumper.PutSyncRecord(dumperKey, start.Unix(), time.Since(start).Milliseconds(), count(), "success, "+cKey+":\n"+cval)
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
