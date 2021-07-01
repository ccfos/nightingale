package cache

import (
	"github.com/toolkits/pkg/logger"

	"sync"
	"time"

	"github.com/didi/nightingale/v4/src/models"
)

type NodeIdentsMap struct {
	sync.RWMutex
	Data map[int64][]string
}

var NodeIdentsMapCache *NodeIdentsMap

func NewNodeIdentsMapCache() *NodeIdentsMap {
	return &NodeIdentsMap{Data: make(map[int64][]string)}
}

func (t *NodeIdentsMap) GetBy(id int64) []string {
	t.RLock()
	defer t.RUnlock()

	return t.Data[id]
}

func (t *NodeIdentsMap) SetAll(objs map[int64][]string) {
	t.Lock()
	defer t.Unlock()

	t.Data = objs
	return
}

func SyncIdentsOfNode() {
	t1 := time.NewTicker(time.Duration(60) * time.Second)

	syncIdentsOfNode()
	logger.Info("[cron] sync IdentsOfNode cron start...")
	for {
		<-t1.C
		logger.Info("[cron] sync IdentsOfNode start...")
		syncIdentsOfNode()
		logger.Info("[cron] sync IdentsOfNode end...")

	}
}

func syncIdentsOfNode() {
	allNode, err := models.NodeGets("")
	if err != nil {
		logger.Warningf("get all Node err:%v %v", err)
		return
	}

	nodeIdentsMap := make(map[int64][]string)
	for i, _ := range allNode {
		nids, err := models.GetLeafNidsForMon(allNode[i].Id, []int64{})
		if err != nil {
			logger.Errorf("err: %v,cache GetLeafNidsForMon by node id: %+v", err, allNode[i].Id)
		}
		rids, err := models.ResIdsGetByNodeIds(nids)
		if err != nil {
			logger.Errorf("err: %v,ResIdsGetByNodeIds node id: %+v", err, nids)
		}

		idents, err := models.ResourceIdentsByIds(rids)
		if err != nil {
			logger.Errorf("err: %v,ResourceIdentsByIds resource id: %+v", err, rids)
		}
		nodeIdentsMap[allNode[i].Id] = idents
	}

	NodeIdentsMapCache.SetAll(nodeIdentsMap)
}
