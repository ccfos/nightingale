package sender

import (
	"html/template"
	"net/url"
	"strings"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
)

type MatterMostMessage struct {
	Text   string
	Tokens []string
	Stats  *astats.Stats
}

type mm struct {
	Channel  string `json:"channel"`
	Username string `json:"username"`
	Text     string `json:"text"`
}

type MmSender struct {
	tpl *template.Template
}

func (ms *MmSender) Send(ctx MessageContext) {
	if len(ctx.Users) == 0 || len(ctx.Events) == 0 {
		return
	}

	urls := ms.extract(ctx.Users)
	if len(urls) == 0 {
		return
	}
	message := BuildTplMessage(models.Mm, ms.tpl, ctx.Events)

	SendMM(ctx.Ctx, MatterMostMessage{
		Text:   message,
		Tokens: urls,
		Stats:  ctx.Stats,
	}, ctx.Events[0])
}

func (ms *MmSender) CallBack(ctx CallBackContext) {
	if len(ctx.Events) == 0 || len(ctx.CallBackURL) == 0 {
		return
	}
	message := BuildTplMessage(models.Mm, ms.tpl, ctx.Events)

	SendMM(ctx.Ctx, MatterMostMessage{
		Text:   message,
		Tokens: []string{ctx.CallBackURL},
		Stats:  ctx.Stats,
	}, ctx.Events[0])

	ctx.Stats.AlertNotifyTotal.WithLabelValues("rule_callback").Inc()
}

func (ms *MmSender) extract(users []*models.User) []string {
	tokens := make([]string, 0, len(users))
	for _, user := range users {
		if token, has := user.ExtractToken(models.Mm); has {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func SendMM(ctx *ctx.Context, message MatterMostMessage, event *models.AlertCurEvent) {
	for i := 0; i < len(message.Tokens); i++ {
		u, err := url.Parse(message.Tokens[i])
		if err != nil {
			logger.Errorf("mm_sender: failed to parse error=%v", err)
			continue
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
			doSendAndRecord(ctx, ur, message.Tokens[i], body, models.Mm, message.Stats, event)
		}
	}
}

func MapStrToStr(arr []string, fn func(s string) string) []string {
	var newArray = []string{}
	for _, it := range arr {
		newArray = append(newArray, fn(it))
	}
	return newArray
}
