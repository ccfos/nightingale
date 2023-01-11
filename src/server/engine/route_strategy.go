package engine

import (
	"strconv"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
)

// Router 抽象由告警事件到订阅者的路由策略
// rule: 告警规则
// event: 告警事件
// prev: 前一次路由结果, Router的实现可以直接修改prev, 也可以返回一个新的Subscription用于AndMerge/OrMerge
type Router func(rule *models.AlertRule, event *models.AlertCurEvent, prev *Subscription) *Subscription

// GroupRouter 处理告警规则的组订阅关系
func GroupRouter(rule *models.AlertRule, event *models.AlertCurEvent, prev *Subscription) *Subscription {
	groupIds := make([]int64, 0, len(event.NotifyGroupsJSON))
	for _, groupId := range event.NotifyGroupsJSON {
		gid, err := strconv.ParseInt(groupId, 10, 64)
		if err != nil {
			continue
		}
		groupIds = append(groupIds, gid)
	}
	groups := memsto.UserGroupCache.GetByUserGroupIds(groupIds)
	subscription := NewSubscription()
	for _, group := range groups {
		for _, userId := range group.UserIds {
			subscription.userMap[userId] = NewNotifyChannels(event.NotifyChannelsJSON)
		}
	}
	return subscription
}

func GlobalWebhookRouter(rule *models.AlertRule, event *models.AlertCurEvent, prev *Subscription) *Subscription {
	conf := config.C.Alerting.Webhook
	if !conf.Enable {
		return nil
	}
	subscription := NewSubscription()
	subscription.webhooks[conf.Url] = conf
	return subscription
}

func EventCallbacksRouter(rule *models.AlertRule, event *models.AlertCurEvent, prev *Subscription) *Subscription {
	for _, c := range event.CallbacksJSON {
		if c == "" {
			continue
		}
		prev.callbacks[c] = struct{}{}
	}
	return nil
}
