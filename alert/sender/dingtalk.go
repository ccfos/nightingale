package sender

import (
	"html/template"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/toolkits/pkg/logger"
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
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}

	urls, ats := ds.extract(ctx.Users)
	if len(urls) == 0 {
		return
	}
	message := BuildTplMessage(models.Dingtalk, ds.tpl, ctx.Events)

	for _, url := range urls {
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

		doSend(url, body, models.Dingtalk, ctx.Stats)
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
			if !strings.HasPrefix(token, "https://") && !strings.HasPrefix(token, "http://") {
				url = "https://oapi.dingtalk.com/robot/send?access_token=" + token
			}
			urls = append(urls, url)
		}
	}
	return urls, ats
}

func doSend(url string, body interface{}, channel string, stats *astats.Stats) {
	stats.AlertNotifyTotal.WithLabelValues(channel).Inc()

	res, code, err := poster.PostJSON(url, time.Second*5, body, 3)
	if err != nil {
		logger.Errorf("%s_sender: result=fail url=%s code=%d error=%v response=%s", channel, url, code, err, string(res))
		stats.AlertNotifyErrorTotal.WithLabelValues(channel).Inc()
	} else {
		logger.Infof("%s_sender: result=succ url=%s code=%d response=%s", channel, url, code, string(res))
	}
}
