package scache

import (
	"sync"

	"github.com/didi/nightingale/src/models"
)

type StraCacheMap struct {
	sync.RWMutex
	Data map[string][]*models.Stra
}

var StraCache *StraCacheMap

func NewStraCache() *StraCacheMap {
	return &StraCacheMap{Data: make(map[string][]*models.Stra)}
}

func (s *StraCacheMap) GetByNode(node string) []*models.Stra {
	s.RLock()
	defer s.RUnlock()

	return s.Data[node]
}

func (s *StraCacheMap) Set(node string, stras []*models.Stra) {
	s.Lock()
	defer s.Unlock()

	s.Data[node] = stras
	return
}

func (s *StraCacheMap) SetAll(strasMap map[string][]*models.Stra) {
	s.Lock()
	defer s.Unlock()

	s.Data = strasMap
	return
}

func (s *StraCacheMap) GetAll() []*models.Stra {
	s.Lock()
	defer s.Unlock()

	data := []*models.Stra{}
	for node, stras := range s.Data {
		instance, exists := ActiveJudgeNode.GetInstanceBy(node)
		if !exists {
			continue
		}
		for _, s := range stras {
			s.JudgeInstance = instance
			data = append(data, s)
		}
	}

	return data
}
