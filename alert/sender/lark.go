package sender

import (
	"html/template"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
)

var (
	_ CallBacker = (*LarkSender)(nil)
)

type LarkSender struct {
	tpl *template.Template
}

func (lk *LarkSender) CallBack(ctx CallBackContext) {
	if len(ctx.Events) == 0 || len(ctx.CallBackURL) == 0 {
		return
	}

	body := feishu{
		Msgtype: "text",
		Content: feishuContent{
			Text: BuildTplMessage(models.Lark, lk.tpl, ctx.Events),
		},
	}

	doSendAndRecord(ctx.Ctx, ctx.CallBackURL, ctx.CallBackURL, body, "callback", ctx.Stats, ctx.Events)
}

func (lk *LarkSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}
	urls, tokens := lk.extract(ctx.Users)
	message := BuildTplMessage(models.Lark, lk.tpl, ctx.Events)
	for i, url := range urls {
		body := feishu{
			Msgtype: "text",
			Content: feishuContent{
				Text: message,
			},
		}
		doSendAndRecord(ctx.Ctx, url, tokens[i], body, models.Lark, ctx.Stats, ctx.Events)
	}
}

func (lk *LarkSender) extract(users []*models.User) ([]string, []string) {
	urls := make([]string, 0, len(users))
	tokens := make([]string, 0, len(users))

	for _, user := range users {
		if token, has := user.ExtractToken(models.Lark); has {
			url := token
			if !strings.HasPrefix(token, "https://") && !strings.HasPrefix(token, "http://") {
				url = "https://open.larksuite.com/open-apis/bot/v2/hook/" + token
			}
			urls = append(urls, url)
			tokens = append(tokens, token)
		}
	}
	return urls, tokens
}
