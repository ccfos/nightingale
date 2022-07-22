package engine

import (
	"encoding/json"
	"plugin"
	"runtime"
	"time"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/common/sender"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
	"github.com/tidwall/gjson"
	"github.com/toolkits/pkg/logger"
)

type NoticeMaintainer struct {
	NotifyUsersObj []*models.User `json:"notify_user_obj" gorm:"-"`
	Title          string         `json:"title"`
	Content        string         `json:"content"`
}

func noticeCallPlugin(stdinBytes []byte) {
	if !config.C.Alerting.CallPlugin.Enable {
		return
	}

	if runtime.GOOS == "windows" {
		logger.Errorf("call notify plugin on unsupported os: %s", runtime.GOOS)
		return
	}

	p, err := plugin.Open(config.C.Alerting.CallPlugin.PluginPath)
	if err != nil {
		logger.Errorf("failed to open notify plugin: %v", err)
		return
	}
	caller, err := p.Lookup(config.C.Alerting.CallPlugin.Caller)
	if err != nil {
		logger.Errorf("failed to load caller: %v", err)
		return
	}
	notifier, ok := caller.(Notifier)
	if !ok {
		logger.Errorf("notifier interface not implemented): %v", err)
		return
	}
	notifier.NotifyMaintainer(stdinBytes)
	logger.Debugf("noticeCallPlugin done. %s", notifier.Descript())
}

// notify to maintainer to handle the error
func notifyToMaintainer(e error, title string) {

	logger.Errorf("notifyToMaintainer，title:%s, error:%v", title, e)

	var noticeMaintainer NoticeMaintainer
	maintainerUsers := memsto.UserCache.GetMaintainerUsers()
	if len(maintainerUsers) == 0 {
		return
	}
	triggerTime := time.Now().Format("2006/01/02 - 15:04:05")
	noticeMaintainer.NotifyUsersObj = maintainerUsers
	noticeMaintainer.Content = "【内部处理错误】当前标题: " + title + "\n【内部处理错误】当前异常: " + e.Error() + "\n【内部处理错误】发送时间: " + triggerTime
	noticeMaintainer.Title = title
	stdinBytes, err := json.Marshal(noticeMaintainer)
	if err != nil {
		logger.Errorf("notifyToMaintainer: failed to marshal noticeMaintainer: %v", err)
	} else {
		noticeCallPlugin(stdinBytes)
	}

	if len(config.C.Alerting.NotifyBuiltinChannels) == 0 {
		return
	}
	emailset := make(map[string]struct{})
	phoneset := make(map[string]struct{})
	wecomset := make(map[string]struct{})
	dingtalkset := make(map[string]struct{})
	feishuset := make(map[string]struct{})

	for _, user := range maintainerUsers {
		if user.Email != "" {
			emailset[user.Email] = struct{}{}
		}

		if user.Phone != "" {
			phoneset[user.Phone] = struct{}{}
		}

		bs, err := user.Contacts.MarshalJSON()
		if err != nil {
			logger.Errorf("handle_notice: failed to marshal contacts: %v", err)
			continue
		}

		ret := gjson.GetBytes(bs, "dingtalk_robot_token")
		if ret.Exists() {
			dingtalkset[ret.String()] = struct{}{}
		}

		ret = gjson.GetBytes(bs, "wecom_robot_token")
		if ret.Exists() {
			wecomset[ret.String()] = struct{}{}
		}

		ret = gjson.GetBytes(bs, "feishu_robot_token")
		if ret.Exists() {
			feishuset[ret.String()] = struct{}{}
		}
	}

	phones := StringSetKeys(phoneset)

	for _, ch := range config.C.Alerting.NotifyBuiltinChannels {
		switch ch {
		case "email":
			if len(emailset) == 0 {
				continue
			}
			content := "【内部处理错误】当前标题: " + title + "\n【内部处理错误】当前异常: " + e.Error() + "\n【内部处理错误】发送时间: " + triggerTime
			sender.WriteEmail(title, content, StringSetKeys(emailset))
		case "dingtalk":
			if len(dingtalkset) == 0 {
				continue
			}
			content := "**【内部处理错误】当前标题: **" + title + "\n**【内部处理错误】当前异常: **" + e.Error() + "\n**【内部处理错误】发送时间: **" + triggerTime
			sender.SendDingtalk(sender.DingtalkMessage{
				Title:     title,
				Text:      content,
				AtMobiles: phones,
				Tokens:    StringSetKeys(dingtalkset),
			})
		case "wecom":
			if len(wecomset) == 0 {
				continue
			}
			content := "**【内部处理错误】当前标题: **" + title + "\n**【内部处理错误】当前异常: **" + e.Error() + "\n**【内部处理错误】发送时间: **" + triggerTime
			sender.SendWecom(sender.WecomMessage{
				Text:   content,
				Tokens: StringSetKeys(wecomset),
			})
		case "feishu":
			if len(feishuset) == 0 {
				continue
			}

			content := "【内部处理错误】当前标题: " + title + "\n【内部处理错误】当前异常: " + e.Error() + "\n【内部处理错误】发送时间: " + triggerTime
			sender.SendFeishu(sender.FeishuMessage{
				Text:      content,
				AtMobiles: phones,
				Tokens:    StringSetKeys(feishuset),
			})
		}
	}
}
