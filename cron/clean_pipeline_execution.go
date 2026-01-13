package cron

import (
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/robfig/cron/v3"
	"github.com/toolkits/pkg/logger"
)

const (
	defaultBatchSize = 100 // 每批删除数量
	defaultSleepMs   = 10  // 每批删除后休眠时间（毫秒）
)

// cleanPipelineExecutionInBatches 分批删除执行记录，避免大批量删除影响数据库性能
func cleanPipelineExecutionInBatches(ctx *ctx.Context, day int) {
	threshold := time.Now().Unix() - 86400*int64(day)
	var totalDeleted int64

	for {
		deleted, err := models.DeleteEventPipelineExecutionsInBatches(ctx, threshold, defaultBatchSize)
		if err != nil {
			logger.Errorf("Failed to clean pipeline execution records in batch: %v", err)
			return
		}

		totalDeleted += deleted

		// 如果本批删除数量小于 batchSize，说明已删除完毕
		if deleted < int64(defaultBatchSize) {
			break
		}

		// 休眠一段时间，降低数据库压力
		time.Sleep(time.Duration(defaultSleepMs) * time.Millisecond)
	}

	if totalDeleted > 0 {
		logger.Infof("Cleaned %d pipeline execution records older than %d days", totalDeleted, day)
	}
}

// CleanPipelineExecution starts a cron job to clean old pipeline execution records in batches
// Runs daily at 6:00 AM
// day: 数据保留天数，默认 7 天
// 使用分批删除方式，每批 1000 条，间隔 100ms，避免大批量删除影响数据库性能
func CleanPipelineExecution(ctx *ctx.Context, day int) {
	c := cron.New()
	if day < 1 {
		day = 7 // default retention: 7 days
	}

	_, err := c.AddFunc("0 6 * * *", func() {
		cleanPipelineExecutionInBatches(ctx, day)
	})

	if err != nil {
		logger.Errorf("Failed to add clean pipeline execution cron job: %v", err)
		return
	}

	c.Start()
	logger.Infof("Pipeline execution cleanup cron started, retention: %d days, batch_size: %d, sleep_ms: %d", day, defaultBatchSize, defaultSleepMs)
}
