package sender

import (
	"time"

	"github.com/didi/nightingale/v5/src/pkg/poster"
	"github.com/toolkits/pkg/logger"
)

type FeishuMessage struct {
	Text      string
	AtMobiles []string
	Tokens    []string
}

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

func SendFeishu(message FeishuMessage) {
	for i := 0; i < len(message.Tokens); i++ {
		url := "https://open.feishu.cn/open-apis/bot/v2/hook/" + message.Tokens[i]
		body := feishu{
			Msgtype: "text",
			Content: feishuContent{
				Text: message.Text,
			},
			At: feishuAt{
				AtMobiles: message.AtMobiles,
				IsAtAll:   false,
			},
		}

		res, code, err := poster.PostJSON(url, time.Second*5, body, 3)
		if err != nil {
			logger.Errorf("feishu_sender: result=fail url=%s code=%d error=%v response=%s", url, code, err, string(res))
		} else {
			logger.Infof("feishu_sender: result=succ url=%s code=%d response=%s", url, code, string(res))
		}
	}
}
