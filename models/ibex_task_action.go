package models

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"gorm.io/gorm"
)

type TaskAction struct {
	Id     int64  `gorm:"column:id;primaryKey"`
	Action string `gorm:"column:action;size:32;not null"`
	Clock  int64  `gorm:"column:clock;not null;default:0"`
}

func (TaskAction) TableName() string {
	return "task_action"
}

func TaskActionGet(ctx *ctx.Context, where string, args ...interface{}) (*TaskAction, error) {
	var obj TaskAction
	ret := DB(ctx).Where(where, args...).Find(&obj)
	if ret.Error != nil {
		return nil, ret.Error
	}

	if ret.RowsAffected == 0 {
		return nil, nil
	}

	return &obj, nil
}

func TaskActionExistsIds(ctx *ctx.Context, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return ids, nil
	}

	var ret []int64
	err := DB(ctx).Model(&TaskAction{}).Where("id in ?", ids).Pluck("id", &ret).Error
	return ret, err
}

func CancelWaitingHosts(ctx *ctx.Context, id int64) error {
	return DB(ctx).Table(tht(id)).Where("id = ? and status = ?", id, "waiting").Update("status", "cancelled").Error
}

func StartTask(ctx *ctx.Context, id int64) error {
	return DB(ctx).Model(&TaskScheduler{}).Where("id = ?", id).Update("scheduler", "").Error
}

func CancelTask(ctx *ctx.Context, id int64) error {
	return CancelWaitingHosts(ctx, id)
}

func KillTask(ctx *ctx.Context, id int64) error {
	if err := CancelWaitingHosts(ctx, id); err != nil {
		return err
	}

	now := time.Now().Unix()

	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&TaskHostDoing{}).Where("id = ? and action <> ?", id, "kill").Updates(map[string]interface{}{
			"clock":  now,
			"action": "kill",
		}).Error
		if err != nil {
			return err
		}

		return tx.Table(tht(id)).Where("id = ? and status = ?", id, "running").Update("status", "killing").Error
	})
}

func (a *TaskAction) Update(ctx *ctx.Context, action string) error {
	if !(action == "start" || action == "cancel" || action == "kill" || action == "pause") {
		return fmt.Errorf("action invalid")
	}

	err := DB(ctx).Model(a).Updates(map[string]interface{}{
		"action": action,
		"clock":  time.Now().Unix(),
	}).Error
	if err != nil {
		return err
	}

	if action == "start" {
		return StartTask(ctx, a.Id)
	}

	if action == "cancel" {
		return CancelTask(ctx, a.Id)
	}

	if action == "kill" {
		return KillTask(ctx, a.Id)
	}

	return nil
}

// LongTaskIds two weeks ago
func LongTaskIds(ctx *ctx.Context) ([]int64, error) {
	clock := time.Now().Unix() - 604800*2
	var ids []int64
	err := DB(ctx).Model(&TaskAction{}).Where("clock < ?", clock).Pluck("id", &ids).Error
	return ids, err
}
