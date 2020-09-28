package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/toolkits/pkg/cache"
	"github.com/toolkits/pkg/str"
)

type TaskMeta struct {
	Id        int64     `json:"id"`
	Title     string    `json:"title"`
	Account   string    `json:"account"`
	Batch     int       `json:"batch"`
	Tolerance int       `json:"tolerance"`
	Timeout   int       `json:"timeout"`
	Pause     string    `json:"pause"`
	Script    string    `json:"script"`
	Args      string    `json:"args"`
	Creator   string    `json:"creator"`
	Created   time.Time `xorm:"created" json:"created"`
	Done      bool      `xorm:"-" json:"done"`
}

func TaskMetaGet(where string, args ...interface{}) (*TaskMeta, error) {
	var obj TaskMeta
	has, err := DB["job"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func taskMetaCacheKey(k string) string {
	return fmt.Sprintf("/cache/task/meta/%s", k)
}

// TaskMetaGet 根据ID获取任务元信息，会用到内存缓存
func TaskMetaGetByID(id interface{}) (*TaskMeta, error) {
	var obj TaskMeta
	if err := cache.Get(taskMetaCacheKey(fmt.Sprint(id)), &obj); err == nil {
		return &obj, nil
	}

	has, err := DB["job"].Where("id=?", id).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	cache.Set(taskMetaCacheKey(fmt.Sprint(id)), obj, cache.DEFAULT)

	return &obj, nil
}

func (m *TaskMeta) CleanFields() error {
	if m.Batch < 0 {
		return fmt.Errorf("arg[batch] should be nonnegative")
	}

	if m.Tolerance < 0 {
		return fmt.Errorf("arg[tolerance] should be nonnegative")
	}

	if m.Timeout < 0 {
		return fmt.Errorf("arg[timeout] should be nonnegative")
	}

	if m.Timeout > 3600*24 {
		return fmt.Errorf("arg[timeout] longer than one day")
	}

	if m.Timeout == 0 {
		m.Timeout = 30
	}

	m.Pause = strings.Replace(m.Pause, "，", ",", -1)
	m.Pause = strings.Replace(m.Pause, " ", "", -1)
	m.Args = strings.Replace(m.Args, "，", ",", -1)

	if m.Title == "" {
		return fmt.Errorf("arg[title] is blank")
	}

	if str.Dangerous(m.Title) {
		return fmt.Errorf("arg[title] is dangerous")
	}

	if m.Script == "" {
		return fmt.Errorf("arg[script] is blank")
	}

	if str.Dangerous(m.Args) {
		return fmt.Errorf("arg[args] is dangerous")
	}

	if str.Dangerous(m.Pause) {
		return fmt.Errorf("arg[pause] is dangerous")
	}

	return nil
}

func (m *TaskMeta) HandleFH(fh string) {
	i := strings.Index(m.Title, " FH: ")
	if i > 0 {
		m.Title = m.Title[:i]
	}
	m.Title = m.Title + " FH: " + fh
}

func (m *TaskMeta) Save(hosts []string, action string) error {
	if err := m.CleanFields(); err != nil {
		return err
	}

	m.HandleFH(hosts[0])

	session := DB["job"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Insert(m); err != nil {
		session.Rollback()
		return err
	}

	id := m.Id

	if _, err := session.Insert(&TaskScheduler{Id: id}); err != nil {
		session.Rollback()
		return err
	}

	if _, err := session.Insert(&TaskAction{Id: id, Action: action, Clock: time.Now().Unix()}); err != nil {
		session.Rollback()
		return err
	}

	for _, host := range hosts {
		sql := fmt.Sprintf("INSERT INTO %s(id, host, status) VALUES(%d, '%s', 'waiting')", tht(id), id, host)
		if _, err := session.Exec(sql); err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}

func (m *TaskMeta) Action() (*TaskAction, error) {
	return TaskActionGet("id=?", m.Id)
}

func (m *TaskMeta) Hosts() ([]TaskHost, error) {
	var ret []TaskHost
	err := DB["job"].Table(tht(m.Id)).Where("id=?", m.Id).Cols("id, host, status").OrderBy("ii").Find(&ret)
	return ret, err
}

func (m *TaskMeta) KillHost(host string) error {
	bean, err := TaskHostGet(m.Id, host)
	if err != nil {
		return err
	}

	if bean == nil {
		return fmt.Errorf("no such host")
	}

	if !(bean.Status == "running" || bean.Status == "timeout") {
		return fmt.Errorf("current status is:%s, cannot kill", bean.Status)
	}

	if err := redoHost(m.Id, host, "kill"); err != nil {
		return err
	}

	return statusSet(m.Id, host, "killing")
}

func (m *TaskMeta) IgnoreHost(host string) error {
	return statusSet(m.Id, host, "ignored")
}

func (m *TaskMeta) RedoHost(host string) error {
	bean, err := TaskHostGet(m.Id, host)
	if err != nil {
		return err
	}

	if bean == nil {
		return fmt.Errorf("no such host")
	}

	if err := redoHost(m.Id, host, "start"); err != nil {
		return err
	}

	return statusSet(m.Id, host, "running")
}

func statusSet(id int64, host, status string) error {
	sql := fmt.Sprintf("UPDATE %s SET status=? WHERE id=? and host=?", tht(id))
	_, err := DB["job"].Exec(sql, status, id, host)
	return err
}

func redoHost(id int64, host, action string) error {
	count, err := DB["job"].Where("id=? and host=?", id, host).Count(new(TaskHostDoing))
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	if count == 0 {
		_, err = DB["job"].Exec("INSERT INTO task_host_doing(id,host,clock,action) VALUES(?,?,?,?)", id, host, now, action)
	} else {
		_, err = DB["job"].Exec("UPDATE task_host_doing SET clock=?, action=? WHERE id=? and host=? and action <> ?", now, action, id, host, action)
	}
	return err
}

func (m *TaskMeta) HostStrs() ([]string, error) {
	var ret []string
	err := DB["job"].Table(tht(m.Id)).Where("id=?", m.Id).Select("host").OrderBy("ii").Find(&ret)
	return ret, err
}

func (m *TaskMeta) Stdouts() ([]TaskHost, error) {
	var ret []TaskHost
	err := DB["job"].Table(tht(m.Id)).Where("id=?", m.Id).Cols("id, host, status, stdout").OrderBy("ii").Find(&ret)
	return ret, err
}

func (m *TaskMeta) Stderrs() ([]TaskHost, error) {
	var ret []TaskHost
	err := DB["job"].Table(tht(m.Id)).Where("id=?", m.Id).Cols("id, host, status, stderr").OrderBy("ii").Find(&ret)
	return ret, err
}

func TaskMetaTotal(creator, query string, before time.Time) (int64, error) {
	session := DB["job"].NewSession()
	defer session.Close()

	session = session.Where("created > '" + before.Format("2006-01-02 15:04:05") + "'")

	if creator != "" {
		session = session.Where("creator=?", creator)
	}

	if query != "" {
		// q1 q2 -q3
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			if arr[i] == "" {
				continue
			}
			if strings.HasPrefix(arr[i], "-") {
				q := "%" + arr[i][1:] + "%"
				session = session.Where("title not like ?", q)
			} else {
				q := "%" + arr[i] + "%"
				session = session.Where("title like ?", q)
			}
		}
	}

	return session.Count(new(TaskMeta))
}

func TaskMetaGets(creator, query string, before time.Time, limit, offset int) ([]TaskMeta, error) {
	session := DB["job"].OrderBy("created desc").Limit(limit, offset)

	session = session.Where("created > '" + before.Format("2006-01-02 15:04:05") + "'")

	if creator != "" {
		session = session.Where("creator=?", creator)
	}

	if query != "" {
		// q1 q2 -q3
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			if arr[i] == "" {
				continue
			}
			if strings.HasPrefix(arr[i], "-") {
				q := "%" + arr[i][1:] + "%"
				session = session.Where("title not like ?", q)
			} else {
				q := "%" + arr[i] + "%"
				session = session.Where("title like ?", q)
			}
		}
	}

	var objs []TaskMeta
	err := session.Find(&objs)
	return objs, err
}
