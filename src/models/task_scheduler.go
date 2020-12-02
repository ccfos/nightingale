package models

type TaskScheduler struct {
	Id        int64
	Scheduler string
}

func TasksOfScheduler(scheduler string) ([]int64, error) {
	var ids []int64
	err := DB["job"].Table("task_scheduler").Where("scheduler=?", scheduler).Select("id").Find(&ids)
	return ids, err
}

func TakeOverTask(id int64, pre, current string) (bool, error) {
	ret, err := DB["job"].Exec("UPDATE task_scheduler SET scheduler=? WHERE id = ? and scheduler = ?", current, id, pre)
	if err != nil {
		return false, err
	}

	affected, err := ret.RowsAffected()
	if err != nil {
		return false, err
	}

	return affected > 0, nil
}

func OrphanTaskIds() ([]int64, error) {
	var ids []int64
	err := DB["job"].Table("task_scheduler").Where("scheduler = ''").Select("id").Find(&ids)
	return ids, err
}

func CleanDoneTask(id int64) error {
	session := DB["job"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM task_scheduler WHERE id = ?", id); err != nil {
		session.Rollback()
		return err
	}

	if _, err := session.Exec("DELETE FROM task_action WHERE id = ?", id); err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}
