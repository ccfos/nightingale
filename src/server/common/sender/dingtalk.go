package sender

import (
	"net/url"
	"strings"
	"time"

	"github.com/didi/nightingale/v5/src/pkg/poster"
	"github.com/toolkits/pkg/logger"
)

type DingtalkMessage struct {
	Title     string
	Text      string
	AtMobiles []string
	Tokens    []string
}

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

func SendDingtalk(message DingtalkMessage) {
	ats := make([]string, len(message.AtMobiles))
	for i := 0; i < len(message.AtMobiles); i++ {
		ats[i] = "@" + message.AtMobiles[i]
	}

	for i := 0; i < len(message.Tokens); i++ {
		u, err := url.Parse(message.Tokens[i])
		if err != nil {
			logger.Errorf("dingtalk_sender: failed to parse error=%v", err)
		}

		v, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			logger.Errorf("dingtalk_sender: failed to parse query error=%v", err)
		}

		ur := "https://oapi.dingtalk.com/robot/send?access_token=" + u.Path
		if strings.HasPrefix(message.Tokens[i], "https://") {
			ur = message.Tokens[i]
		}
		body := dingtalk{
			Msgtype: "markdown",
			Markdown: dingtalkMarkdown{
				Title: message.Title,
				Text:  message.Text,
			},
		}

		if v.Get("noat") != "1" {
			body.Markdown.Text = message.Text + " " + strings.Join(ats, " ")
			body.At = dingtalkAt{
				AtMobiles: message.AtMobiles,
				IsAtAll:   false,
			}
		}

		res, code, err := poster.PostJSON(ur, time.Second*5, body, 3)
		if err != nil {
			logger.Errorf("dingtalk_sender: result=fail url=%s code=%d error=%v response=%s", ur, code, err, string(res))
		} else {
			logger.Infof("dingtalk_sender: result=succ url=%s code=%d response=%s", ur, code, string(res))
		}
	}
}
