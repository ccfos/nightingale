package http

import (
	"strings"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/modules/server/cron"
	"github.com/didi/nightingale/v4/src/modules/server/redisc"

	"github.com/gin-gonic/gin"
)

func v1SendMail(c *gin.Context) {
	var message dataobj.Message
	bind(c, &message)

	if message.Tos == nil || len(message.Tos) == 0 {
		bomb("tos is empty")
	}

	message.Subject = strings.TrimSpace(message.Subject)
	message.Content = strings.TrimSpace(message.Content)

	if message.Subject == "" {
		bomb("subject is blank")
	}

	if message.Content == "" {
		bomb("content is blank")
	}

	renderMessage(c, redisc.Write(&message, cron.MAIL_QUEUE_NAME))
}

func v1SendSms(c *gin.Context) {
	var message dataobj.Message
	bind(c, &message)

	if message.Tos == nil || len(message.Tos) == 0 {
		bomb("tos is empty")
	}

	message.Content = strings.TrimSpace(message.Content)

	if message.Content == "" {
		bomb("content is blank")
	}

	renderMessage(c, redisc.Write(&message, cron.SMS_QUEUE_NAME))
}

func v1SendVoice(c *gin.Context) {
	var message dataobj.Message
	bind(c, &message)

	if message.Tos == nil || len(message.Tos) == 0 {
		bomb("tos is empty")
	}

	message.Content = strings.TrimSpace(message.Content)

	if message.Content == "" {
		bomb("content is blank")
	}

	renderMessage(c, redisc.Write(&message, cron.VOICE_QUEUE_NAME))
}

func v1SendIm(c *gin.Context) {
	var message dataobj.Message
	bind(c, &message)

	if message.Tos == nil || len(message.Tos) == 0 {
		bomb("tos is empty")
	}

	message.Content = strings.TrimSpace(message.Content)

	if message.Content == "" {
		bomb("content is blank")
	}

	renderMessage(c, redisc.Write(&message, cron.IM_QUEUE_NAME))
}
