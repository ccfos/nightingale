package sender

import (
	"html/template"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
)

type wecomMarkdown struct {
	Content string `json:"content"`
}

type wecom struct {
	Msgtype  string        `json:"msgtype"`
	Markdown wecomMarkdown `json:"markdown"`
}

var (
	_ CallBacker = (*WecomSender)(nil)
)

type WecomSender struct {
	tpl *template.Template
}

func (ws *WecomSender) CallBack(ctx CallBackContext) {
	if len(ctx.Events) == 0 || len(ctx.CallBackURL) == 0 {
		return
	}

	message := BuildTplMessage(models.Wecom, ws.tpl, ctx.Events)
	body := wecom{
		Msgtype: "markdown",
		Markdown: wecomMarkdown{
			Content: message,
		},
	}

	doSendAndRecord(ctx.Ctx, ctx.CallBackURL, ctx.CallBackURL, body, "callback",
		ctx.Stats, ctx.Events[0])
	ctx.Stats.AlertNotifyTotal.WithLabelValues("rule_callback").Inc()
}

func (ws *WecomSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}
	urls, tokens := ws.extract(ctx.Users)
	message := BuildTplMessage(models.Wecom, ws.tpl, ctx.Events)
	for i, url := range urls {
		body := wecom{
			Msgtype: "markdown",
			Markdown: wecomMarkdown{
				Content: message,
			},
		}
		doSendAndRecord(ctx.Ctx, url, tokens[i], body, models.Wecom, ctx.Stats, ctx.Events[0])
	}
}

func (ws *WecomSender) extract(users []*models.User) ([]string, []string) {
	urls := make([]string, 0, len(users))
	tokens := make([]string, 0, len(users))
	for _, user := range users {
		if token, has := user.ExtractToken(models.Wecom); has {
			url := token
			if !strings.HasPrefix(token, "https://") && !strings.HasPrefix(token, "http://") {
				url = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=" + token
			}
			urls = append(urls, url)
			tokens = append(tokens, token)
		}
	}
	return urls, tokens
}
