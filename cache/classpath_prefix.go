package cache

import (
	"sync"
)

type ClasspathPrefixMap struct {
	sync.RWMutex
	Data map[int64][]int64
}

var ClasspathPrefix = &ClasspathPrefixMap{Data: make(map[int64][]int64)}

func (c *ClasspathPrefixMap) Get(id int64) ([]int64, bool) {
	c.RLock()
	defer c.RUnlock()
	ids, exists := c.Data[id]
	return ids, exists
}

func (c *ClasspathPrefixMap) SetAll(data map[int64][]int64) {
	c.Lock()
	defer c.Unlock()

	c.Data = data
	return
}
