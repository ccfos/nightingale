package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"time"
)

type TaskSchedulerHealth struct {
	Scheduler string `gorm:"column:scheduler;uniqueIndex;size:128;not null"`
	Clock     int64  `gorm:"column:clock;not null;index"`
}

func (TaskSchedulerHealth) TableName() string {
	return "task_scheduler_health"
}

func TaskSchedulerHeartbeat(ctx *ctx.Context, scheduler string) error {
	var cnt int64
	err := DB(ctx).Model(&TaskSchedulerHealth{}).Where("scheduler = ?", scheduler).Count(&cnt).Error
	if err != nil {
		return err
	}

	if cnt == 0 {
		ret := DB(ctx).Create(&TaskSchedulerHealth{
			Scheduler: scheduler,
			Clock:     time.Now().Unix(),
		})
		err = ret.Error
	} else {
		err = DB(ctx).Model(&TaskSchedulerHealth{}).Where("scheduler = ?", scheduler).Update("clock", time.Now().Unix()).Error
	}

	return err
}

func DeadTaskSchedulers(ctx *ctx.Context) ([]string, error) {
	clock := time.Now().Unix() - 10
	var arr []string
	err := DB(ctx).Model(&TaskSchedulerHealth{}).Where("clock < ?", clock).Pluck("scheduler", &arr).Error
	return arr, err
}

func DelDeadTaskScheduler(ctx *ctx.Context, scheduler string) error {
	return DB(ctx).Where("scheduler = ?", scheduler).Delete(&TaskSchedulerHealth{}).Error
}
