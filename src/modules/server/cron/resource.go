package cron

import (
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"

	"github.com/toolkits/pkg/logger"
)

func SyncResources() {
	t1 := time.NewTicker(time.Duration(cache.CHECK_INTERVAL) * time.Second)

	syncResource()
	logger.Info("[cron] sync SyncResources start...")
	for {
		<-t1.C
		syncResource()
	}
}

func syncResource() {
	resources, err := models.ResourceGets("")
	if err != nil {
		logger.Warningf("get all resources err:%v %v", err)
		return
	}

	resourceMap := make(map[int64]*models.Resource)
	for i := range resources {
		resourceMap[resources[i].Id] = &resources[i]
	}
	cache.ResourceCache.SetAll(resourceMap)
}
