package cache

import (
	"sync"

	"github.com/didi/nightingale/v5/models"
)

type ClasspathResMap struct {
	sync.RWMutex
	Data map[int64]*ClasspathAndRes
}

type ClasspathAndRes struct {
	Res       []string
	Classpath *models.Classpath
}

// classpath_id -> classpath & res_idents
var ClasspathRes = &ClasspathResMap{Data: make(map[int64]*ClasspathAndRes)}

func (c *ClasspathResMap) Get(id int64) (*ClasspathAndRes, bool) {
	c.RLock()
	defer c.RUnlock()
	resources, exists := c.Data[id]
	return resources, exists
}

func (c *ClasspathResMap) SetAll(collectRulesMap map[int64]*ClasspathAndRes) {
	c.Lock()
	defer c.Unlock()
	c.Data = collectRulesMap
}
