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

func ConsumeVoice() {
	for {
		list := redisc.Pop(1, VOICE_QUEUE_NAME)
		if len(list) == 0 {
			time.Sleep(time.Millisecond * 200)
			continue
		}
		sendVoiceList(list)
	}
}

func sendVoiceList(list []*dataobj.Message) {
	for _, message := range list {
		VoiceWorkerChan <- 1
		go sendVoice(message)
	}
}

func sendVoice(message *dataobj.Message) {
	defer func() {
		<-VoiceWorkerChan
	}()

	switch Sender["voice"].Way {
	case "api":
		sendVoiceByAPI(message)
	case "shell":
		sendVoiceByShell(message)
	default:
		logger.Errorf("not support %s to send voice, voice: %+v", Sender["voice"].Way, message)
	}
}

func sendVoiceByAPI(message *dataobj.Message) {
	api := Sender["voice"].API
	res, code, err := httplib.PostJSON(api, time.Second, message, nil)
	logger.Infof("SendVoiceByAPI, api:%s, voice:%+v, error:%v, response:%s, statuscode:%d", api, message, err, string(res), code)
}

func sendVoiceByShell(message *dataobj.Message) {
	shell := path.Join(file.SelfDir(), "script", "send_voice")
	if !file.IsExist(shell) {
		logger.Errorf("%s not found", shell)
		return
	}

	output, err, isTimeout := sys.CmdRunT(time.Second*10, shell, strings.Join(message.Tos, ","), message.Content)
	logger.Infof("SendVoiceByShell, voice:%+v, output:%s, error: %v, isTimeout: %v", message, output, err, isTimeout)
}
