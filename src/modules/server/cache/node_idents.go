package cache

import (
	"sync"

	"github.com/didi/nightingale/v4/src/models"
)

type NodeIdentsMap struct {
	sync.RWMutex
	Data map[int64][]*models.Resource
}

var NodeIdentsMapCache *NodeIdentsMap

func NewNodeIdentsMapCache() *NodeIdentsMap {
	return &NodeIdentsMap{Data: make(map[int64][]*models.Resource)}
}

func (t *NodeIdentsMap) GetBy(id int64) []*models.Resource {
	t.RLock()
	defer t.RUnlock()

	return t.Data[id]
}

func (t *NodeIdentsMap) SetAll(objs map[int64][]*models.Resource) {
	t.Lock()
	defer t.Unlock()

	t.Data = objs
	return
}
