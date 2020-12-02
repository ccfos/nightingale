package cache

import (
	"sync"

	"github.com/didi/nightingale/src/common/dataobj"
)

type SafeAggrCalcMap struct {
	sync.RWMutex
	M         map[string]map[string][]*dataobj.RawMetricAggrCalc
	MetricMap map[int64]string
}

var (
	AggrCalcMap = &SafeAggrCalcMap{
		M:         make(map[string]map[string][]*dataobj.RawMetricAggrCalc),
		MetricMap: make(map[int64]string),
	}
)

func (s *SafeAggrCalcMap) ReInit(m map[string]map[string][]*dataobj.RawMetricAggrCalc) {
	s.Lock()
	defer s.Unlock()
	s.M = m
}

func (s *SafeAggrCalcMap) ReInitMetric(m map[int64]string) {
	s.Lock()
	defer s.Unlock()
	s.MetricMap = m
}

func (s *SafeAggrCalcMap) GetMetric(id int64) string {
	s.RLock()
	defer s.RUnlock()

	return s.MetricMap[id]
}

func (s *SafeAggrCalcMap) GetByKey(key string) []*dataobj.RawMetricAggrCalc {
	s.RLock()
	defer s.RUnlock()
	m, exists := s.M[key[0:2]]
	if !exists {
		return []*dataobj.RawMetricAggrCalc{}
	}

	return m[key]
}

func (s *SafeAggrCalcMap) GetAll() []*dataobj.RawMetricAggrCalc {
	s.RLock()
	defer s.RUnlock()
	stras := make([]*dataobj.RawMetricAggrCalc, 0)
	for _, m := range s.M {
		for _, stra := range m {
			stras = append(stras, stra...)
		}
	}

	return stras
}
