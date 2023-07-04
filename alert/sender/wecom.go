package sender

import (
	"html/template"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/toolkits/pkg/logger"
)

type wecomMarkdown struct {
	Content string `json:"content"`
}

type wecom struct {
	Msgtype  string        `json:"msgtype"`
	Markdown wecomMarkdown `json:"markdown"`
}

type WecomSender struct {
	tpl *template.Template
}

func (ws *WecomSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}
	urls := ws.extract(ctx.Users)
	message := BuildTplMessage(ws.tpl, ctx.Events)
	for _, url := range urls {
		body := wecom{
			Msgtype: "markdown",
			Markdown: wecomMarkdown{
				Content: message,
			},
		}
		ws.doSend(url, body)
	}
}

func (ws *WecomSender) extract(users []*models.User) []string {
	urls := make([]string, 0, len(users))
	for _, user := range users {
		if token, has := user.ExtractToken(models.Wecom); has {
			url := token
			if !strings.HasPrefix(token, "https://") {
				url = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=" + token
			}
			urls = append(urls, url)
		}
	}
	return urls
}

func (ws *WecomSender) doSend(url string, body wecom) {
	res, code, err := poster.PostJSON(url, time.Second*5, body, 3)
	if err != nil {
		logger.Errorf("wecom_sender: result=fail url=%s code=%d error=%v response=%s", url, code, err, string(res))
	} else {
		logger.Infof("wecom_sender: result=succ url=%s code=%d response=%s", url, code, string(res))
	}
}
