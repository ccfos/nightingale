package sender

import (
	"html/template"
	"strings"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"

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

type TelegramSender struct {
	tpl *template.Template
}

func (ts *TelegramSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}
	tokens := ts.extract(ctx.Users)
	message := BuildTplMessage(models.Telegram, ts.tpl, ctx.Events)

	SendTelegram(TelegramMessage{
		Text:   message,
		Tokens: tokens,
		Stats:  ctx.Stats,
	})
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

func SendTelegram(message TelegramMessage) {
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

		doSend(url, body, models.Telegram, message.Stats)
	}
}
