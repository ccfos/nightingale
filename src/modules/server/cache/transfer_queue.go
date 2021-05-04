package cache

import (
	"sync"
	"time"

	"github.com/toolkits/pkg/container/list"
)

type SafeJudgeQueue struct {
	sync.RWMutex
	Data map[string]*list.SafeListLimited
	Ts   map[string]int64
}

func NewJudgeQueue() SafeJudgeQueue {
	return SafeJudgeQueue{
		Data: make(map[string]*list.SafeListLimited),
		Ts:   make(map[string]int64),
	}
}

func (s *SafeJudgeQueue) Del(instance string) {
	s.Lock()
	defer s.Unlock()
	delete(s.Data, instance)
}

func (s *SafeJudgeQueue) Set(instance string, q *list.SafeListLimited) {
	s.Lock()
	defer s.Unlock()
	s.Data[instance] = q
	s.Ts[instance] = time.Now().Unix()
}

func (s *SafeJudgeQueue) Get(instance string) (*list.SafeListLimited, bool) {
	s.RLock()
	defer s.RUnlock()
	q, exists := s.Data[instance]
	return q, exists
}

func (s *SafeJudgeQueue) Exists(instance string) bool {
	s.RLock()
	defer s.RUnlock()
	_, exists := s.Data[instance]
	return exists
}

func (s *SafeJudgeQueue) GetAll() map[string]*list.SafeListLimited {
	s.RLock()
	defer s.RUnlock()
	return s.Data
}

func (s *SafeJudgeQueue) UpdateTS(instance string) {
	s.Lock()
	defer s.Unlock()
	s.Ts[instance] = time.Now().Unix()
}

func (s *SafeJudgeQueue) Clean() {
	s.Lock()
	defer s.Unlock()

	for instance, ts := range s.Ts {
		if time.Now().Unix()-ts > 3600 {
			delete(s.Data, instance)
			delete(s.Ts, instance)
		}
	}
}
