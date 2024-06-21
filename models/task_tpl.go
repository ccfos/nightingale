package models

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"
)

type TaskTpl struct {
	Id        int64    `json:"id" gorm:"primaryKey"`
	GroupId   int64    `json:"group_id"`
	Title     string   `json:"title"`
	Batch     int      `json:"batch"`
	Tolerance int      `json:"tolerance"`
	Timeout   int      `json:"timeout"`
	Pause     string   `json:"pause"`
	Script    string   `json:"script"`
	Args      string   `json:"args"`
	Tags      string   `json:"-"`
	TagsJSON  []string `json:"tags" gorm:"-"`
	Account   string   `json:"account"`
	CreateAt  int64    `json:"create_at"`
	CreateBy  string   `json:"create_by"`
	UpdateAt  int64    `json:"update_at"`
	UpdateBy  string   `json:"update_by"`
}

func (t *TaskTpl) TableName() string {
	return "task_tpl"
}

func TaskTplTotal(ctx *ctx.Context, bgids []int64, query string) (int64, error) {
	session := DB(ctx).Model(&TaskTpl{})
	if len(bgids) > 0 {
		session = session.Where("group_id in (?)", bgids)
	}

	if query == "" {
		return Count(session)
	}

	arr := strings.Fields(query)
	for i := 0; i < len(arr); i++ {
		arg := "%" + arr[i] + "%"
		session = session.Where("title like ? or tags like ?", arg, arg)
	}

	return Count(session)
}

func TaskTplStatistics(ctx *ctx.Context) (*Statistics, error) {
	if !ctx.IsCenter {
		return poster.GetByUrls[*Statistics](ctx, "/v1/n9e/task-tpl/statistics")
	}

	session := DB(ctx).Model(&TaskTpl{}).Select("count(*) as total", "max(update_at) as last_updated")

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func TaskTplGetAll(ctx *ctx.Context) ([]*TaskTpl, error) {
	if !ctx.IsCenter {
		return poster.GetByUrls[[]*TaskTpl](ctx, "/v1/n9e/task-tpls")
	}

	lst := make([]*TaskTpl, 0)
	err := DB(ctx).Find(&lst).Error
	return lst, err

}

func TaskTplGets(ctx *ctx.Context, bgids []int64, query string, limit, offset int) ([]TaskTpl, error) {
	session := DB(ctx).Order("title").Limit(limit).Offset(offset)
	if len(bgids) > 0 {
		session = session.Where("group_id in (?)", bgids)
	}

	var tpls []TaskTpl
	if query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			arg := "%" + arr[i] + "%"
			session = session.Where("title like ? or tags like ?", arg, arg)
		}
	}

	err := session.Find(&tpls).Error
	if err == nil {
		for i := 0; i < len(tpls); i++ {
			tpls[i].TagsJSON = strings.Fields(tpls[i].Tags)
		}
	}

	return tpls, err
}

func TaskTplGetById(ctx *ctx.Context, id int64) (*TaskTpl, error) {
	if !ctx.IsCenter {
		tpl, err := poster.GetByUrls[*TaskTpl](ctx, "/v1/n9e/task-tpl/"+strconv.FormatInt(id, 10))
		return tpl, err
	}

	return TaskTplGet(ctx, "id = ?", id)
}

func TaskTplGet(ctx *ctx.Context, where string, args ...interface{}) (*TaskTpl, error) {
	var arr []*TaskTpl
	err := DB(ctx).Where(where, args...).Find(&arr).Error
	if err != nil {
		return nil, err
	}

	if len(arr) == 0 {
		return nil, nil
	}

	arr[0].TagsJSON = strings.Fields(arr[0].Tags)

	return arr[0], nil
}

func (t *TaskTpl) CleanFields() error {
	if t.Batch < 0 {
		return errors.New("arg(batch) should be nonnegative")
	}

	if t.Tolerance < 0 {
		return errors.New("arg(tolerance) should be nonnegative")
	}

	if t.Timeout < 0 {
		return errors.New("arg(timeout) should be nonnegative")
	}

	if t.Timeout == 0 {
		t.Timeout = 30
	}

	if t.Timeout > 3600*24*5 {
		return errors.New("arg(timeout) longer than five days")
	}

	t.Pause = strings.Replace(t.Pause, "，", ",", -1)
	t.Pause = strings.Replace(t.Pause, " ", "", -1)
	t.Args = strings.Replace(t.Args, "，", ",", -1)
	t.Tags = strings.Replace(t.Tags, "，", ",", -1)

	if t.Title == "" {
		return errors.New("arg(title) is required")
	}

	if str.Dangerous(t.Title) {
		return errors.New("arg(title) is dangerous")
	}

	if t.Script == "" {
		return errors.New("arg(script) is required")
	}
	t.Script = strings.Replace(t.Script, "\r\n", "\n", -1)

	if str.Dangerous(t.Args) {
		return errors.New("arg(args) is dangerous")
	}

	if str.Dangerous(t.Pause) {
		return errors.New("arg(pause) is dangerous")
	}

	if str.Dangerous(t.Tags) {
		return errors.New("arg(tags) is dangerous")
	}

	return nil
}

type TaskTplHost struct {
	Id   int64  `json:"id"`
	Host string `json:"host"`
}

func (t *TaskTpl) Save(ctx *ctx.Context, hosts []string) error {
	if err := t.CleanFields(); err != nil {
		return err
	}

	cnt, err := Count(DB(ctx).Model(&TaskTpl{}).Where("group_id=? and title=?", t.GroupId, t.Title))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return fmt.Errorf("task template already exists")
	}

	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(t).Error; err != nil {
			return err
		}

		for i := 0; i < len(hosts); i++ {
			host := strings.TrimSpace(hosts[i])
			if host == "" {
				continue
			}

			taskTplHost := TaskTplHost{
				Id:   t.Id,
				Host: host,
			}

			err := tx.Table("task_tpl_host").Create(&taskTplHost).Error

			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (t *TaskTpl) Hosts(ctx *ctx.Context) ([]string, error) {
	var arr []string
	err := DB(ctx).Table("task_tpl_host").Where("id=?", t.Id).Order("ii").Pluck("host", &arr).Error
	return arr, err
}

func (t *TaskTpl) Update(ctx *ctx.Context, hosts []string) error {
	if err := t.CleanFields(); err != nil {
		return err
	}

	cnt, err := Count(DB(ctx).Model(&TaskTpl{}).Where("group_id=? and title=? and id <> ?", t.GroupId, t.Title, t.Id))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return fmt.Errorf("task template already exists")
	}

	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Model(t).Updates(map[string]interface{}{
			"title":     t.Title,
			"batch":     t.Batch,
			"tolerance": t.Tolerance,
			"timeout":   t.Timeout,
			"pause":     t.Pause,
			"script":    t.Script,
			"args":      t.Args,
			"tags":      t.Tags,
			"account":   t.Account,
			"update_by": t.UpdateBy,
			"update_at": t.UpdateAt,
		}).Error

		if err != nil {
			return err
		}

		if err = tx.Exec("DELETE FROM task_tpl_host WHERE id = ?", t.Id).Error; err != nil {
			return err
		}

		for i := 0; i < len(hosts); i++ {
			host := strings.TrimSpace(hosts[i])
			if host == "" {
				continue
			}

			err := tx.Table("task_tpl_host").Create(map[string]interface{}{
				"id":   t.Id,
				"host": host,
			}).Error

			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (t *TaskTpl) Del(ctx *ctx.Context) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM task_tpl_host WHERE id=?", t.Id).Error; err != nil {
			return err
		}

		if err := tx.Delete(t).Error; err != nil {
			return err
		}

		return nil
	})
}

func (t *TaskTpl) AddTags(ctx *ctx.Context, tags []string, updateBy string) error {
	for i := 0; i < len(tags); i++ {
		if -1 == strings.Index(t.Tags, tags[i]+" ") {
			t.Tags += tags[i] + " "
		}
	}

	arr := strings.Fields(t.Tags)
	sort.Strings(arr)

	return DB(ctx).Model(t).Updates(map[string]interface{}{
		"tags":      strings.Join(arr, " ") + " ",
		"update_by": updateBy,
		"update_at": time.Now().Unix(),
	}).Error
}

func (t *TaskTpl) DelTags(ctx *ctx.Context, tags []string, updateBy string) error {
	for i := 0; i < len(tags); i++ {
		t.Tags = strings.ReplaceAll(t.Tags, tags[i]+" ", "")
	}

	return DB(ctx).Model(t).Updates(map[string]interface{}{
		"tags":      t.Tags,
		"update_by": updateBy,
		"update_at": time.Now().Unix(),
	}).Error
}

func (t *TaskTpl) UpdateGroup(ctx *ctx.Context, groupId int64, updateBy string) error {
	return DB(ctx).Model(t).Updates(map[string]interface{}{
		"group_id":  groupId,
		"update_by": updateBy,
		"update_at": time.Now().Unix(),
	}).Error
}

type TaskForm struct {
	Title          string   `json:"title"`
	Account        string   `json:"account"`
	Batch          int      `json:"batch"`
	Tolerance      int      `json:"tolerance"`
	Timeout        int      `json:"timeout"`
	Pause          string   `json:"pause"`
	Script         string   `json:"script"`
	Args           string   `json:"args"`
	Stdin          string   `json:"stdin"`
	Action         string   `json:"action"`
	Creator        string   `json:"creator"`
	Hosts          []string `json:"hosts"`
	AlertTriggered bool     `json:"alert_triggered"`
}

func (f *TaskForm) Verify() error {
	if f.Batch < 0 {
		return fmt.Errorf("arg(batch) should be nonnegative")
	}

	if f.Tolerance < 0 {
		return fmt.Errorf("arg(tolerance) should be nonnegative")
	}

	if f.Timeout < 0 {
		return fmt.Errorf("arg(timeout) should be nonnegative")
	}

	if f.Timeout > 3600*24*5 {
		return fmt.Errorf("arg(timeout) longer than five days")
	}

	if f.Timeout == 0 {
		f.Timeout = 30
	}

	f.Pause = strings.Replace(f.Pause, "，", ",", -1)
	f.Pause = strings.Replace(f.Pause, " ", "", -1)
	f.Args = strings.Replace(f.Args, "，", ",", -1)

	if f.Title == "" {
		return fmt.Errorf("arg(title) is required")
	}

	if str.Dangerous(f.Title) {
		return fmt.Errorf("arg(title) is dangerous")
	}

	if f.Script == "" {
		return fmt.Errorf("arg(script) is required")
	}
	f.Script = strings.Replace(f.Script, "\r\n", "\n", -1)

	if str.Dangerous(f.Args) {
		return fmt.Errorf("arg(args) is dangerous")
	}

	if str.Dangerous(f.Pause) {
		return fmt.Errorf("arg(pause) is dangerous")
	}

	if len(f.Hosts) == 0 {
		return fmt.Errorf("arg(hosts) empty")
	}

	if f.Action != "start" && f.Action != "pause" {
		return fmt.Errorf("arg(action) invalid")
	}

	return nil
}

func (f *TaskForm) HandleFH(fh string) {
	i := strings.Index(f.Title, " FH: ")
	if i > 0 {
		f.Title = f.Title[:i]
	}
	f.Title = f.Title + " FH: " + fh
}
