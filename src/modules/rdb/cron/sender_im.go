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

func ConsumeIm() {
	if !config.Config.Redis.Enable {
		return
	}

	for {
		list := redisc.Pop(1, config.IM_QUEUE_NAME)
		if len(list) == 0 {
			time.Sleep(time.Millisecond * 200)
			continue
		}
		sendImList(list)
	}
}

func sendImList(list []*dataobj.Message) {
	for _, message := range list {
		ImWorkerChan <- 1
		go sendIm(message)
	}
}

func sendIm(message *dataobj.Message) {
	defer func() {
		<-ImWorkerChan
	}()

	switch config.Config.Sender["im"].Way {
	case "api":
		sendImByAPI(message)
	case "shell":
		sendImByShell(message)
	default:
		logger.Errorf("not support %s to send im, im: %+v", config.Config.Sender["im"].Way, message)
	}
}

func sendImByAPI(message *dataobj.Message) {
	api := config.Config.Sender["im"].API
	res, code, err := httplib.PostJSON(api, time.Second, message, nil)
	logger.Infof("SendImByAPI, api:%s, im:%+v, error:%v, response:%s, statuscode:%d", api, message, err, string(res), code)
}

func sendImByShell(message *dataobj.Message) {
	shell := path.Join(file.SelfDir(), "script", "send_im")
	if !file.IsExist(shell) {
		logger.Errorf("%s not found", shell)
		return
	}

	output, err, isTimeout := sys.CmdRunT(time.Second*10, shell, strings.Join(message.Tos, ","), message.Content)
	logger.Infof("SendImByShell, im:%+v, output:%s, error: %v, isTimeout: %v", message, output, err, isTimeout)
}
