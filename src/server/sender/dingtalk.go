package sender

import (
	"time"

	"github.com/didi/nightingale/v5/src/server/poster"
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
	for i := 0; i < len(message.Tokens); i++ {
		url := "https://oapi.dingtalk.com/robot/send?access_token=" + message.Tokens[i]
		body := dingtalk{
			Msgtype: "markdown",
			Markdown: dingtalkMarkdown{
				Title: message.Title,
				Text:  message.Text,
			},
			At: dingtalkAt{
				AtMobiles: message.AtMobiles,
				IsAtAll:   false,
			},
		}

		res, code, err := poster.PostJSON(url, time.Second*5, body)
		if err != nil {
			logger.Errorf("dingtalk_sender: result=fail url=%s code=%d error=%v response=%s", url, code, err, string(res))
		} else {
			logger.Infof("dingtalk_sender: result=succ url=%s code=%d response=%s", url, code, string(res))
		}
	}
}
