package timer

import (
	"context"
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/ibex/models"

	"github.com/toolkits/pkg/logger"
)

// CacheHostDoing 缓存task_host_doing表全部内容，减轻DB压力
func CacheHostDoing() {
	if err := cacheHostDoing(); err != nil {
		fmt.Println("cannot cache task_host_doing data: ", err)
	}
	go loopCacheHostDoing()
}

func loopCacheHostDoing() {
	for {
		time.Sleep(time.Millisecond * 400)
		if err := cacheHostDoing(); err != nil {
			logger.Warning("cannot cache task_host_doing data: ", err)
		}
	}
}

func cacheHostDoing() error {
	doingsFromDb, err := models.TableRecordGets[[]models.TaskHostDoing](models.TaskHostDoing{}.TableName(), "")
	if err != nil {
		logger.Errorf("models.TableRecordGets fail: %v", err)
	}

	ctx := context.Background()

	doingsFromRedis, err := models.CacheRecordGets[models.TaskHostDoing](ctx)
	if err != nil {
		logger.Errorf("models.CacheRecordGets fail: %v", err)
	}

	set := make(map[string][]models.TaskHostDoing)
	for _, doing := range doingsFromDb {
		doing.AlertTriggered = false
		set[doing.Host] = append(set[doing.Host], doing)
	}
	for _, doing := range doingsFromRedis {
		doing.AlertTriggered = true
		set[doing.Host] = append(set[doing.Host], doing)
	}

	models.SetDoingCache(set)

	return err
}
