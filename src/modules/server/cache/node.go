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

func (t *TreeNodeMap) GetBy(id int64) *models.Node {
	t.RLock()
	defer t.RUnlock()

	return t.Data[id]
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
