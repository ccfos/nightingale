package models

import "gorm.io/gorm"

type TaskScheduler struct {
	Id        int64  `gorm:"column:id;primaryKey"`
	Scheduler string `gorm:"column:scheduler;size:128;not null;default:''"`
}

func (TaskScheduler) TableName() string {
	return "task_scheduler"
}

func TasksOfScheduler(scheduler string) ([]int64, error) {
	var ids []int64
	err := IbexDB().Model(&TaskScheduler{}).Where("scheduler = ?", scheduler).Pluck("id", &ids).Error
	return ids, err
}

func TakeOverTask(id int64, pre, current string) (bool, error) {
	ret := IbexDB().Model(&TaskScheduler{}).Where("id = ? and scheduler = ?", id, pre).Update("scheduler", current)
	if ret.Error != nil {
		return false, ret.Error
	}

	return ret.RowsAffected > 0, nil
}

func OrphanTaskIds() ([]int64, error) {
	var ids []int64
	err := IbexDB().Model(&TaskScheduler{}).Where("scheduler = ''").Pluck("id", &ids).Error
	return ids, err
}

func CleanDoneTask(id int64) error {
	return IbexDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Delete(&TaskScheduler{}).Error; err != nil {
			return err
		}

		return tx.Where("id = ?", id).Delete(&TaskAction{}).Error
	})
}
