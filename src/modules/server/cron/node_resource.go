package cron

import (
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"
	"github.com/toolkits/pkg/logger"
)

func SyncNodeResources() {
	t1 := time.NewTicker(time.Duration(cache.CHECK_INTERVAL) * time.Second)

	syncNodeResource()
	logger.Info("[cron] sync SyncNodeResources start...")
	for {
		<-t1.C
		syncNodeResource()
	}
}

func syncNodeResource() {
	nodeReource, err := models.NodeResourceGetAll()
	if err != nil {
		logger.Warningf("get all nodeReource err:%v %v", err)
		return
	}

	nodeResource := make(map[int64][]int64)
	for i := range nodeReource {
		nid := nodeReource[i].NodeId
		if _, exists := nodeResource[nid]; !exists {
			nodeResource[nid] = []int64{}
		}
		nodeResource[nid] = append(nodeResource[nid], nodeReource[i].ResId)
	}

	cache.NodeResourceCache.SetAll(nodeResource)
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
	allNode := cache.TreeNodeCache.GetAll()
	if len(allNode) == 0 {
		return
	}

	nodeIdentsMap := make(map[int64][]*models.Resource)
	for i, _ := range allNode {
		nids, err := cache.GetLeafNidsForMon(allNode[i].Id, []int64{})
		if err != nil {
			logger.Errorf("err: %v,cache GetLeafNidsForMon by node id: %+v", err, allNode[i].Id)
			continue
		}

		rids := cache.NodeResourceCache.GetByNids(nids)

		resources := cache.ResourceCache.GetByIds(rids)

		nodeIdentsMap[allNode[i].Id] = resources
	}

	cache.NodeIdentsMapCache.SetAll(nodeIdentsMap)
}
