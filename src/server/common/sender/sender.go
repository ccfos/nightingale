package sender

import (
	"bytes"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/toolkits/pkg/slice"
	"html/template"

	"github.com/didi/nightingale/v5/src/models"
)

type MessageContext struct {
	Users []*models.User
	Rule  *models.AlertRule
	Event *models.AlertCurEvent
}

type Sender interface {
	Send(ctx MessageContext)
	SendRaw(users []*models.User, title, message string)
}

func NewSender(key string, tpls map[string]*template.Template) Sender {
	if !slice.ContainsString(config.C.Alerting.NotifyBuiltinChannels, key) {
		return nil
	}

	switch key {
	case models.Dingtalk:
		return &DingtalkSender{tpl: tpls["dingtalk.tpl"]}
	case models.Wecom:
		return &WecomSender{tpl: tpls["wecom.tpl"]}
	case models.Feishu:
		return &FeishuSender{tpl: tpls["feishu.tpl"]}
	case models.Email:
		return &EmailSender{subjectTpl: tpls["subject.tpl"], contentTpl: tpls["mailbody.tpl"]}
	case models.Mm:
		return &MmSender{tpl: tpls["mm.tpl"]}
	case models.Telegram:
		return &TelegramSender{tpl: tpls["telegram.tpl"]}
	}
	return nil
}

func BuildTplMessage(tpl *template.Template, event *models.AlertCurEvent) string {
	if tpl == nil {
		return "tpl for current sender not found, please check configuration"
	}
	var body bytes.Buffer
	if err := tpl.Execute(&body, event); err != nil {
		return err.Error()
	}
	return body.String()
}
