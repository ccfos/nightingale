package sender

import (
	"html/template"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/toolkits/pkg/logger"
)

type TelegramMessage struct {
	Text   string
	Tokens []string
}

type telegram struct {
	ParseMode string `json:"parse_mode"`
	Text      string `json:"text"`
}

type TelegramSender struct {
	tpl *template.Template
}

func (ts *TelegramSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || ctx.Rule == nil || ctx.Event == nil {
		return
	}
	tokens := ts.extract(ctx.Users)
	message := BuildTplMessage(ts.tpl, ctx.Event)

	SendTelegram(TelegramMessage{
		Text:   message,
		Tokens: tokens,
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
		if strings.HasPrefix(message.Tokens[i], "https://") {
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

		res, code, err := poster.PostJSON(url, time.Second*5, body, 3)
		if err != nil {
			logger.Errorf("telegram_sender: result=fail url=%s code=%d error=%v response=%s", url, code, err, string(res))
		} else {
			logger.Infof("telegram_sender: result=succ url=%s code=%d response=%s", url, code, string(res))
		}
	}
}
