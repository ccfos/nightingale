package cron

import (
	"path"
	"strings"
	"time"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
	"github.com/toolkits/pkg/sys"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/didi/nightingale/src/modules/rdb/redisc"
)

func ConsumeVoice() {
	if !config.Config.Redis.Enable {
		return
	}

	for {
		list := redisc.Pop(1, config.VOICE_QUEUE_NAME)
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

	switch config.Config.Sender["voice"].Way {
	case "api":
		sendVoiceByAPI(message)
	case "shell":
		sendVoiceByShell(message)
	default:
		logger.Errorf("not support %s to send voice, voice: %+v", config.Config.Sender["voice"].Way, message)
	}
}

func sendVoiceByAPI(message *dataobj.Message) {
	api := config.Config.Sender["voice"].API
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
