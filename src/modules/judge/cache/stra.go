package cache

import (
	"sync"
	"time"

	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/toolkits/stats"
)

var Strategy *StrategyMap
var NodataStra *StrategyMap

type StrategyMap struct {
	sync.RWMutex
	Data map[int64]*model.Stra
	TS   map[int64]int64
}

func NewStrategyMap() *StrategyMap {
	stra := &StrategyMap{
		Data: make(map[int64]*model.Stra),
		TS:   make(map[int64]int64),
	}
	return stra
}

func (s *StrategyMap) Set(id int64, stra *model.Stra) {
	s.Lock()
	defer s.Unlock()
	s.Data[id] = stra
	s.TS[id] = time.Now().Unix()
}

func (s *StrategyMap) Get(id int64) (*model.Stra, bool) {
	s.RLock()
	defer s.RUnlock()

	stra, exists := s.Data[id]
	return stra, exists
}

func (s *StrategyMap) GetAll() []*model.Stra {
	s.RLock()
	defer s.RUnlock()
	var stras []*model.Stra
	for _, stra := range s.Data {
		stras = append(stras, stra)
	}
	return stras
}

func (s *StrategyMap) Clean() {
	s.Lock()
	defer s.Unlock()
	now := time.Now().Unix()
	for id, ts := range s.TS {
		if now-ts > 60 {
			stats.Counter.Set("stra.clean", 1)
			delete(s.Data, id)
			delete(s.TS, id)
		}
	}
}
