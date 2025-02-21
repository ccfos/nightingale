package memsto

import (
	"sync"
)

type SourceCountCache struct {
	sourceMetricsStats map[string]int
	mu                 sync.Mutex
}

func NewSourceCountCache() *SourceCountCache {
	sc := &SourceCountCache{
		sourceMetricsStats: make(map[string]int),
	}

	return sc
}

// 将 source ip 记录到 SourceMetricsStats 中，若存在则数量加一
func (s *SourceCountCache) Increase(source string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sourceMetricsStats[source]++
}

func (s *SourceCountCache) GetAndFlush() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentStats := s.sourceMetricsStats
	s.sourceMetricsStats = make(map[string]int)

	return currentStats
}
