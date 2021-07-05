package cache

import (
	"sync"
)

var NodeResourceCache *NodeResourceMap

type NodeResourceMap struct {
	sync.RWMutex
	Data map[int64][]int64
}

func NewNodeResourceCache() *NodeResourceMap {
	return &NodeResourceMap{Data: make(map[int64][]int64)}
}

func (t *NodeResourceMap) GetResourceBy(id int64) []int64 {
	t.RLock()
	defer t.RUnlock()

	return t.Data[id]
}

func (t *NodeResourceMap) GetByNids(ids []int64) []int64 {
	t.RLock()
	defer t.RUnlock()
	var objs []int64
	m := make(map[int64]struct{})
	for _, id := range ids {
		for _, resourceId := range t.Data[id] {
			m[resourceId] = struct{}{}
		}
	}

	for rid, _ := range m {
		objs = append(objs, rid)
	}

	return objs
}

func (t *NodeResourceMap) SetAll(objs map[int64][]int64) {
	t.Lock()
	defer t.Unlock()

	t.Data = objs
	return
}
