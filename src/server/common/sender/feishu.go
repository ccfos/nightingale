package sender

import (
	"html/template"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/poster"
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
	if len(ctx.Users) == 0 || ctx.Rule == nil || ctx.Event == nil {
		return
	}
	urls, ats := fs.extract(ctx.Users)
	message := BuildTplMessage(fs.tpl, ctx.Event)
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
		fs.doSend(url, body)
	}
}

func (fs *FeishuSender) SendRaw(users []*models.User, title, message string) {
	if len(users) == 0 {
		return
	}
	urls, _ := fs.extract(users)
	body := feishu{
		Msgtype: "text",
		Content: feishuContent{
			Text: message,
		},
	}
	for _, url := range urls {
		fs.doSend(url, body)
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
			if !strings.HasPrefix(token, "https://") {
				url = "https://open.feishu.cn/open-apis/bot/v2/hook/" + token
			}
			urls = append(urls, url)
		}
	}
	return urls, ats
}

func (fs *FeishuSender) doSend(url string, body feishu) {
	res, code, err := poster.PostJSON(url, time.Second*5, body, 3)
	if err != nil {
		logger.Errorf("feishu_sender: result=fail url=%s code=%d error=%v response=%s", url, code, err, string(res))
	} else {
		logger.Infof("feishu_sender: result=succ url=%s code=%d response=%s", url, code, string(res))
	}
}
