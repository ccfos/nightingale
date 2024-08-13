package cron

import (
	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/robfig/cron/v3"
)

const (
	limitAlertRecordCountCron = "@every 1h"
)

func InitLimitAlertRecordCountCron(ctx *ctx.Context, redis *storage.Redis) error {
	c := cron.New()

	_, err := c.AddFunc(limitAlertRecordCountCron, func() {
		process.LimitAlertRecordCount(ctx, redis)
	})

	if err != nil {
		return err
	}

	c.Start()

	return nil
}
