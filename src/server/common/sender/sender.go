package sender

import (
	"bytes"
	"html/template"

	"github.com/toolkits/pkg/slice"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
)

type (
	// Sender 发送消息通知的接口
	Sender interface {
		Send(ctx MessageContext)

		// SendRaw 发送原始消息,目前在notifyMaintainer时使用
		SendRaw(users []*models.User, title, message string)
	}

	// MessageContext 一个event所生成的告警通知的上下文
	MessageContext struct {
		Users []*models.User
		Rule  *models.AlertRule
		Event *models.AlertCurEvent
	}
)

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
	case models.FeishuCard:
		return &FeishuCardSender{tpl: tpls["feishucard.tpl"]}
	case models.Email:
		return &EmailSender{subjectTpl: tpls["subject.tpl"], contentTpl: tpls["mailbody.tpl"]}
	case models.Mm:
		return &MmSender{tpl: tpls["mm.tpl"]}
	case models.Telegram:
		return &TelegramSender{tpl: tpls["telegram.tpl"]}
	}
	return nil
}

func BuildMessageContext(rule *models.AlertRule, event *models.AlertCurEvent, uids []int64) MessageContext {
	users := memsto.UserCache.GetByUserIds(uids)
	return MessageContext{
		Rule:  rule,
		Event: event,
		Users: users,
	}
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
