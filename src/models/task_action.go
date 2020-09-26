package models

import (
	"fmt"
	"time"
)

type TaskAction struct {
	Id     int64
	Action string
	Clock  int64
}

func TaskActionGet(where string, args ...interface{}) (*TaskAction, error) {
	var obj TaskAction
	has, err := DB["job"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func TaskActionExistsIds(ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return ids, nil
	}

	var ret []int64
	err := DB["job"].Table("task_action").In("id", ids).Select("id").Find(&ret)
	return ret, err
}

func CancelWaitingHosts(id int64) error {
	sql := fmt.Sprintf("UPDATE %s SET status = 'cancelled' WHERE id = %d and status = 'waiting'", tht(id), id)
	_, err := DB["job"].Exec(sql)
	return err
}

func StartTask(id int64) error {
	_, err := DB["job"].Exec("UPDATE task_scheduler SET scheduler='' WHERE id=?", id)
	return err
}

func CancelTask(id int64) error {
	return CancelWaitingHosts(id)
}

func KillTask(id int64) error {
	if err := CancelWaitingHosts(id); err != nil {
		return err
	}

	now := time.Now().Unix()

	session := DB["job"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec("UPDATE task_host_doing SET clock=?, action='kill' WHERE id=? and action <> 'kill'", now, id); err != nil {
		session.Rollback()
		return err
	}

	sql := fmt.Sprintf("UPDATE %s SET status = 'killing' WHERE id=%d and status='running'", tht(id), id)

	if _, err := session.Exec(sql); err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

func (a *TaskAction) Update(action string) error {
	if !(action == "start" || action == "cancel" || action == "kill" || action == "pause") {
		return fmt.Errorf("action invalid")
	}

	a.Action = action
	a.Clock = time.Now().Unix()
	_, err := DB["job"].Where("id=?", a.Id).Cols("action", "clock").Update(a)
	if err != nil {
		return err
	}

	if action == "start" {
		return StartTask(a.Id)
	}

	if action == "cancel" {
		return CancelTask(a.Id)
	}

	if action == "kill" {
		return KillTask(a.Id)
	}

	return nil
}

// LongTaskIds two weeks ago
func LongTaskIds() ([]int64, error) {
	clock := time.Now().Unix() - 604800*2
	var ids []int64
	err := DB["job"].Table("task_action").Where("clock < ?", clock).Select("id").Find(&ids)
	return ids, err
}
