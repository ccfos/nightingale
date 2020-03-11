package scache

import (
	"sync"

	"github.com/didi/nightingale/src/model"
)

type CollectCacheMap struct {
	sync.RWMutex
	Data map[string]*model.Collect
}

var CollectCache *CollectCacheMap

func NewCollectCache() *CollectCacheMap {
	return &CollectCacheMap{Data: make(map[string]*model.Collect)}
}

func (c *CollectCacheMap) GetBy(endpoint string) *model.Collect {
	c.RLock()
	defer c.RUnlock()

	return c.Data[endpoint]
}

func (c *CollectCacheMap) Set(endpoint string, collect *model.Collect) {
	c.Lock()
	defer c.Unlock()

	c.Data[endpoint] = collect
	return
}

func (c *CollectCacheMap) SetAll(strasMap map[string]*model.Collect) {
	c.Lock()
	defer c.Unlock()

	c.Data = strasMap
	return
}
