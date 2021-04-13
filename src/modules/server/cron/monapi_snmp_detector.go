package cron

import (
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"

	"github.com/toolkits/pkg/consistent"
	"github.com/toolkits/pkg/logger"
)

func CheckSnmpDetectorNodes() {
	t1 := time.NewTicker(time.Duration(cache.CHECK_INTERVAL * time.Second))
	checkSnmpDetectorNodes()
	for {
		<-t1.C
		checkSnmpDetectorNodes()
	}
}

func checkSnmpDetectorNodes() {
	detectors, err := models.GetAllInstances("snmp", 1)
	if err != nil {
		logger.Errorf("get api detector err:%v", err)
		return
	}

	if len(detectors) < 1 {
		logger.Error("get api detector err: len(detectors) < 1 ")
		return
	}

	nodesMap := make(map[string]map[string]struct{})
	for _, d := range detectors {
		if d.Active {
			if _, exists := nodesMap[d.Region]; exists {
				nodesMap[d.Region][d.Identity] = struct{}{}
			} else {
				nodesMap[d.Region] = make(map[string]struct{})
				nodesMap[d.Region][d.Identity] = struct{}{}
			}
		}
	}

	for region, nodes := range nodesMap {
		rehash := false
		if _, exists := cache.SnmpDetectorHashRing[region]; !exists {
			logger.Warningf("hash ring do not exists %v", region)
			continue
		}
		oldNodes := cache.SnmpDetectorHashRing[region].GetRing().Members()
		if len(oldNodes) != len(nodes) { //ActiveNode中的node数量和新获取的不同，重新rehash
			rehash = true
		} else {
			for _, node := range oldNodes {
				if _, exists := nodes[node]; !exists {
					rehash = true
					break
				}
			}
		}

		if rehash {
			//重建 hash环
			r := consistent.New()
			r.NumberOfReplicas = 500
			for node, _ := range nodes {
				r.Add(node)
			}
			logger.Warningf("detector hash ring rebuild old:%v new:%v", oldNodes, r.Members())
			cache.SnmpDetectorHashRing[region].Set(r)
		}
	}

	return
}
