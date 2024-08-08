package cron

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/tool"
	"github.com/robfig/cron/v3"
)

const (
	limitAlertRecordCountCron = "@every 1m"
)

func InitCron(ctx *ctx.Context) error {
	c := cron.New()
	// 添加一个任务，每分钟执行一次
	_, err := c.AddFunc(limitAlertRecordCountCron, func() {
		tool.LimitAlertRecordCount(ctx)
	})

	if err != nil {
		return err
	}

	// 启动 Cron 调度器
	c.Start()

	return nil
}
