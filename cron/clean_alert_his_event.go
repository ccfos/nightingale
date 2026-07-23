package cron

import (
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/robfig/cron/v3"
	"github.com/toolkits/pkg/logger"
)

const (
	hisEventBatchSize = 500 // 每批删除数量
	hisEventSleepMs   = 100 // 每批删除后休眠时间（毫秒），防止长时间锁表
)

// cleanAlertHisEventInBatches 按 id 游标分批删除过期的历史告警事件，
// 活跃告警的历史记录跳过不删（cur.id 即对应的 his.id）
func cleanAlertHisEventInBatches(ctx *ctx.Context, day int) {
	threshold := time.Now().Unix() - 86400*int64(day)

	ids, err := models.AlertCurEventIds(ctx)
	if err != nil {
		logger.Errorf("Failed to clean alert history events: query active event ids fail: %v", err)
		return
	}
	activeIds := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		activeIds[id] = struct{}{}
	}

	var totalDeleted int64
	minId := int64(0)
	for {
		fetched, deleted, maxId, err := models.AlertHisEventBatchDelete(ctx, threshold, nil, minId, hisEventBatchSize, activeIds)
		if err != nil {
			logger.Errorf("Failed to clean alert history events in batch: %v", err)
			return
		}

		totalDeleted += deleted

		if fetched < hisEventBatchSize {
			break
		}
		minId = maxId

		time.Sleep(time.Duration(hisEventSleepMs) * time.Millisecond)
	}

	if totalDeleted > 0 {
		logger.Infof("Cleaned %d alert history events older than %d days", totalDeleted, day)
	}
}

// CleanAlertHisEvent starts a cron job to clean old alert history events in batches
// Runs daily at 2:00 AM
// day: 数据保留天数。历史告警默认永久保留，day <= 0 表示不开启自动清理
func CleanAlertHisEvent(ctx *ctx.Context, day int) {
	if day <= 0 {
		return
	}

	c := cron.New()
	_, err := c.AddFunc("0 2 * * *", func() {
		cleanAlertHisEventInBatches(ctx, day)
	})

	if err != nil {
		logger.Errorf("Failed to add clean alert history event cron job: %v", err)
		return
	}

	c.Start()
	logger.Infof("Alert history event cleanup cron started, retention: %d days, batch_size: %d, sleep_ms: %d", day, hisEventBatchSize, hisEventSleepMs)
}
