package models

import (
	"fmt"
	"time"
)

type TaskHost struct {
	Id     int64  `json:"id"`
	Host   string `json:"host"`
	Status string `json:"status"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

func tht(id int64) string {
	return fmt.Sprintf("task_host_%d", id%100)
}

func TaskHostGet(id int64, host string) (*TaskHost, error) {
	var obj TaskHost
	has, err := DB["job"].Table(tht(id)).Where("id=? and host=?", id, host).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func MarkDoneStatus(id, clock int64, host, status, stdout, stderr string) error {
	count, err := DoingHostCount("id=? and host=? and clock=?", id, host, clock)
	if err != nil {
		return err
	}

	sql := fmt.Sprintf("UPDATE %s SET status=?, stdout=?, stderr=? WHERE id=? and host=?", tht(id))

	if count == 0 {
		// 如果是timeout了，后来任务执行完成之后，结果又上来了，stdout和stderr最好还是存库，让用户看到
		count, err = DB["job"].Table(tht(id)).Where("id=? and host=? and status=?", id, host, "timeout").Count()
		if err != nil {
			return err
		}

		if count == 1 {
			_, err = DB["job"].Exec(sql, status, stdout, stderr, id, host)
			return err
		}

		return nil
	}

	session := DB["job"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec(sql, status, stdout, stderr, id, host); err != nil {
		session.Rollback()
		return err
	}

	if _, err := session.Exec("DELETE FROM task_host_doing WHERE id=? and host=?", id, host); err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

func WaitingHostList(id int64, limit ...int) ([]TaskHost, error) {
	var hosts []TaskHost
	session := DB["job"].Table(tht(id)).Where("id = ? and status = 'waiting'", id).OrderBy("ii")
	if len(limit) > 0 {
		session = session.Limit(limit[0])
	}
	err := session.Find(&hosts)
	return hosts, err
}

func WaitingHostCount(id int64) (int64, error) {
	return DB["job"].Table(tht(id)).Where("id=? and status='waiting'", id).Count()
}

func UnexpectedHostCount(id int64) (int64, error) {
	return DB["job"].Table(tht(id)).Where("id=? and status in ('failed', 'timeout', 'killfailed')", id).Count()
}

func IngStatusHostCount(id int64) (int64, error) {
	return DB["job"].Table(tht(id)).Where("id=? and status in ('waiting', 'running', 'killing')", id).Count()
}

func RunWaitingHosts(hosts []TaskHost) error {
	count := len(hosts)
	if count == 0 {
		return nil
	}

	now := time.Now().Unix()

	session := DB["job"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	for i := 0; i < count; i++ {
		sql := fmt.Sprintf("UPDATE %s SET status=? WHERE id=? and host=?", tht(hosts[i].Id))
		if _, err := session.Exec(sql, "running", hosts[i].Id, hosts[i].Host); err != nil {
			session.Rollback()
			return err
		}

		if _, err := session.Insert(&TaskHostDoing{Id: hosts[i].Id, Host: hosts[i].Host, Clock: now, Action: "start"}); err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}

func TaskHostStatus(id int64) ([]TaskHost, error) {
	var ret []TaskHost
	err := DB["job"].Table(tht(id)).Cols("id", "host", "status").Where("id=?", id).OrderBy("ii").Find(&ret)
	return ret, err
}

func TaskHostGets(id int64) ([]TaskHost, error) {
	var ret []TaskHost
	err := DB["job"].Table(tht(id)).Where("id=?", id).OrderBy("ii").Find(&ret)
	return ret, err
}
