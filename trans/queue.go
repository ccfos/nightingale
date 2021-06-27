package trans

import (
	"sync"

	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"
)

type SafeJudgeQueue struct {
	sync.RWMutex
	Data         map[string]*list.SafeListLimited
	QueueMaxSize int
}

var queues = NewJudgeQueue()

func NewJudgeQueue() SafeJudgeQueue {
	return SafeJudgeQueue{
		Data:         make(map[string]*list.SafeListLimited),
		QueueMaxSize: 10240000,
	}
}

func (s *SafeJudgeQueue) Del(instance string) {
	s.Lock()
	delete(s.Data, instance)
	s.Unlock()
}

func (s *SafeJudgeQueue) Set(instance string, q *list.SafeListLimited) {
	s.Lock()
	s.Data[instance] = q
	s.Unlock()
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

func (s *SafeJudgeQueue) Update(instances []string) {
	for _, instance := range instances {
		if !s.Exists(instance) {
			q := list.NewSafeListLimited(s.QueueMaxSize)
			s.Set(instance, q)
			go send2JudgeTask(q, instance)
		}
	}

	toDel := make(map[string]struct{})
	all := s.GetAll()
	for key := range all {
		if !slice.ContainsString(instances, key) {
			toDel[key] = struct{}{}
		}
	}

	for key := range toDel {
		if queue, ok := s.Get(key); ok {
			queue.RemoveAll()
		}
		s.Del(key)
		logger.Infof("server instance %s dead, so remove from judge queues", key)
	}
}
