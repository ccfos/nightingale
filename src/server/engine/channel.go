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
	if other == nil {
		return
	}
	for k, v := range other {
		if curV, has := nc[k]; has {
			nc[k] = curV || v
		} else {
			nc[k] = v
		}
	}
}

func (nc NotifyChannels) AndMerge(other NotifyChannels) {
	if other == nil {
		return
	}
	for k, v := range other {
		if curV, has := nc[k]; has {
			nc[k] = curV && v
		} else {
			nc[k] = v
		}
	}
}

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
				channelMap := make(map[string]bool)
				channelMap[channel] = true
				s.userMap[u.Id] = channelMap
			}
		}
	}
	return s
}

func (s *Subscription) OrMerge(other *Subscription) {
	if other == nil {
		return
	}
	for k, v := range other.userMap {
		if curV, has := s.userMap[k]; has {
			curV.OrMerge(v)
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

func (s *Subscription) AndMerge(other *Subscription) {
	if other == nil {
		return
	}
	for k, v := range other.userMap {
		if curV, has := s.userMap[k]; has {
			curV.AndMerge(v)
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
