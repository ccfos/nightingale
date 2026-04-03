package sender

import (
	"html/template"

	"github.com/ccfos/nightingale/v6/models"
)

type slackPayload struct {
	Text string `json:"text"`
}

type SlackWebhookSender struct {
	tpl *template.Template
}

func (s *SlackWebhookSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}

	urls := s.extract(ctx.Users)
	if len(urls) == 0 {
		return
	}

	message := BuildTplMessage(models.SlackWebhook, s.tpl, ctx.Events)
	body := slackPayload{Text: message}

	for _, url := range urls {
		doSendAndRecord(ctx.Ctx, url, url, body, models.SlackWebhook, ctx.Stats, ctx.Events)
	}
}

func (s *SlackWebhookSender) extract(users []*models.User) []string {
	urls := make([]string, 0, len(users))
	for _, user := range users {
		if token, has := user.ExtractToken(models.SlackWebhook); has {
			urls = append(urls, token)
		}
	}
	return urls
}
