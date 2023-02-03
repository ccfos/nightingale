package sender

import (
	"html/template"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/poster"
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

type DingtalkSender struct {
	tpl *template.Template
}

func (ds *DingtalkSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || ctx.Rule == nil || ctx.Event == nil {
		return
	}

	urls, ats := ds.extract(ctx.Users)
	if len(urls) == 0 {
		return
	}
	message := BuildTplMessage(ds.tpl, ctx.Event)

	for _, url := range urls {
		var body dingtalk
		// NoAt in url
		if strings.Contains(url, "noat=1") {
			body = dingtalk{
				Msgtype: "markdown",
				Markdown: dingtalkMarkdown{
					Title: ctx.Rule.Name,
					Text:  message,
				},
			}
		} else {
			body = dingtalk{
				Msgtype: "markdown",
				Markdown: dingtalkMarkdown{
					Title: ctx.Rule.Name,
					Text:  message + " " + strings.Join(ats, " "),
				},
				At: dingtalkAt{
					AtMobiles: ats,
					IsAtAll:   false,
				},
			}
		}
		ds.doSend(url, body)
	}
}

func (ds *DingtalkSender) SendRaw(users []*models.User, title, message string) {
	if len(users) == 0 {
		return
	}
	urls, _ := ds.extract(users)
	body := dingtalk{
		Msgtype: "markdown",
		Markdown: dingtalkMarkdown{
			Title: title,
			Text:  message,
		},
	}
	for _, url := range urls {
		ds.doSend(url, body)
	}
}

// extract urls and ats from Users
func (ds *DingtalkSender) extract(users []*models.User) ([]string, []string) {
	urls := make([]string, 0, len(users))
	ats := make([]string, 0, len(users))

	for _, user := range users {
		if user.Phone != "" {
			ats = append(ats, "@"+user.Phone)
		}
		if token, has := user.ExtractToken(models.Dingtalk); has {
			url := token
			if !strings.HasPrefix(token, "https://") {
				url = "https://oapi.dingtalk.com/robot/send?access_token=" + token
			}
			urls = append(urls, url)
		}
	}
	return urls, ats
}

func (ds *DingtalkSender) doSend(url string, body dingtalk) {
	res, code, err := poster.PostJSON(url, time.Second*5, body, 3)
	if err != nil {
		logger.Errorf("dingtalk_sender: result=fail url=%s code=%d error=%v response=%s", url, code, err, string(res))
	} else {
		logger.Infof("dingtalk_sender: result=succ url=%s code=%d response=%s", url, code, string(res))
	}
}
