package sender

import (
	"fmt"
	"html/template"
	"net/url"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
)

type LarkCardSender struct {
	tpl *template.Template
}

func (fs *LarkCardSender) CallBack(ctx CallBackContext) {
	if len(ctx.Events) == 0 || len(ctx.CallBackURL) == 0 {
		return
	}

	ats := ExtractAtsParams(ctx.CallBackURL)
	message := BuildTplMessage(models.LarkCard, fs.tpl, ctx.Events)

	if len(ats) > 0 {
		atTags := ""
		for _, at := range ats {
			if strings.Contains(at, "@") {
				atTags += fmt.Sprintf("<at email=\"%s\" ></at>", at)
			} else {
				atTags += fmt.Sprintf("<at id=\"%s\" ></at>", at)
			}
		}
		message = atTags + message
	}

	color := "red"
	lowerUnicode := strings.ToLower(message)
	if strings.Count(lowerUnicode, Recovered) > 0 && strings.Count(lowerUnicode, Triggered) > 0 {
		color = "orange"
	} else if strings.Count(lowerUnicode, Recovered) > 0 {
		color = "green"
	}

	SendTitle := fmt.Sprintf("🔔 %s", ctx.Events[0].RuleName)
	body := createFeishuCardBody()
	body.Card.Header.Title.Content = SendTitle
	body.Card.Header.Template = color
	body.Card.Elements[0].Text.Content = message
	body.Card.Elements[2].Elements[0].Content = SendTitle

	// This is to be compatible with the Larkcard interface, if with query string parameters, the request will fail
	// Remove query parameters from the URL,
	parsedURL, err := url.Parse(ctx.CallBackURL)
	if err != nil {
		return
	}
	parsedURL.RawQuery = ""

	doSendAndRecord(ctx.Ctx, ctx.CallBackURL, ctx.CallBackURL, body, "callback",
		ctx.Stats, ctx.Events[0])
}

func (fs *LarkCardSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}
	urls, _ := fs.extract(ctx.Users)
	message := BuildTplMessage(models.LarkCard, fs.tpl, ctx.Events)
	color := "red"
	lowerUnicode := strings.ToLower(message)
	if strings.Count(lowerUnicode, Recovered) > 0 && strings.Count(lowerUnicode, Triggered) > 0 {
		color = "orange"
	} else if strings.Count(lowerUnicode, Recovered) > 0 {
		color = "green"
	}

	SendTitle := fmt.Sprintf("🔔 %s", ctx.Events[0].RuleName)
	body := createFeishuCardBody()
	body.Card.Header.Title.Content = SendTitle
	body.Card.Header.Template = color
	body.Card.Elements[0].Text.Content = message
	body.Card.Elements[2].Elements[0].Content = SendTitle
	for _, url := range urls {
		doSend(url, body, models.LarkCard, ctx.Stats)
	}
}

func (fs *LarkCardSender) extract(users []*models.User) ([]string, []string) {
	urls := make([]string, 0, len(users))
	ats := make([]string, 0)
	for i := range users {
		if token, has := users[i].ExtractToken(models.Lark); has {
			url := token
			if !strings.HasPrefix(token, "https://") && !strings.HasPrefix(token, "http://") {
				url = "https://open.larksuite.com/open-apis/bot/v2/hook/" + strings.TrimSpace(token)
			}
			urls = append(urls, url)
		}
	}
	return urls, ats
}
