package cron

import (
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/robfig/cron/v3"
	"github.com/toolkits/pkg/logger"
)

const (
	// 单批删除行数：太大则单条 DELETE 持锁久、binlog 事务大导致主从延迟；太小则往返次数多
	cleanNotifyRecordBatchSize = 2000
	// 批次之间让出 IO 给在线请求
	cleanNotifyRecordBatchPause = 100 * time.Millisecond
	// 批次总数上限，兜住「写入快过删除」这类清不完的场景，剩余部分留给下一次调度
	cleanNotifyRecordMaxBatch = 5000
)

func cleanNotifyRecord(ctx *ctx.Context, day int) {
	lastWeek := time.Now().Unix() - 86400*int64(day)

	var total int64
	for i := 0; i < cleanNotifyRecordMaxBatch; i++ {
		deleted, err := models.NotificationRecordDeleteBefore(ctx, lastWeek, cleanNotifyRecordBatchSize)
		if err != nil {
			logger.Errorf("Failed to clean notify record: %v", err)
			return
		}

		total += deleted
		if deleted < cleanNotifyRecordBatchSize {
			break
		}

		time.Sleep(cleanNotifyRecordBatchPause)
	}

	if total > 0 {
		logger.Infof("cleaned %d notify records created before %d", total, lastWeek)
	}
}

// 每天凌晨1点执行清理任务
func CleanNotifyRecord(ctx *ctx.Context, day int) {
	c := cron.New()
	if day < 1 {
		day = 7
	}

	// 使用cron表达式设置每天凌晨1点执行
	_, err := c.AddFunc("0 1 * * *", func() {
		cleanNotifyRecord(ctx, day)
	})

	if err != nil {
		logger.Errorf("Failed to add clean notify record cron job: %v", err)
		return
	}

	// 启动cron任务
	c.Start()
}
