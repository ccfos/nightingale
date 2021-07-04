package cache

import (
	"github.com/toolkits/pkg/logger"

	"sync"
	"time"

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

func (t *TreeNodeMap) GetLeafNidsById(id int64) []int64 {
	t.RLock()
	defer t.RUnlock()

	return t.Data[id].LeafNids
}


func SyncTreeNodes() {
	t1 := time.NewTicker(time.Duration(CHECK_INTERVAL) * time.Second)

	syncTreeNode()
	logger.Info("[cron] sync SyncTreeNodes start...")
	for {
		<-t1.C
		syncTreeNode()
	}
}

func syncTreeNode() {
	allNode, err := models.NodeGets("")
	if err != nil {
		logger.Warningf("get all Node err:%v %v", err)
		return
	}

	nodeMap := make(map[int64]*models.Node)
	for i, _ := range allNode {
		nids, err := models.GetLeafNidsForMon(allNode[i].Id, []int64{})
		if err != nil {
			logger.Errorf("err: %v,cache GetLeafNidsForMon by node id: %+v", err, allNode[i].Id)
		}
		allNode[i].LeafNids = nids
		nodeMap[allNode[i].Id] = &allNode[i]
	}

	TreeNodeCache.SetAll(nodeMap)
}
