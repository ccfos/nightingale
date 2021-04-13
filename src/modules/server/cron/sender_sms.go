package cron

import (
	"path"
	"strings"
	"time"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/modules/server/redisc"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
	"github.com/toolkits/pkg/sys"
)

func ConsumeSms() {
	for {
		list := redisc.Pop(1, SMS_QUEUE_NAME)
		if len(list) == 0 {
			time.Sleep(time.Millisecond * 200)
			continue
		}
		sendSmsList(list)
	}
}

func sendSmsList(list []*dataobj.Message) {
	for _, message := range list {
		SmsWorkerChan <- 1
		go sendSms(message)
	}
}

func sendSms(message *dataobj.Message) {
	defer func() {
		<-SmsWorkerChan
	}()

	switch Sender["sms"].Way {
	case "api":
		sendSmsByAPI(message)
	case "shell":
		sendSmsByShell(message)
	default:
		logger.Errorf("not support %s to send sms, sms: %+v", Sender["sms"].Way, message)
	}
}

func sendSmsByAPI(message *dataobj.Message) {
	api := Sender["sms"].API
	res, err := httplib.Post(api).JSONBodyQuiet(message).SetTimeout(time.Second * 3).String()
	logger.Infof("SendSmsByAPI, api:%s, sms:%+v, error:%v, response:%s", api, message, err, res)
}

func sendSmsByShell(message *dataobj.Message) {
	shell := path.Join(file.SelfDir(), "script", "send_sms")
	if !file.IsExist(shell) {
		logger.Errorf("%s not found", shell)
		return
	}

	output, err, isTimeout := sys.CmdRunT(time.Second*10, shell, strings.Join(message.Tos, ","), message.Content)
	logger.Infof("SendSmsByShell, sms:%+v, output:%s, error: %v, isTimeout: %v", message, output, err, isTimeout)
}
