package engine

import (
	"encoding/json"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/notifier"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
)

type MaintainMessage struct {
	Tos     []*models.User `json:"tos"`
	Title   string         `json:"title"`
	Content string         `json:"content"`
}

// notify to maintainer to handle the error
func notifyToMaintainer(title, msg string) {
	logger.Errorf("notifyToMaintainer, msg: %s", msg)

	users := memsto.UserCache.GetMaintainerUsers()
	if len(users) == 0 {
		return
	}

	triggerTime := time.Now().Format("2006/01/02 - 15:04:05")
	msg = "Title: " + title + "\nContent: " + msg + "\nTime: " + triggerTime

	notifyMaintainerWithPlugin(title, msg, users)
	notifyMaintainerWithBuiltin(title, msg, users)
}

func notifyMaintainerWithPlugin(title, msg string, users []*models.User) {
	if !config.C.Alerting.CallPlugin.Enable {
		return
	}

	stdinBytes, err := json.Marshal(MaintainMessage{
		Tos:     users,
		Title:   title,
		Content: msg,
	})

	if err != nil {
		logger.Error("failed to marshal MaintainMessage:", err)
		return
	}

	notifier.Instance.NotifyMaintainer(stdinBytes)
	logger.Debugf("notify maintainer with plugin done")
}

func notifyMaintainerWithBuiltin(title, msg string, users []*models.User) {
	subscription := NewSubscriptionFromUsers(users)
	for channel, uids := range subscription.ToChannelUserMap() {
		currentUsers := memsto.UserCache.GetByUserIds(uids)
		rwLock.RLock()
		s := Senders[channel]
		rwLock.RUnlock()
		if s == nil {
			logger.Warningf("no sender for channel: %s", channel)
			continue
		}
		go s.SendRaw(currentUsers, title, msg)
	}
}
