package sender

import (
	"html/template"
	"strings"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
)

type TelegramMessage struct {
	Text   string
	Tokens []string
	Stats  *astats.Stats
}

type telegram struct {
	ParseMode string `json:"parse_mode"`
	Text      string `json:"text"`
}

var (
	_ CallBacker = (*TelegramSender)(nil)
)

type TelegramSender struct {
	tpl *template.Template
}

func (ts *TelegramSender) CallBack(ctx CallBackContext) {
	if len(ctx.Events) == 0 || len(ctx.CallBackURL) == 0 {
		return
	}

	message := BuildTplMessage(models.Telegram, ts.tpl, ctx.Events)
	SendTelegram(ctx.Ctx, TelegramMessage{
		Text:   message,
		Tokens: []string{ctx.CallBackURL},
		Stats:  ctx.Stats,
	}, ctx.Events[0])

	ctx.Stats.AlertNotifyTotal.WithLabelValues("rule_callback").Inc()
}

func (ts *TelegramSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}
	tokens := ts.extract(ctx.Users)
	message := BuildTplMessage(models.Telegram, ts.tpl, ctx.Events)

	SendTelegram(ctx.Ctx, TelegramMessage{
		Text:   message,
		Tokens: tokens,
		Stats:  ctx.Stats,
	}, ctx.Events[0])
}

func (ts *TelegramSender) extract(users []*models.User) []string {
	tokens := make([]string, 0, len(users))
	for _, user := range users {
		if token, has := user.ExtractToken(models.Telegram); has {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func SendTelegram(ctx *ctx.Context, message TelegramMessage, event *models.AlertCurEvent) {
	for i := 0; i < len(message.Tokens); i++ {
		if !strings.Contains(message.Tokens[i], "/") && !strings.HasPrefix(message.Tokens[i], "https://") {
			logger.Errorf("telegram_sender: result=fail invalid token=%s", message.Tokens[i])
			continue
		}
		var url string
		if strings.HasPrefix(message.Tokens[i], "https://") || strings.HasPrefix(message.Tokens[i], "http://") {
			url = message.Tokens[i]
		} else {
			array := strings.Split(message.Tokens[i], "/")
			if len(array) != 2 {
				logger.Errorf("telegram_sender: result=fail invalid token=%s", message.Tokens[i])
				continue
			}
			botToken := array[0]
			chatId := array[1]
			url = "https://api.telegram.org/bot" + botToken + "/sendMessage?chat_id=" + chatId
		}
		body := telegram{
			ParseMode: "markdown",
			Text:      message.Text,
		}

		doSendAndRecord(ctx, url, message.Tokens[i], body, models.Telegram, message.Stats, event)
	}
}
