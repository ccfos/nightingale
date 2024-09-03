package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"gorm.io/gorm"
)

type TaskScheduler struct {
	Id        int64  `gorm:"column:id;primaryKey"`
	Scheduler string `gorm:"column:scheduler;size:128;not null;default:''"`
}

func (TaskScheduler) TableName() string {
	return "task_scheduler"
}

func TasksOfScheduler(ctx *ctx.Context, scheduler string) ([]int64, error) {
	var ids []int64
	err := DB(ctx).Model(&TaskScheduler{}).Where("scheduler = ?", scheduler).Pluck("id", &ids).Error
	return ids, err
}

func TakeOverTask(ctx *ctx.Context, id int64, pre, current string) (bool, error) {
	ret := DB(ctx).Model(&TaskScheduler{}).Where("id = ? and scheduler = ?", id, pre).Update("scheduler", current)
	if ret.Error != nil {
		return false, ret.Error
	}

	return ret.RowsAffected > 0, nil
}

func OrphanTaskIds(ctx *ctx.Context) ([]int64, error) {
	var ids []int64
	err := DB(ctx).Model(&TaskScheduler{}).Where("scheduler = ''").Pluck("id", &ids).Error
	return ids, err
}

func CleanDoneTask(ctx *ctx.Context, id int64) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Delete(&TaskScheduler{}).Error; err != nil {
			return err
		}

		return tx.Where("id = ?", id).Delete(&TaskAction{}).Error
	})
}
