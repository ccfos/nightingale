package cache

import (
	"sync"

	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/logger"
)

// Counter: sorted tags
type CounterTsMap struct {
	sync.RWMutex
	M map[string]int64 `json:"counters"` // map[counter]ts
}

func NewCounterTsMap() *CounterTsMap {
	return &CounterTsMap{M: make(map[string]int64)}
}

func (c *CounterTsMap) Set(counter string, ts int64) {
	c.Lock()
	defer c.Unlock()
	c.M[counter] = ts
}

func (c *CounterTsMap) Clean(now, timeDuration int64, endpoint, metric string) {
	c.Lock()
	defer c.Unlock()
	for counter, ts := range c.M {
		if now-ts > timeDuration {
			delete(c.M, counter)
			stats.Counter.Set("counter.clean", 1)

			logger.Debugf("clean endpoint index:%s metric:%s counter:%s", endpoint, metric, counter)
		}
	}
}

func (c *CounterTsMap) GetCounters() map[string]int64 {
	c.RLock()
	defer c.RUnlock()
	m := make(map[string]int64)
	for k, v := range c.M {
		m[k] = v
	}
	return m
}

func (c *CounterTsMap) Len() int {
	c.RLock()
	defer c.RUnlock()
	return len(c.M)
}
