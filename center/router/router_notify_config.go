package router

import (
	"encoding/json"

	"github.com/ccfos/nightingale/v6/models"

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
