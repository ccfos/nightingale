package cron

import (
	"strconv"
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"
	"github.com/toolkits/pkg/logger"
)

func SyncApiCollects() {
	t1 := time.NewTicker(time.Duration(cache.CHECK_INTERVAL) * time.Second)

	syncApiCollects()
	logger.Info("[cron] sync api collects start...")
	for {
		<-t1.C
		syncApiCollects()
	}
}

func syncApiCollects() {
	apiConfigs, err := models.GetApiCollects()
	if err != nil {
		logger.Warningf("get log collects err:%v %v", err)
	}

	configsMap := make(map[string][]*models.ApiCollect)
	for _, api := range apiConfigs {
		if _, exists := cache.ApiDetectorHashRing[api.Region]; !exists {
			logger.Warningf("get node err, hash ring do noe exists %v", api)
			continue
		}
		node, err := cache.ApiDetectorHashRing[api.Region].GetNode(strconv.FormatInt(api.Id, 10))
		if err != nil {
			logger.Warningf("get node err:%v %v", err, api)
			continue
		}
		api.Decode()
		key := api.Region + "-" + node
		if _, exists := configsMap[key]; exists {
			configsMap[key] = append(configsMap[key], api)
		} else {
			configsMap[key] = []*models.ApiCollect{api}
		}
	}

	cache.ApiCollectCache.SetAll(configsMap)
}
