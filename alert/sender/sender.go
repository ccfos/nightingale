package sender

import (
	"bytes"
	"html/template"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
)

type (
	// Sender 发送消息通知的接口
	Sender interface {
		Send(ctx MessageContext)
	}

	// MessageContext 一个event所生成的告警通知的上下文
	MessageContext struct {
		Users  []*models.User
		Rule   *models.AlertRule
		Events []*models.AlertCurEvent
	}
)

func NewSender(key string, tpls map[string]*template.Template, smtp aconf.SMTPConfig) Sender {
	switch key {
	case models.Dingtalk:
		return &DingtalkSender{tpl: tpls[models.Dingtalk]}
	case models.Wecom:
		return &WecomSender{tpl: tpls[models.Wecom]}
	case models.Feishu:
		return &FeishuSender{tpl: tpls[models.Feishu]}
	case models.FeishuCard:
		return &FeishuCardSender{tpl: tpls[models.FeishuCard]}
	case models.Email:
		return &EmailSender{subjectTpl: tpls["mailsubject"], contentTpl: tpls[models.Email], smtp: smtp}
	case models.Mm:
		return &MmSender{tpl: tpls[models.Mm]}
	case models.Telegram:
		return &TelegramSender{tpl: tpls[models.Telegram]}
	}
	return nil
}

func BuildMessageContext(rule *models.AlertRule, events []*models.AlertCurEvent, uids []int64, userCache *memsto.UserCacheType) MessageContext {
	users := userCache.GetByUserIds(uids)
	return MessageContext{
		Rule:   rule,
		Events: events,
		Users:  users,
	}
}

type BuildTplMessageFunc func(tpl *template.Template, events []*models.AlertCurEvent) string

var BuildTplMessage BuildTplMessageFunc = buildTplMessage

func buildTplMessage(tpl *template.Template, events []*models.AlertCurEvent) string {
	if tpl == nil {
		return "tpl for current sender not found, please check configuration"
	}

	var content string
	for _, event := range events {
		var body bytes.Buffer
		if err := tpl.Execute(&body, event); err != nil {
			return err.Error()
		}
		content += body.String() + "\n\n"
	}

	return content
}
