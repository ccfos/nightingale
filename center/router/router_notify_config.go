package router

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/sender"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/gin-gonic/gin"
	"github.com/pelletier/go-toml/v2"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/str"
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
		webhooks[i].Headers = []string{}
		if len(webhooks[i].HeaderMap) > 0 {
			for k, v := range webhooks[i].HeaderMap {
				webhooks[i].Headers = append(webhooks[i].Headers, k)
				webhooks[i].Headers = append(webhooks[i].Headers, v)
			}
		}
	}

	data, err := json.Marshal(webhooks)
	ginx.Dangerous(err)
	username := c.MustGet("username").(string)
	ginx.NewRender(c).Message(models.ConfigsSetWithUname(rt.Ctx, models.WEBHOOKKEY, string(data), username))
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
	username := c.MustGet("username").(string)
	ginx.NewRender(c).Message(models.ConfigsSetWithUname(rt.Ctx, models.NOTIFYSCRIPT, string(data), username))
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
	username := c.MustGet("username").(string)
	ginx.NewRender(c).Message(models.ConfigsSetWithUname(rt.Ctx, models.NOTIFYCHANNEL, string(data), username))
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
	username := c.MustGet("username").(string)
	ginx.NewRender(c).Message(models.ConfigsSetWithUname(rt.Ctx, models.NOTIFYCONTACT, string(data), username))
}

func (rt *Router) notifyConfigGet(c *gin.Context) {
	key := ginx.QueryStr(c, "ckey")
	cval, err := models.ConfigsGet(rt.Ctx, key)
	if cval == "" {
		switch key {
		case models.IBEX:
			cval = memsto.DefaultIbex
		case models.SMTP:
			cval = memsto.DefaultSMTP
		}
	}
	ginx.NewRender(c).Data(cval, err)
}

func (rt *Router) notifyConfigPut(c *gin.Context) {
	var f models.Configs
	ginx.BindJSON(c, &f)
	userVariableMap := rt.NotifyConfigCache.ConfigCache.Get()
	text := tplx.ReplaceTemplateUseText(f.Ckey, f.Cval, userVariableMap)
	switch f.Ckey {
	case models.SMTP:
		var smtp aconf.SMTPConfig
		err := toml.Unmarshal([]byte(text), &smtp)
		ginx.Dangerous(err)
	case models.IBEX:
		var ibex aconf.Ibex
		err := toml.Unmarshal([]byte(f.Cval), &ibex)
		ginx.Dangerous(err)
	default:
		ginx.Bomb(200, "key %s can not modify", f.Ckey)
	}
	username := c.MustGet("username").(string)
	//insert or update build-in config
	ginx.Dangerous(models.ConfigsSetWithUname(rt.Ctx, f.Ckey, f.Cval, username))
	if f.Ckey == models.SMTP {
		// 重置邮件发送器

		smtp, errSmtp := SmtpValidate(text)
		ginx.Dangerous(errSmtp)
		go sender.RestartEmailSender(smtp)
	}

	ginx.NewRender(c).Message(nil)
}
func SmtpValidate(text string) (aconf.SMTPConfig, error) {
	var smtp aconf.SMTPConfig
	var err error

	err = toml.Unmarshal([]byte(text), &smtp)
	if err != nil {
		return smtp, err
	}
	if smtp.Host == "" || smtp.Port == 0 {
		return smtp, fmt.Errorf("smtp host or port can not be empty")
	}
	return smtp, err
}

type form struct {
	models.Configs
	Email string `json:"email"`
}

// After configuring the aconf.SMTPConfig, users can choose to perform a test. In this test, the function attempts to send an email
func (rt *Router) attemptSendEmail(c *gin.Context) {
	var f form
	ginx.BindJSON(c, &f)

	if f.Email = strings.TrimSpace(f.Email); f.Email == "" || !str.IsMail(f.Email) {
		ginx.Bomb(200, "email(%s) invalid", f.Email)
	}

	if f.Ckey != models.SMTP {
		ginx.Bomb(200, "config(%v) invalid", f)
	}
	userVariableMap := rt.NotifyConfigCache.ConfigCache.Get()
	text := tplx.ReplaceTemplateUseText(f.Ckey, f.Cval, userVariableMap)
	smtp, err := SmtpValidate(text)
	ginx.Dangerous(err)

	ginx.NewRender(c).Message(sender.SendEmail("Email test", "email content", []string{f.Email}, smtp))

}
