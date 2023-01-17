package engine

import (
	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
)

// NotifyChannels channelKey -> bool
type NotifyChannels map[string]bool

func NewNotifyChannels(channels []string) NotifyChannels {
	nc := make(NotifyChannels)
	for _, ch := range channels {
		nc[ch] = true
	}
	return nc
}

func (nc NotifyChannels) OrMerge(other NotifyChannels) {
	nc.merge(other, func(a, b bool) bool { return a || b })
}

func (nc NotifyChannels) AndMerge(other NotifyChannels) {
	nc.merge(other, func(a, b bool) bool { return a && b })
}

func (nc NotifyChannels) merge(other NotifyChannels, f func(bool, bool) bool) {
	if other == nil {
		return
	}
	for k, v := range other {
		if curV, has := nc[k]; has {
			nc[k] = f(curV, v)
		} else {
			nc[k] = v
		}
	}
}

// Subscription 维护所有需要发送的用户-通道/回调/钩子信息,用map维护的数据结构具有去重功能
type Subscription struct {
	userMap   map[int64]NotifyChannels
	webhooks  map[string]config.Webhook
	callbacks map[string]struct{}
}

func NewSubscription() *Subscription {
	return &Subscription{
		userMap:   make(map[int64]NotifyChannels),
		webhooks:  make(map[string]config.Webhook),
		callbacks: make(map[string]struct{}),
	}
}

// NewSubscriptionFromUsers 根据用户的token配置,生成订阅信息，用于notifyMaintainer
func NewSubscriptionFromUsers(users []*models.User) *Subscription {
	s := NewSubscription()
	for _, u := range users {
		if u == nil {
			continue
		}
		for channel, token := range u.ExtractAllToken() {
			if token == "" {
				continue
			}
			if channelMap, has := s.userMap[u.Id]; has {
				channelMap[channel] = true
			} else {
				s.userMap[u.Id] = map[string]bool{
					channel: true,
				}
			}
		}
	}
	return s
}

// OrMerge 将channelMap按照or的方式合并,方便实现多种组合的策略,比如根据某个tag进行路由等
func (s *Subscription) OrMerge(other *Subscription) {
	s.merge(other, NotifyChannels.OrMerge)
}

// AndMerge 将channelMap中的bool值按照and的逻辑进行合并,可以单独将人/通道维度的通知移除
// 常用的场景有:
// 1. 人员离职了不需要发送告警了
// 2. 某个告警通道进行维护,暂时不需要发送告警了
// 3. 业务值班的重定向逻辑，将高等级的告警额外发送给应急人员等
// 可以结合业务需求自己实现router
func (s *Subscription) AndMerge(other *Subscription) {
	s.merge(other, NotifyChannels.AndMerge)
}

func (s *Subscription) merge(other *Subscription, f func(NotifyChannels, NotifyChannels)) {
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
func (s *Subscription) ToChannelUserMap() map[string][]int64 {
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

func (s *Subscription) ToCallbackList() []string {
	callbacks := make([]string, 0, len(s.callbacks))
	for cb := range s.callbacks {
		callbacks = append(callbacks, cb)
	}
	return callbacks
}

func (s *Subscription) ToWebhookList() []config.Webhook {
	webhooks := make([]config.Webhook, 0, len(s.webhooks))
	for _, wh := range s.webhooks {
		webhooks = append(webhooks, wh)
	}
	return webhooks
}
