package judge

import (
	"sync"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/models"
)

type SafeEventMap struct {
	sync.RWMutex
	M map[string]*models.AlertEvent
}

var (
	LastEvents = &SafeEventMap{M: make(map[string]*models.AlertEvent)}
)

func (s *SafeEventMap) Get(key string) (*models.AlertEvent, bool) {
	s.RLock()
	defer s.RUnlock()
	event, exists := s.M[key]
	return event, exists
}

func (s *SafeEventMap) Set(key string, event *models.AlertEvent) {
	s.Lock()
	defer s.Unlock()
	s.M[key] = event
}

func (s *SafeEventMap) Del(key string) {
	s.Lock()
	defer s.Unlock()
	delete(s.M, key)
}

func (s *SafeEventMap) DeleteOrSendRecovery(promql string, toKeepKeys map[string]struct{}) {
	s.Lock()
	defer s.Unlock()
	for k, ev := range s.M {
		if _, loaded := toKeepKeys[k]; loaded {
			continue
		}
		if ev.ReadableExpression == promql {
			logger.Debugf("[to_del][ev.IsRecovery:%+v][ev.LastSend:%+v]", ev.IsRecovery, ev.LastSend)
			delete(s.M, k)
			now := time.Now().Unix()
			// promql 没查询到结果，需要将告警标记为已恢复并发送
			// 同时需要满足 已经发送过触发信息，并且时间差满足 大于AlertDuration
			// 为了避免 发送告警后 一个点 断点了就立即发送恢复信息的case
			if ev.IsAlert() && ev.LastSend && now-ev.TriggerTime > ev.AlertDuration {
				ev.MarkRecov()
				EventQueue.PushFront(ev)
			}
		}
	}
}
