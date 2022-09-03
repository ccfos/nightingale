package sender

import (
	"strings"
	"time"

	"github.com/didi/nightingale/v5/src/pkg/poster"
	"github.com/toolkits/pkg/logger"
)

type WecomMessage struct {
	Text   string
	Tokens []string
}

type wecomMarkdown struct {
	Content string `json:"content"`
}

type wecom struct {
	Msgtype  string        `json:"msgtype"`
	Markdown wecomMarkdown `json:"markdown"`
}

func SendWecom(message WecomMessage) {
	for i := 0; i < len(message.Tokens); i++ {
		url := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=" + message.Tokens[i]
		if strings.HasPrefix(message.Tokens[i], "https://") {
			url = message.Tokens[i]
		}
		body := wecom{
			Msgtype: "markdown",
			Markdown: wecomMarkdown{
				Content: message.Text,
			},
		}

		res, code, err := poster.PostJSON(url, time.Second*5, body, 3)
		if err != nil {
			logger.Errorf("wecom_sender: result=fail url=%s code=%d error=%v response=%s", url, code, err, string(res))
		} else {
			logger.Infof("wecom_sender: result=succ url=%s code=%d response=%s", url, code, string(res))
		}
	}
}
