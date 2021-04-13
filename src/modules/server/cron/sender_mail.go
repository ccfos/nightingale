package cron

import (
	"crypto/tls"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/redisc"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
	"github.com/toolkits/pkg/sys"
	gomail "gopkg.in/gomail.v2"
)

func ConsumeMail() {
	for {
		list := redisc.Pop(1, MAIL_QUEUE_NAME)
		if len(list) == 0 {
			time.Sleep(time.Millisecond * 200)
			continue
		}
		sendMailList(list)
	}
}

func sendMailList(list []*dataobj.Message) {
	for _, message := range list {
		MailWorkerChan <- 1
		go sendMail(message)
	}
}

func sendMail(message *dataobj.Message) {
	defer func() {
		<-MailWorkerChan
	}()

	cnt := len(message.Tos)
	tos := make([]string, 0, cnt)
	for i := 0; i < cnt; i++ {
		item := strings.TrimSpace(message.Tos[i])
		if item == "" {
			continue
		}
		tos = append(tos, item)
	}

	if len(tos) == 0 {
		logger.Debug("email receivers empty")
		return
	}

	message.Tos = tos

	switch Sender["mail"].Way {
	case "api":
		sendMailByAPI(message)
	case "smtp":
		SendMailBySMTP(message)
	case "shell":
		sendMailByShell(message)
	default:
		logger.Errorf("not support %s to send mail, mail: %+v", Sender["mail"].Way, message)
	}
}

func sendMailByAPI(message *dataobj.Message) {
	api := Sender["mail"].API
	res, code, err := httplib.PostJSON(api, time.Second, message, nil)
	logger.Infof("SendMailByAPI, api:%s, mail:%+v, error:%v, response:%s, statuscode:%d", api, message, err, string(res), code)
}

func sendMailByShell(message *dataobj.Message) {
	shell := path.Join(file.SelfDir(), "script", "send_mail")
	if !file.IsExist(shell) {
		logger.Errorf("%s not found", shell)
		return
	}

	fp := fmt.Sprintf("/tmp/n9e.mail.content.%d", time.Now().UnixNano())
	_, err := file.WriteString(fp, message.Content)
	if err != nil {
		logger.Errorf("cannot write string to %s", fp)
		return
	}

	output, err, isTimeout := sys.CmdRunT(time.Second*10, shell, strings.Join(message.Tos, ","), message.Subject, fp)
	logger.Infof("SendMailByShell, mail:%+v, output:%s, error: %v, isTimeout: %v", message, output, err, isTimeout)

	file.Unlink(fp)
}

func SendMailBySMTP(message *dataobj.Message) error {
	ckeys := []string{
		"smtpHost",
		"smtpPort",
		"smtpUser",
		"smtpPass",
		"smtpInsecureSkipVerify",
	}

	smtpConf, err := models.ConfigsGets(ckeys)
	if err != nil {
		wraperr := fmt.Errorf("SendMailBySMTP, message:%+v, error: cannot retrieve smtp config:%v", message, err)
		logger.Info(wraperr.Error())
		return wraperr
	}

	port, err := strconv.Atoi(smtpConf["smtpPort"])
	if err != nil {
		wraperr := fmt.Errorf("SendMailBySMTP, message:%+v, error: cannot convert smtp port: %s", message, smtpConf["smtpPort"])
		logger.Info(wraperr.Error())
		return wraperr
	}

	mailer := gomail.NewDialer(smtpConf["smtpHost"], port, smtpConf["smtpUser"], smtpConf["smtpPass"])
	if smtpConf["smtpInsecureSkipVerify"] == "1" {
		mailer.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}

	m := gomail.NewMessage()
	m.SetHeader("From", smtpConf["smtpUser"])
	m.SetHeader("To", message.Tos...)
	m.SetHeader("Subject", message.Subject)
	m.SetBody("text/html", message.Content)

	err = mailer.DialAndSend(m)
	logger.Infof("SendMailBySMTP, error: %v, message.tos: %v, message.subject: %s", err, message.Tos, message.Subject)
	return err
}
