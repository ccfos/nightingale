package sender

import (
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

type FeishuSender struct {
	tpl *template.Template
}

func (fs *FeishuSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}
	urls, ats := fs.extract(ctx.Users)
	message := BuildTplMessage(models.Feishu, fs.tpl, ctx.Events)
	for _, url := range urls {
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
		doSend(url, body, models.Feishu, ctx.Stats)
	}
}

func (fs *FeishuSender) extract(users []*models.User) ([]string, []string) {
	urls := make([]string, 0, len(users))
	ats := make([]string, 0, len(users))

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
		}
	}
	return urls, ats
}
