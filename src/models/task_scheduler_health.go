package models

import (
	"time"
)

type TaskSchedulerHealth struct {
	Scheduler string
	Clock     time.Time
}

func TaskSchedulerHeartbeat(endpoint string) error {
	cnt, err := DB["job"].Where("scheduler = ?", endpoint).Count(new(TaskSchedulerHealth))
	if err != nil {
		return err
	}

	if cnt == 0 {
		_, err = DB["job"].Exec("INSERT INTO task_scheduler_health(scheduler, clock) VALUES(?, now())", endpoint)
	} else {
		_, err = DB["job"].Exec("UPDATE task_scheduler_health SET clock = now() WHERE scheduler = ?", endpoint)
	}

	return err
}

func DeadTaskSchedulers() ([]string, error) {
	var arr []string
	err := DB["job"].Table("task_scheduler_health").Where("clock < DATE_SUB(now(),INTERVAL 10 SECOND)").Select("scheduler").Find(&arr)
	return arr, err
}

func DelDeadTaskScheduler(scheduler string) error {
	_, err := DB["job"].Exec("DELETE FROM task_scheduler_health WHERE scheduler = ?", scheduler)
	return err
}
