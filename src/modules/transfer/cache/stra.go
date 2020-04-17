package cache

import (
	"sync"

	"github.com/didi/nightingale/src/model"
)

type SafeStraMap struct {
	sync.RWMutex
	M map[string]map[string][]*model.Stra
}

var (
	StraMap = &SafeStraMap{M: make(map[string]map[string][]*model.Stra)}
)

func (s *SafeStraMap) ReInit(m map[string]map[string][]*model.Stra) {
	s.Lock()
	defer s.Unlock()
	s.M = m
}

func (s *SafeStraMap) GetByKey(key string) []*model.Stra {
	s.RLock()
	defer s.RUnlock()
	m, exists := s.M[key[0:2]]
	if !exists {
		return []*model.Stra{}
	}

	return m[key]
}

func (s *SafeStraMap) GetAll() []*model.Stra {
	s.RLock()
	defer s.RUnlock()
	stras := make([]*model.Stra, 0)
	for _, m := range s.M {
		for _, stra := range m {
			stras = append(stras, stra...)
		}
	}

	return stras
}
