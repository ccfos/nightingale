package models

import (
	"time"
)

type TaskSchedulerHealth struct {
	Scheduler string `gorm:"column:scheduler;uniqueIndex;size:128;not null"`
	Clock     int64  `gorm:"column:clock;not null;index"`
}

func (TaskSchedulerHealth) TableName() string {
	return "task_scheduler_health"
}

func TaskSchedulerHeartbeat(scheduler string) error {
	var cnt int64
	err := IbexDB().Model(&TaskSchedulerHealth{}).Where("scheduler = ?", scheduler).Count(&cnt).Error
	if err != nil {
		return err
	}

	if cnt == 0 {
		ret := IbexDB().Create(&TaskSchedulerHealth{
			Scheduler: scheduler,
			Clock:     time.Now().Unix(),
		})
		err = ret.Error
	} else {
		err = IbexDB().Model(&TaskSchedulerHealth{}).Where("scheduler = ?", scheduler).Update("clock", time.Now().Unix()).Error
	}

	return err
}

func DeadTaskSchedulers() ([]string, error) {
	clock := time.Now().Unix() - 10
	var arr []string
	err := IbexDB().Model(&TaskSchedulerHealth{}).Where("clock < ?", clock).Pluck("scheduler", &arr).Error
	return arr, err
}

func DelDeadTaskScheduler(scheduler string) error {
	return IbexDB().Where("scheduler = ?", scheduler).Delete(&TaskSchedulerHealth{}).Error
}
