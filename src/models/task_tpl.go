package models

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/toolkits/pkg/slice"
	"github.com/toolkits/pkg/str"
)

type TaskTpl struct {
	Id          int64     `json:"id"`
	NodeId      int64     `json:"node_id"`
	Title       string    `json:"title"`
	Batch       int       `json:"batch"`
	Tolerance   int       `json:"tolerance"`
	Timeout     int       `json:"timeout"`
	Pause       string    `json:"pause"`
	Script      string    `json:"script"`
	Args        string    `json:"args"`
	Tags        string    `json:"tags"`
	Account     string    `json:"account"`
	Creator     string    `json:"creator"`
	LastUpdated time.Time `xorm:"<-" json:"last_updated"`
}

func TaskTplTotal(nodeId int64, query string) (int64, error) {
	session := DB["job"].Where("node_id = ?", nodeId)
	if query == "" {
		return session.Count(new(TaskTpl))
	}

	arr := strings.Fields(query)
	for i := 0; i < len(arr); i++ {
		arg := "%" + arr[i] + "%"
		session = session.And("title like ? or tags like ?", arg, arg)
	}

	return session.Count(new(TaskTpl))
}

func TaskTplGets(nodeId int64, query string, limit, offset int) ([]TaskTpl, error) {
	session := DB["job"].Where("node_id = ?", nodeId).OrderBy("title").Limit(limit, offset)

	var tpls []TaskTpl
	if query == "" {
		err := session.Find(&tpls)
		return tpls, err
	}

	arr := strings.Fields(query)
	for i := 0; i < len(arr); i++ {
		arg := "%" + arr[i] + "%"
		session = session.And("title like ? or tags like ?", arg, arg)
	}

	err := session.Find(&tpls)
	return tpls, err
}

func TaskTplGet(where string, args ...interface{}) (*TaskTpl, error) {
	var obj TaskTpl
	has, err := DB["job"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func (t *TaskTpl) CleanFields() error {
	if t.Batch < 0 {
		return fmt.Errorf("arg[batch] should be nonnegative")
	}

	if t.Tolerance < 0 {
		return fmt.Errorf("arg[tolerance] should be nonnegative")
	}

	if t.Timeout < 0 {
		return fmt.Errorf("arg[timeout] should be nonnegative")
	}

	if t.Timeout == 0 {
		t.Timeout = 30
	}

	if t.Timeout > 3600*24 {
		return fmt.Errorf("arg[timeout] longer than one day")
	}

	t.Pause = strings.Replace(t.Pause, "，", ",", -1)
	t.Pause = strings.Replace(t.Pause, " ", "", -1)
	t.Args = strings.Replace(t.Args, "，", ",", -1)
	t.Tags = strings.Replace(t.Tags, "，", ",", -1)
	t.Tags = strings.Replace(t.Tags, " ", "", -1)

	if t.Title == "" {
		return fmt.Errorf("arg[title] is blank")
	}

	if str.Dangerous(t.Title) {
		return fmt.Errorf("arg[title] is dangerous")
	}

	if t.Script == "" {
		return fmt.Errorf("arg[script] is blank")
	}

	if str.Dangerous(t.Args) {
		return fmt.Errorf("arg[args] is dangerous")
	}

	if str.Dangerous(t.Pause) {
		return fmt.Errorf("arg[pause] is dangerous")
	}

	if str.Dangerous(t.Tags) {
		return fmt.Errorf("arg[tags] is dangerous")
	}

	return nil
}

func (t *TaskTpl) Save(hosts []string) error {
	if err := t.CleanFields(); err != nil {
		return err
	}

	session := DB["job"].NewSession()
	defer session.Close()

	cnt, err := session.Where("node_id=? and title=?", t.NodeId, t.Title).Count(new(TaskTpl))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return fmt.Errorf("title[%s] already exists", t.Title)
	}

	if err = session.Begin(); err != nil {
		return err
	}

	if _, err = session.Insert(t); err != nil {
		session.Rollback()
		return err
	}

	for i := 0; i < len(hosts); i++ {
		host := strings.TrimSpace(hosts[i])
		if host == "" {
			continue
		}

		if _, err = session.Exec("INSERT INTO task_tpl_host(id, host) VALUES(?, ?)", t.Id, host); err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}

func (t *TaskTpl) Hosts() ([]string, error) {
	var arr []string
	err := DB["job"].Table("task_tpl_host").Where("id=?", t.Id).Select("host").OrderBy("ii").Find(&arr)
	return arr, err
}

func (t *TaskTpl) Update(hosts []string) error {
	if err := t.CleanFields(); err != nil {
		return err
	}

	session := DB["job"].NewSession()
	defer session.Close()

	cnt, err := session.Where("node_id=? and title=? and id <> ?", t.NodeId, t.Title, t.Id).Count(new(TaskTpl))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return fmt.Errorf("title[%s] already exists", t.Title)
	}

	if err = session.Begin(); err != nil {
		return err
	}

	if _, err = session.Where("id=?", t.Id).Cols("title", "batch", "tolerance", "timeout", "pause", "script", "args", "tags", "account").Update(t); err != nil {
		session.Rollback()
		return err
	}

	if _, err = session.Exec("DELETE FROM task_tpl_host WHERE id=?", t.Id); err != nil {
		session.Rollback()
		return err
	}

	for i := 0; i < len(hosts); i++ {
		host := strings.TrimSpace(hosts[i])
		if host == "" {
			continue
		}

		if _, err = session.Exec("INSERT INTO task_tpl_host(id, host) VALUES(?, ?)", t.Id, host); err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}

func (t *TaskTpl) Del() error {
	session := DB["job"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM task_tpl_host WHERE id=?", t.Id); err != nil {
		session.Rollback()
		return err
	}

	if _, err := session.Exec("DELETE FROM task_tpl WHERE id=?", t.Id); err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

func (t *TaskTpl) BindTags(tags []string) error {
	if t.Tags == "" {
		t.Tags = strings.Join(tags, ",")
	} else {
		arr := strings.Split(t.Tags, ",")
		lst := slice.UniqueString(append(arr, tags...))
		sort.Strings(lst)
		t.Tags = strings.Join(lst, ",")
	}

	_, err := DB["job"].Where("id=?", t.Id).Cols("tags").Update(t)
	return err
}

func (t *TaskTpl) UnbindTags(tags []string) error {
	if t.Tags == "" {
		return nil
	}

	arr := strings.Split(t.Tags, ",")
	lst := slice.SubString(arr, tags)
	sort.Strings(lst)
	t.Tags = strings.Join(lst, ",")
	_, err := DB["job"].Where("id=?", t.Id).Cols("tags").Update(t)
	return err
}

func (t *TaskTpl) UpdateGroup(nodeId int64) error {
	t.NodeId = nodeId
	_, err := DB["job"].Where("id=?", t.Id).Cols("node_id").Update(t)
	return err
}
