package cache

import (
	"sync"
	"time"

	"github.com/didi/nightingale/src/dataobj"
)

var MetricCached *Cached

func Init() {
	MetricCached = NewCached()
}

type Cached struct {
	sync.RWMutex
	Data map[string]dataobj.MetricValue
}

func NewCached() *Cached {
	h := Cached{
		Data: make(map[string]dataobj.MetricValue),
	}

	go h.Clean()
	return &h
}

func (c *Cached) Set(key string, item dataobj.MetricValue) {
	c.Lock()
	defer c.Unlock()

	c.Data[key] = item
}

func (c *Cached) Get(key string) (dataobj.MetricValue, bool) {
	c.RLock()
	defer c.RUnlock()

	item, exists := c.Data[key]
	return item, exists
}

func (c *Cached) Clean() {
	for range time.Tick(10 * time.Minute) {
		c.clean()
	}
}

func (c *Cached) clean() {
	c.Lock()
	defer c.Unlock()

	now := time.Now().Unix()
	for key, item := range c.Data {
		if now-item.Timestamp > 10*item.Step {
			delete(c.Data, key)
		}
	}
}
