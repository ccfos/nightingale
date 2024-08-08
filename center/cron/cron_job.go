package cron

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/tool"
	"github.com/robfig/cron/v3"
)

const (
	limitAlertRecordCountCron = "@every 1h"
)

func InitCron(ctx *ctx.Context) error {
	c := cron.New()

	_, err := c.AddFunc(limitAlertRecordCountCron, func() {
		tool.LimitAlertRecordCount(ctx)
	})

	if err != nil {
		return err
	}

	c.Start()

	return nil
}
