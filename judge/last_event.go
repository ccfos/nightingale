package judge

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/didi/nightingale/v5/models"
	"github.com/toolkits/pkg/logger"
)

// rule_id -> hash_id -> *models.AlertEvent
type SafeEventMap struct {
	sync.RWMutex
	M map[int64]map[string]*models.AlertEvent
}

var (
	LastEvents = &SafeEventMap{M: make(map[int64]map[string]*models.AlertEvent)}
)

func (s *SafeEventMap) Get(ruleId int64, hashId string) (*models.AlertEvent, bool) {
	s.RLock()
	defer s.RUnlock()

	m, has := s.M[ruleId]
	if !has {
		return nil, false
	}

	event, has := m[hashId]
	return event, has
}

func (s *SafeEventMap) Set(event *models.AlertEvent) {
	s.Lock()
	defer s.Unlock()

	_, has := s.M[event.RuleId]
	if !has {
		m := make(map[string]*models.AlertEvent)
		m[event.HashId] = event
		s.M[event.RuleId] = m
	} else {
		s.M[event.RuleId][event.HashId] = event
	}
}

func (s *SafeEventMap) Init() {
	aes, err := models.AlertEventGetAll()
	if err != nil {
		fmt.Println("load all alert_event fail:", err)
		os.Exit(1)
	}

	if len(aes) == 0 {
		return
	}

	data := make(map[int64]map[string]*models.AlertEvent)
	for i := 0; i < len(aes); i++ {
		event := aes[i]
		_, has := data[event.RuleId]
		if !has {
			m := make(map[string]*models.AlertEvent)
			m[event.HashId] = event
			data[event.RuleId] = m
		} else {
			data[event.RuleId][event.HashId] = event
		}
	}

	s.Lock()
	s.M = data
	s.Unlock()
}

func (s *SafeEventMap) Del(ruleId int64, hashId string) {
	s.Lock()
	defer s.Unlock()

	_, has := s.M[ruleId]
	if !has {
		return
	}

	delete(s.M[ruleId], hashId)
}

func (s *SafeEventMap) DeleteOrSendRecovery(ruleId int64, toKeepKeys map[string]struct{}) {
	s.Lock()
	defer s.Unlock()

	m, has := s.M[ruleId]
	if !has {
		return
	}

	for k, ev := range m {
		if _, loaded := toKeepKeys[k]; loaded {
			continue
		}

		// 如果因为promql修改，导致本来是告警状态变成了恢复，也接受
		logger.Debugf("[to_del][ev.IsRecovery:%+v][ev.LastSend:%+v]", ev.IsRecovery, ev.LastSend)

		// promql 没查询到结果，需要将告警标记为已恢复并发送
		// 同时需要满足 已经发送过触发信息，并且时间差满足 大于AlertDuration
		// 为了避免 发送告警后 一个点 断点了就立即发送恢复信息的case
		now := time.Now().Unix()
		if ev.IsAlert() && ev.LastSend && now-ev.TriggerTime > ev.AlertDuration {
			logger.Debugf("[prom.alert.MarkRecov][ev.RuleName:%v]", ev.RuleName)
			ev.MarkRecov()
			EventQueue.PushFront(ev)
			delete(s.M[ruleId], k)
		}
	}
}
