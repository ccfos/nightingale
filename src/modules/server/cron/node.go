package cron

import (
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"

	"github.com/toolkits/pkg/logger"
)

func SyncTreeNodes() {
	t1 := time.NewTicker(time.Duration(cache.CHECK_INTERVAL) * time.Second)

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
		nids, err := allNode[i].LeafIds()
		if err != nil {
			logger.Errorf("err: %v,cache GetLeafNidsForMon by node id: %+v", err, allNode[i].Id)
		}
		allNode[i].LeafNids = nids
		nodeMap[allNode[i].Id] = &allNode[i]
	}

	cache.TreeNodeCache.SetAll(nodeMap)
}
