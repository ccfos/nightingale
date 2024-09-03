package timer

import (
	"fmt"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"time"

	"github.com/toolkits/pkg/logger"
)

// CacheHostDoing 缓存task_host_doing表全部内容，减轻DB压力
func CacheHostDoing(ctx *ctx.Context) {
	if err := cacheHostDoing(ctx); err != nil {
		fmt.Println("cannot cache task_host_doing data: ", err)
	}
	go loopCacheHostDoing(ctx)
}

func loopCacheHostDoing(ctx *ctx.Context) {
	for {
		time.Sleep(time.Millisecond * 400)
		if err := cacheHostDoing(ctx); err != nil {
			logger.Warning("cannot cache task_host_doing data: ", err)
		}
	}
}

func cacheHostDoing(ctx *ctx.Context) error {
	doingsFromDb, err := models.TableRecordGets[[]models.TaskHostDoing](ctx, models.TaskHostDoing{}.TableName(), "")
	if err != nil {
		logger.Errorf("models.TableRecordGets fail: %v", err)
	}

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
