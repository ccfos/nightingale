package sender

import (
	"fmt"
	"html/template"
	"net/url"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
)

type Conf struct {
	WideScreenMode bool `json:"wide_screen_mode"`
	EnableForward  bool `json:"enable_forward"`
}

type Te struct {
	Content string `json:"content"`
	Tag     string `json:"tag"`
}

type Element struct {
	Tag      string    `json:"tag"`
	Text     Te        `json:"text"`
	Content  string    `json:"content"`
	Elements []Element `json:"elements"`
}

type Titles struct {
	Content string `json:"content"`
	Tag     string `json:"tag"`
}

type Headers struct {
	Title    Titles `json:"title"`
	Template string `json:"template"`
}

type Cards struct {
	Config   Conf      `json:"config"`
	Elements []Element `json:"elements"`
	Header   Headers   `json:"header"`
}

type feishuCard struct {
	feishu
	Card Cards `json:"card"`
}

type FeishuCardSender struct {
	tpl *template.Template
}

const (
	Recovered = "recovered"
	Triggered = "triggered"
)

func createFeishuCardBody() feishuCard {
	return feishuCard{
		feishu: feishu{Msgtype: "interactive"},
		Card: Cards{
			Config: Conf{
				WideScreenMode: true,
				EnableForward:  true,
			},
			Header: Headers{
				Title: Titles{
					Tag: "plain_text",
				},
			},
			Elements: []Element{
				{
					Tag: "div",
					Text: Te{
						Tag: "lark_md",
					},
				},
				{
					Tag: "hr",
				},
				{
					Tag: "note",
					Elements: []Element{
						{
							Tag: "lark_md",
						},
					},
				},
			},
		},
	}
}

func (fs *FeishuCardSender) CallBack(ctx CallBackContext) {
	if len(ctx.Events) == 0 || len(ctx.CallBackURL) == 0 {
		return
	}

	ats := ExtractAtsParams(ctx.CallBackURL)
	message := BuildTplMessage(models.FeishuCard, fs.tpl, ctx.Events)

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

	// This is to be compatible with the feishucard interface, if with query string parameters, the request will fail
	// Remove query parameters from the URL,
	parsedURL, err := url.Parse(ctx.CallBackURL)
	if err != nil {
		return
	}
	parsedURL.RawQuery = ""

	doSendAndRecord(ctx.Ctx, parsedURL.String(), parsedURL.String(), body, "callback",
		ctx.Stats, ctx.Events[0])
}

func (fs *FeishuCardSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}
	urls, tokens := fs.extract(ctx.Users)
	message := BuildTplMessage(models.FeishuCard, fs.tpl, ctx.Events)
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
	for i, url := range urls {
		doSendAndRecord(ctx.Ctx, url, tokens[i], body, models.FeishuCard,
			ctx.Stats, ctx.Events[0])
	}
}

func (fs *FeishuCardSender) extract(users []*models.User) ([]string, []string) {
	urls := make([]string, 0, len(users))
	tokens := make([]string, 0, len(users))
	for i := range users {
		if token, has := users[i].ExtractToken(models.FeishuCard); has {
			url := token
			if !strings.HasPrefix(token, "https://") && !strings.HasPrefix(token, "http://") {
				url = "https://open.feishu.cn/open-apis/bot/v2/hook/" + strings.TrimSpace(token)
			}
			urls = append(urls, url)
			tokens = append(tokens, token)
		}
	}
	return urls, tokens
}
