package cron

import (
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"

	"github.com/toolkits/pkg/logger"
)

func SyncHardwares() {
	t1 := time.NewTicker(time.Duration(cache.CHECK_INTERVAL) * time.Second)

	syncHardwares()
	logger.Info("[cron] sync snmp collects start...")
	for {
		<-t1.C
		syncHardwares()
	}
}

func syncHardwares() {
	configsMap := make(map[string][]*models.NetworkHardware)

	hwList, err := models.NetworkHardwareList("", 10000000, 0)
	if err != nil {
		logger.Warningf("get snmp hw err:%v", err)
		return
	}

	for i := range hwList {
		if _, exists := cache.SnmpDetectorHashRing[hwList[i].Region]; !exists {
			logger.Warningf("get node err, hash ring do noe exists %v", hwList[i])
			continue
		}
		node, err := cache.SnmpDetectorHashRing[hwList[i].Region].GetNode(hwList[i].IP)
		if err != nil {
			logger.Warningf("get node err:%v %v", err, hwList[i])
			continue
		}

		key := hwList[i].Region + "-" + node
		if _, exists := configsMap[key]; exists {
			configsMap[key] = append(configsMap[key], &hwList[i])
		} else {
			configsMap[key] = []*models.NetworkHardware{&hwList[i]}
		}
	}

	cache.SnmpHWCache.SetAll(configsMap)
}
