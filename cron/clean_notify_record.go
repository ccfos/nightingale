package cron

import (
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/robfig/cron/v3"
	"github.com/toolkits/pkg/logger"
)

func cleanNotifyRecord(ctx *ctx.Context, day int) {
	lastWeek := time.Now().Unix() - 86400*int64(day)
	err := models.DB(ctx).Model(&models.NotificaitonRecord{}).Where("created_at < ?", lastWeek).Delete(&models.NotificaitonRecord{}).Error
	if err != nil {
		logger.Errorf("Failed to clean notify record: %v", err)
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
