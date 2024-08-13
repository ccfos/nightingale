package sender

import (
	"html/template"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
)

type dingtalkMarkdown struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type dingtalkAt struct {
	AtMobiles []string `json:"atMobiles"`
	IsAtAll   bool     `json:"isAtAll"`
}

type dingtalk struct {
	Msgtype  string           `json:"msgtype"`
	Markdown dingtalkMarkdown `json:"markdown"`
	At       dingtalkAt       `json:"at"`
}

var (
	_ CallBacker = (*DingtalkSender)(nil)
)

type DingtalkSender struct {
	tpl *template.Template
}

func (ds *DingtalkSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}

	urls, ats, tokens := ds.extract(ctx.Users)
	if len(urls) == 0 {
		return
	}
	message := BuildTplMessage(models.Dingtalk, ds.tpl, ctx.Events)

	for i, url := range urls {
		var body dingtalk
		// NoAt in url
		if strings.Contains(url, "noat=1") {
			body = dingtalk{
				Msgtype: "markdown",
				Markdown: dingtalkMarkdown{
					Title: ctx.Events[0].RuleName,
					Text:  message,
				},
			}
		} else {
			body = dingtalk{
				Msgtype: "markdown",
				Markdown: dingtalkMarkdown{
					Title: ctx.Events[0].RuleName,
					Text:  message + "\n" + strings.Join(ats, " "),
				},
				At: dingtalkAt{
					AtMobiles: ats,
					IsAtAll:   false,
				},
			}
		}

		doSendAndRecord(ctx.Ctx, url, tokens[i], body, models.Dingtalk, ctx.Stats, ctx.Events[0])
	}
}

func (ds *DingtalkSender) CallBack(ctx CallBackContext) {
	if len(ctx.Events) == 0 || len(ctx.CallBackURL) == 0 {
		return
	}

	body := dingtalk{
		Msgtype: "markdown",
		Markdown: dingtalkMarkdown{
			Title: ctx.Events[0].RuleName,
		},
	}

	ats := ExtractAtsParams(ctx.CallBackURL)
	message := BuildTplMessage(models.Dingtalk, ds.tpl, ctx.Events)

	if len(ats) > 0 {
		body.Markdown.Text = message + "\n@" + strings.Join(ats, "@")
		body.At = dingtalkAt{
			AtMobiles: ats,
			IsAtAll:   false,
		}
	} else {
		// NoAt in url
		body.Markdown.Text = message
	}

	doSendAndRecord(ctx.Ctx, ctx.CallBackURL, ctx.CallBackURL, body,
		"callback", ctx.Stats, ctx.Events[0])

	ctx.Stats.AlertNotifyTotal.WithLabelValues("rule_callback").Inc()
}

// extract urls and ats from Users
func (ds *DingtalkSender) extract(users []*models.User) ([]string, []string, []string) {
	urls := make([]string, 0, len(users))
	ats := make([]string, 0, len(users))
	tokens := make([]string, 0, len(users))

	for _, user := range users {
		if user.Phone != "" {
			ats = append(ats, "@"+user.Phone)
		}
		if token, has := user.ExtractToken(models.Dingtalk); has {
			url := token
			if !strings.HasPrefix(token, "https://") && !strings.HasPrefix(token, "http://") {
				url = "https://oapi.dingtalk.com/robot/send?access_token=" + token
			}
			urls = append(urls, url)
			tokens = append(tokens, token)
		}
	}
	return urls, ats, tokens
}
