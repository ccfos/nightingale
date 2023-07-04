package sender

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/toolkits/pkg/logger"
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

var (
	body = feishuCard{
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
)

func (fs *FeishuCardSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}
	urls, _ := fs.extract(ctx.Users)
	message := BuildTplMessage(fs.tpl, ctx.Events)
	color := "red"
	lowerUnicode := strings.ToLower(message)
	if strings.Count(lowerUnicode, Recovered) > 0 && strings.Count(lowerUnicode, Triggered) > 0 {
		color = "orange"
	} else if strings.Count(lowerUnicode, Recovered) > 0 {
		color = "green"
	}

	SendTitle := fmt.Sprintf("🔔 %s", ctx.Events[0].RuleName)
	body.Card.Header.Title.Content = SendTitle
	body.Card.Header.Template = color
	body.Card.Elements[0].Text.Content = message
	body.Card.Elements[2].Elements[0].Content = SendTitle
	for _, url := range urls {
		fs.doSend(url, body)
	}
}

func (fs *FeishuCardSender) extract(users []*models.User) ([]string, []string) {
	urls := make([]string, 0, len(users))
	ats := make([]string, 0)
	for i := range users {
		if token, has := users[i].ExtractToken(models.FeishuCard); has {
			url := token
			if !strings.HasPrefix(token, "https://") {
				url = "https://open.feishu.cn/open-apis/bot/v2/hook/" + strings.TrimSpace(token)
			}
			urls = append(urls, url)
		}
	}
	return urls, ats
}

func (fs *FeishuCardSender) doSend(url string, body feishuCard) {
	res, code, err := poster.PostJSON(url, time.Second*5, body, 3)
	if err != nil {
		logger.Errorf("feishucard_sender: result=fail url=%s code=%d error=%v response=%s", url, code, err, string(res))
	} else {
		logger.Debugf("feishucard_sender: result=succ url=%s code=%d response=%s", url, code, string(res))
	}
}
