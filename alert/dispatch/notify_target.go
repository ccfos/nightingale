package dispatch

import (
	"strconv"

	"github.com/ccfos/nightingale/v6/models"
)

// NotifyTarget 维护所有需要发送的目标 用户-通道/回调/钩子信息,用map维护的数据结构具有去重功能
type NotifyTarget struct {
	userMap   map[int64]NotifyChannels
	webhooks  map[string]*models.Webhook
	callbacks map[string]struct{}
}

func NewNotifyTarget() *NotifyTarget {
	return &NotifyTarget{
		userMap:   make(map[int64]NotifyChannels),
		webhooks:  make(map[string]*models.Webhook),
		callbacks: make(map[string]struct{}),
	}
}

// OrMerge 将 channelMap 按照 or 的方式合并,方便实现多种组合的策略,比如根据某个 tag 进行路由等
func (s *NotifyTarget) OrMerge(other *NotifyTarget) {
	s.merge(other, NotifyChannels.OrMerge)
}

// AndMerge 将 channelMap 中的 bool 值按照 and 的逻辑进行合并,可以单独将人/通道维度的通知移除
// 常用的场景有:
// 1. 人员离职了不需要发送告警了
// 2. 某个告警通道进行维护,暂时不需要发送告警了
// 3. 业务值班的重定向逻辑，将高等级的告警额外发送给应急人员等
// 可以结合业务需求自己实现router
func (s *NotifyTarget) AndMerge(other *NotifyTarget) {
	s.merge(other, NotifyChannels.AndMerge)
}

func (s *NotifyTarget) merge(other *NotifyTarget, f func(NotifyChannels, NotifyChannels)) {
	if other == nil {
		return
	}
	for k, v := range other.userMap {
		if curV, has := s.userMap[k]; has {
			f(curV, v)
		} else {
			s.userMap[k] = v
		}
	}
	for k, v := range other.webhooks {
		s.webhooks[k] = v
	}
	for k, v := range other.callbacks {
		s.callbacks[k] = v
	}
}

// ToChannelUserMap userMap(map[uid][channel]bool) 转换为 map[channel][]uid 的结构
func (s *NotifyTarget) ToChannelUserMap() map[string][]int64 {
	m := make(map[string][]int64)
	for uid, nc := range s.userMap {
		for ch, send := range nc {
			if send {
				m[ch] = append(m[ch], uid)
			}
		}
	}
	return m
}

func (s *NotifyTarget) ToCallbackList() []string {
	callbacks := make([]string, 0, len(s.callbacks))
	for cb := range s.callbacks {
		callbacks = append(callbacks, cb)
	}
	return callbacks
}

func (s *NotifyTarget) ToWebhookList() []*models.Webhook {
	webhooks := make([]*models.Webhook, 0, len(s.webhooks))
	for _, wh := range s.webhooks {
		webhooks = append(webhooks, wh)
	}
	return webhooks
}

// Dispatch 抽象由告警事件到信息接收者的路由策略
// rule: 告警规则
// event: 告警事件
// prev: 前一次路由结果, Dispatch 的实现可以直接修改 prev, 也可以返回一个新的 NotifyTarget 用于 AndMerge/OrMerge
type NotifyTargetDispatch func(rule *models.AlertRule, event *models.AlertCurEvent, prev *NotifyTarget, dispatch *Dispatch) *NotifyTarget

// GroupDispatch 处理告警规则的组订阅关系
func NotifyGroupDispatch(rule *models.AlertRule, event *models.AlertCurEvent, prev *NotifyTarget, dispatch *Dispatch) *NotifyTarget {
	groupIds := make([]int64, 0, len(event.NotifyGroupsJSON))
	for _, groupId := range event.NotifyGroupsJSON {
		gid, err := strconv.ParseInt(groupId, 10, 64)
		if err != nil {
			continue
		}
		groupIds = append(groupIds, gid)
	}

	groups := dispatch.userGroupCache.GetByUserGroupIds(groupIds)
	NotifyTarget := NewNotifyTarget()
	for _, group := range groups {
		for _, userId := range group.UserIds {
			NotifyTarget.userMap[userId] = NewNotifyChannels(event.NotifyChannelsJSON)
		}
	}
	return NotifyTarget
}

func GlobalWebhookDispatch(rule *models.AlertRule, event *models.AlertCurEvent, prev *NotifyTarget, dispatch *Dispatch) *NotifyTarget {
	webhooks := dispatch.notifyConfigCache.GetWebhooks()
	NotifyTarget := NewNotifyTarget()
	for _, webhook := range webhooks {
		if !webhook.Enable {
			continue
		}
		NotifyTarget.webhooks[webhook.Url] = webhook
	}
	return NotifyTarget
}

func EventCallbacksDispatch(rule *models.AlertRule, event *models.AlertCurEvent, prev *NotifyTarget, dispatch *Dispatch) *NotifyTarget {
	for _, c := range event.CallbacksJSON {
		if c == "" {
			continue
		}
		prev.callbacks[c] = struct{}{}
	}
	return nil
}
