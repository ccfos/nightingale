package sender

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
)

type feishuContent struct {
	Text string `json:"text"`
}

type feishuAt struct {
	AtMobiles []string `json:"atMobiles"`
	IsAtAll   bool     `json:"isAtAll"`
}

type feishu struct {
	Msgtype string        `json:"msg_type"`
	Content feishuContent `json:"content"`
	At      feishuAt      `json:"at"`
}

var (
	_ CallBacker = (*FeishuSender)(nil)
)

type FeishuSender struct {
	tpl *template.Template
}

func (fs *FeishuSender) CallBack(ctx CallBackContext) {
	if len(ctx.Events) == 0 || len(ctx.CallBackURL) == 0 {
		return
	}

	ats := ExtractAtsParams(ctx.CallBackURL)
	message := BuildTplMessage(models.Feishu, fs.tpl, ctx.Events)

	if len(ats) > 0 {
		atTags := ""
		for _, at := range ats {
			atTags += fmt.Sprintf("<at user_id=\"%s\"></at> ", at)
		}
		message = atTags + message
	}

	body := feishu{
		Msgtype: "text",
		Content: feishuContent{
			Text: message,
		},
	}

	doSendAndRecord(ctx.Ctx, ctx.CallBackURL, ctx.CallBackURL, body, "callback",
		ctx.Stats, ctx.Events[0])
	ctx.Stats.AlertNotifyTotal.WithLabelValues("rule_callback").Inc()
}

func (fs *FeishuSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}
	urls, ats, tokens := fs.extract(ctx.Users)
	message := BuildTplMessage(models.Feishu, fs.tpl, ctx.Events)
	for i, url := range urls {
		body := feishu{
			Msgtype: "text",
			Content: feishuContent{
				Text: message,
			},
		}
		if !strings.Contains(url, "noat=1") {
			body.At = feishuAt{
				AtMobiles: ats,
				IsAtAll:   false,
			}
		}
		doSendAndRecord(ctx.Ctx, url, tokens[i], body, models.Feishu, ctx.Stats, ctx.Events[0])
	}
}

func (fs *FeishuSender) extract(users []*models.User) ([]string, []string, []string) {
	urls := make([]string, 0, len(users))
	ats := make([]string, 0, len(users))
	tokens := make([]string, 0, len(users))

	for _, user := range users {
		if user.Phone != "" {
			ats = append(ats, user.Phone)
		}
		if token, has := user.ExtractToken(models.Feishu); has {
			url := token
			if !strings.HasPrefix(token, "https://") && !strings.HasPrefix(token, "http://") {
				url = "https://open.feishu.cn/open-apis/bot/v2/hook/" + token
			}
			urls = append(urls, url)
			tokens = append(tokens, token)
		}
	}
	return urls, ats, tokens
}
