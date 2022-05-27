package engine

import (
	"time"

	"github.com/didi/nightingale/v5/src/server/common/sender"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
	"github.com/tidwall/gjson"
	"github.com/toolkits/pkg/logger"
)

// notify to maintainer to handle the error
func notifyToMaintainer(e error, title string) {

	logger.Errorf("notifyToMaintainer，title:%s, error:%v", title, e)

	if len(config.C.Alerting.NotifyBuiltinChannels) == 0 {
		return
	}

	maintainerUsers := memsto.UserCache.GetMaintainerUsers()
	if len(maintainerUsers) == 0 {
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
	triggerTime := time.Now().Format("2006/01/02 - 15:04:05")

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
