package scache

import (
	"sync"

	"github.com/didi/nightingale/src/models"
)

type CollectCacheMap struct {
	sync.RWMutex
	Data map[string]*models.Collect
}

var CollectCache *CollectCacheMap

func NewCollectCache() *CollectCacheMap {
	return &CollectCacheMap{Data: make(map[string]*models.Collect)}
}

func (c *CollectCacheMap) GetBy(endpoint string) *models.Collect {
	c.RLock()
	defer c.RUnlock()

	return c.Data[endpoint]
}

func (c *CollectCacheMap) Set(endpoint string, collect *models.Collect) {
	c.Lock()
	defer c.Unlock()

	c.Data[endpoint] = collect
	return
}

func (c *CollectCacheMap) SetAll(strasMap map[string]*models.Collect) {
	c.Lock()
	defer c.Unlock()

	c.Data = strasMap
	return
}
