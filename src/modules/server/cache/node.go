package cache

import (
	"sync"

	"github.com/didi/nightingale/v4/src/models"
)

type TreeNodeMap struct {
	sync.RWMutex
	Data map[int64]*models.Node
}

var TreeNodeCache *TreeNodeMap

func NewTreeNodeCache() *TreeNodeMap {
	return &TreeNodeMap{Data: make(map[int64]*models.Node)}
}

func (t *TreeNodeMap) Len() int {
	t.RLock()
	defer t.RUnlock()

	return len(t.Data)
}

func (t *TreeNodeMap) GetBy(id int64) *models.Node {
	t.RLock()
	defer t.RUnlock()

	return t.Data[id]
}

func (t *TreeNodeMap) GetAll() []*models.Node {
	t.RLock()
	defer t.RUnlock()
	var objs []*models.Node
	for _, obj := range t.Data {
		objs = append(objs, obj)
	}

	return objs
}

func (t *TreeNodeMap) GetByIds(ids []int64) []*models.Node {
	t.RLock()
	defer t.RUnlock()
	var objs []*models.Node
	for _, id := range ids {
		objs = append(objs, t.Data[id])
	}

	return objs
}

func (t *TreeNodeMap) SetAll(objs map[int64]*models.Node) {
	t.Lock()
	defer t.Unlock()

	t.Data = objs
	return
}

func (t *TreeNodeMap) GetLeafNidsById(id int64) []int64 {
	t.RLock()
	defer t.RUnlock()

	return t.Data[id].LeafNids
}

func GetLeafNidsForMon(nid int64, exclNid []int64) ([]int64, error) {
	var nids []int64
	idsMap := make(map[int64]struct{})

	node := TreeNodeCache.GetBy(nid)
	if node == nil {
		return []int64{}, nil
	}

	for _, id := range node.LeafNids {
		idsMap[id] = struct{}{}
	}

	for _, id := range exclNid {
		node := TreeNodeCache.GetBy(nid)
		if node == nil {
			continue
		}

		if node.Leaf == 1 {
			delete(idsMap, id)
		} else {
			for _, id := range node.LeafNids {
				delete(idsMap, id)
			}
		}
	}

	for id, _ := range idsMap {
		nids = append(nids, id)
	}

	return nids, nil
}

func GetRelatedNidsForMon(nid int64, exclNid []int64) ([]int64, error) {
	var nids []int64
	idsMap := make(map[int64]struct{})

	node := TreeNodeCache.GetBy(nid)
	if node == nil {
		return []int64{}, nil
	}

	if node == nil {
		return nids, nil
	}

	for _, id := range node.LeafNids {
		idsMap[id] = struct{}{}
	}
	idsMap[node.Id] = struct{}{}

	for _, id := range exclNid {
		node := TreeNodeCache.GetBy(nid)
		if node == nil {
			continue
		}

		if node.Leaf == 1 {
			delete(idsMap, id)
		} else {
			for _, id := range node.LeafNids {
				delete(idsMap, id)
			}
			delete(idsMap, id)
		}
	}

	for id, _ := range idsMap {
		nids = append(nids, id)
	}

	return nids, nil
}
