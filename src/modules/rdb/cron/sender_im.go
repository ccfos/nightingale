package cron

import (
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
	"github.com/toolkits/pkg/sys"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/didi/nightingale/src/modules/rdb/dingtalk"
	"github.com/didi/nightingale/src/modules/rdb/redisc"
	"github.com/didi/nightingale/src/modules/rdb/wechat"
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
	case "wechat":
		sendImByWeChat(message)
	case "wechat_robot":
		sendImByWeChatRobot(message)
	case "dingtalk_robot":
		sendImByDingTalkRobot(message)
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

func sendImByWeChat(message *dataobj.Message) {
	corpID := config.Config.WeChat.CorpID
	agentID := config.Config.WeChat.AgentID
	secret := config.Config.WeChat.Secret

	cnt := len(message.Tos)
	if cnt == 0 {
		logger.Warningf("im send wechat fail, empty tos, message: %+v", message)
		return
	}

	client := wechat.New(corpID, agentID, secret)
	var err error
	for i := 0; i < cnt; i++ {
		toUser := strings.TrimSpace(message.Tos[i])
		if toUser == "" {
			continue
		}
		err = client.Send(wechat.Message{
			ToUser:  toUser,
			MsgType: "text",
			Text:    wechat.Content{Content: message.Content},
		})

		if err != nil {
			logger.Warningf("im wechat send to %s fail: %v", message.Tos[i], err)
		} else {
			logger.Infof("im wechat send to %s succ", message.Tos[i])
		}
	}
}

func sendImByWeChatRobot(message *dataobj.Message) {
	cnt := len(message.Tos)
	if cnt == 0 {
		logger.Warningf("im send wechat_robot fail, empty tos, message: %+v", message)
		return
	}

	set := make(map[string]struct{}, cnt)
	for i := 0; i < cnt; i++ {
		toUser := strings.TrimSpace(message.Tos[i])
		if toUser == "" {
			continue
		}

		if _, ok := set[toUser]; !ok {
			set[toUser] = struct{}{}
		}
	}

	for tokenUser := range set {
		mess := wechat.Message{
			ToUser:  tokenUser,
			MsgType: "text",
			Text:    wechat.Content{Content: message.Content},
		}

		err := wechat.RobotSend(mess)
		if err != nil {
			logger.Warningf("im wechat_robot send to %s fail: %v", tokenUser, err)
		} else {
			logger.Infof("im wechat_robot send to %s succ", tokenUser)
		}
	}
}

func sendImByDingTalkRobot(message *dataobj.Message) {
	cnt := len(message.Tos)
	if cnt == 0 {
		logger.Warningf("im send dingtalk_robot fail, empty tos, message: %+v", message)
		return
	}

	set := make(map[string]struct{}, cnt)
	for i := 0; i < cnt; i++ {
		toUser := strings.TrimSpace(message.Tos[i])
		if toUser == "" {
			continue
		}

		if _, ok := set[toUser]; !ok {
			set[toUser] = struct{}{}
		}
	}

	req := regexp.MustCompile("^1[0-9]{10}$")
	var atUser []string
	var tokenUser string
	for user := range set {
		if req.MatchString(user){
			atUser = append(atUser, user)
		} else {
			tokenUser = user
		}
	}
	err := dingtalk.RobotSend(tokenUser, message.Content, atUser)
	if err != nil {
		logger.Warningf("im dingtalk_robot send to %s fail: %v", tokenUser, err)
	} else {
		logger.Infof("im dingtalk_robot send to %s succ", tokenUser)
	}
}
