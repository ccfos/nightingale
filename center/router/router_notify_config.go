package router

import (
	"encoding/json"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/sender"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/pelletier/go-toml/v2"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) webhookGets(c *gin.Context) {
	var webhooks []models.Webhook
	cval, err := models.ConfigsGet(rt.Ctx, models.WEBHOOKKEY)
	ginx.Dangerous(err)
	if cval == "" {
		ginx.NewRender(c).Data(webhooks, nil)
		return
	}

	err = json.Unmarshal([]byte(cval), &webhooks)
	ginx.NewRender(c).Data(webhooks, err)
}

func (rt *Router) webhookPuts(c *gin.Context) {
	var webhooks []models.Webhook
	ginx.BindJSON(c, &webhooks)
	for i := 0; i < len(webhooks); i++ {
		for k, v := range webhooks[i].HeaderMap {
			webhooks[i].Headers = append(webhooks[i].Headers, k)
			webhooks[i].Headers = append(webhooks[i].Headers, v)
		}
	}

	data, err := json.Marshal(webhooks)
	ginx.Dangerous(err)

	ginx.NewRender(c).Message(models.ConfigsSet(rt.Ctx, models.WEBHOOKKEY, string(data)))
}

func (rt *Router) notifyScriptGet(c *gin.Context) {
	var notifyScript models.NotifyScript
	cval, err := models.ConfigsGet(rt.Ctx, models.NOTIFYSCRIPT)
	ginx.Dangerous(err)

	if cval == "" {
		ginx.NewRender(c).Data(notifyScript, nil)
		return
	}

	err = json.Unmarshal([]byte(cval), &notifyScript)
	ginx.NewRender(c).Data(notifyScript, err)
}

func (rt *Router) notifyScriptPut(c *gin.Context) {
	var notifyScript models.NotifyScript
	ginx.BindJSON(c, &notifyScript)

	data, err := json.Marshal(notifyScript)
	ginx.Dangerous(err)

	ginx.NewRender(c).Message(models.ConfigsSet(rt.Ctx, models.NOTIFYSCRIPT, string(data)))
}

func (rt *Router) notifyChannelGets(c *gin.Context) {
	var notifyChannels []models.NotifyChannel
	cval, err := models.ConfigsGet(rt.Ctx, models.NOTIFYCHANNEL)
	ginx.Dangerous(err)
	if cval == "" {
		ginx.NewRender(c).Data(notifyChannels, nil)
		return
	}

	err = json.Unmarshal([]byte(cval), &notifyChannels)
	ginx.NewRender(c).Data(notifyChannels, err)
}

func (rt *Router) notifyChannelPuts(c *gin.Context) {
	var notifyChannels []models.NotifyChannel
	ginx.BindJSON(c, &notifyChannels)

	channels := []string{models.Dingtalk, models.Wecom, models.Feishu, models.Mm, models.Telegram, models.Email}

	m := make(map[string]struct{})
	for _, v := range notifyChannels {
		m[v.Ident] = struct{}{}
	}

	for _, v := range channels {
		if _, ok := m[v]; !ok {
			ginx.Bomb(200, "channel %s ident can not modify", v)
		}
	}

	data, err := json.Marshal(notifyChannels)
	ginx.Dangerous(err)

	ginx.NewRender(c).Message(models.ConfigsSet(rt.Ctx, models.NOTIFYCHANNEL, string(data)))
}

func (rt *Router) notifyContactGets(c *gin.Context) {
	var notifyContacts []models.NotifyContact
	cval, err := models.ConfigsGet(rt.Ctx, models.NOTIFYCONTACT)
	ginx.Dangerous(err)
	if cval == "" {
		ginx.NewRender(c).Data(notifyContacts, nil)
		return
	}

	err = json.Unmarshal([]byte(cval), &notifyContacts)
	ginx.NewRender(c).Data(notifyContacts, err)
}

func (rt *Router) notifyContactPuts(c *gin.Context) {
	var notifyContacts []models.NotifyContact
	ginx.BindJSON(c, &notifyContacts)

	keys := []string{models.DingtalkKey, models.WecomKey, models.FeishuKey, models.MmKey, models.TelegramKey}

	m := make(map[string]struct{})
	for _, v := range notifyContacts {
		m[v.Ident] = struct{}{}
	}

	for _, v := range keys {
		if _, ok := m[v]; !ok {
			ginx.Bomb(200, "contact %s ident can not modify", v)
		}
	}

	data, err := json.Marshal(notifyContacts)
	ginx.Dangerous(err)

	ginx.NewRender(c).Message(models.ConfigsSet(rt.Ctx, models.NOTIFYCONTACT, string(data)))
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

func (rt *Router) notifyConfigGet(c *gin.Context) {
	key := ginx.QueryStr(c, "ckey")
	cval, err := models.ConfigsGet(rt.Ctx, key)
	if cval == "" {
		switch key {
		case models.IBEX:
			cval = DefaultIbex
		case models.SMTP:
			cval = DefaultSMTP
		}
	}
	ginx.NewRender(c).Data(cval, err)
}

func (rt *Router) notifyConfigPut(c *gin.Context) {
	var f models.Configs
	ginx.BindJSON(c, &f)
	switch f.Ckey {
	case models.SMTP:
		var smtp aconf.SMTPConfig
		err := toml.Unmarshal([]byte(f.Cval), &smtp)
		ginx.Dangerous(err)
	case models.IBEX:
		var ibex aconf.Ibex
		err := toml.Unmarshal([]byte(f.Cval), &ibex)
		ginx.Dangerous(err)
	default:
		ginx.Bomb(200, "key %s can not modify", f.Ckey)
	}

	err := models.ConfigsSet(rt.Ctx, f.Ckey, f.Cval)
	if err != nil {
		ginx.Bomb(200, err.Error())
	}

	if f.Ckey == models.SMTP {
		// 重置邮件发送器
		var smtp aconf.SMTPConfig
		err := toml.Unmarshal([]byte(f.Cval), &smtp)
		ginx.Dangerous(err)

		if smtp.Host == "" || smtp.Port == 0 {
			ginx.Bomb(200, "smtp host or port can not be empty")
		}

		go sender.RestartEmailSender(smtp)
	}

	ginx.NewRender(c).Message(nil)
}
