package stats

import "sync"

type CounterMetric struct {
	sync.RWMutex
	prefix  string
	metrics map[string]int
}

var Counter *CounterMetric

func NewCounter(prefix string) *CounterMetric {
	return &CounterMetric{
		metrics: make(map[string]int),
		prefix:  prefix,
	}
}

func (c *CounterMetric) Set(metric string, value int) {
	c.Lock()
	defer c.Unlock()
	if _, exists := c.metrics[metric]; exists {
		c.metrics[metric] += value
	} else {
		c.metrics[metric] = value
	}
}

func (c *CounterMetric) Dump() map[string]int {
	c.Lock()
	defer c.Unlock()
	metrics := make(map[string]int)
	for key, value := range c.metrics {
		newKey := c.prefix + "." + key
		metrics[newKey] = value
		c.metrics[key] = 0
	}

	return metrics
}
