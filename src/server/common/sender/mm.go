package sender

import (
	"net/url"
	"strings"
	"time"

	"github.com/didi/nightingale/v5/src/pkg/poster"
	"github.com/toolkits/pkg/logger"
)

type MatterMostMessage struct {
	Text   string
	Tokens []string
}

type mm struct {
	Channel  string `json:"channel"`
	Username string `json:"username"`
	Text     string `json:"text"`
}

func MapStrToStr(arr []string, fn func(s string) string) []string {
	var newArray = []string{}
	for _, it := range arr {
		newArray = append(newArray, fn(it))
	}
	return newArray
}

func SendMM(message MatterMostMessage) {

	for i := 0; i < len(message.Tokens); i++ {
		u, err := url.Parse(message.Tokens[i])
		if err != nil {
			logger.Errorf("mm_sender: failed to parse error=%v", err)
		}

		v, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			logger.Errorf("mm_sender: failed to parse query error=%v", err)
		}

		channels := v["channel"] // do not get
		txt := ""
		atuser := v["atuser"]
		if len(atuser) != 0 {
			txt = strings.Join(MapStrToStr(atuser, func(u string) string {
				return "@" + u
			}), ",") + "\n"
		}
		username := v.Get("username")
		if err != nil {
			logger.Errorf("mm_sender: failed to parse error=%v", err)
		}
		// simple concatenating
		ur := u.Scheme + "://" + u.Host + u.Path
		for _, channel := range channels {
			body := mm{
				Channel:  channel,
				Username: username,
				Text:     txt + message.Text,
			}

			res, code, err := poster.PostJSON(ur, time.Second*5, body, 3)
			if err != nil {
				logger.Errorf("mm_sender: result=fail url=%s code=%d error=%v response=%s", ur, code, err, string(res))
			} else {
				logger.Infof("mm_sender: result=succ url=%s code=%d response=%s", ur, code, string(res))
			}
		}
	}
}
