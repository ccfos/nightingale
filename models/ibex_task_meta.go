package models

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/ibex/server/config"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"
)

type TaskMeta struct {
	Id        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Title     string    `gorm:"column:title;size:255;not null;default:''" json:"title"`
	Account   string    `gorm:"column:account;size:64;not null" json:"account"`
	Batch     int       `gorm:"column:batch;not null;default:0" json:"batch"`
	Tolerance int       `gorm:"column:tolerance;not null;default:0" json:"tolerance"`
	Timeout   int       `gorm:"column:timeout;not null;default:0" json:"timeout"`
	Pause     string    `gorm:"column:pause;size:255;not null;default:''" json:"pause"`
	Script    string    `gorm:"column:script;type:text;not null" json:"script"`
	Args      string    `gorm:"column:args;size:512;not null;default:''" json:"args"`
	Stdin     string    `gorm:"column:stdin;size:1024;not null;default:''" json:"stdin"`
	Creator   string    `gorm:"column:creator;size:64;not null;default:'';index" json:"creator"`
	Created   time.Time `gorm:"column:created;not null;default:CURRENT_TIMESTAMP;type:timestamp;index" json:"created"`
	Done      bool      `json:"done" gorm:"-"`
}

func (TaskMeta) TableName() string {
	return "task_meta"
}

func (taskMeta *TaskMeta) MarshalBinary() ([]byte, error) {
	return json.Marshal(taskMeta)
}

func (taskMeta *TaskMeta) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, taskMeta)
}

func (taskMeta *TaskMeta) Create(ctx *ctx.Context) error {
	if config.C.IsCenter {
		return DB(ctx).Create(taskMeta).Error
	}

	id, err := poster.PostByUrlsWithResp[int64](ctx, "/ibex/v1/task/meta", taskMeta)
	if err == nil {
		taskMeta.Id = id
	}

	return err
}

func taskMetaCacheKey(id int64) string {
	return fmt.Sprintf("task:meta:%d", id)
}

func TaskMetaGet(ctx *ctx.Context, where string, args ...interface{}) (*TaskMeta, error) {
	lst, err := TableRecordGets[[]*TaskMeta](ctx, TaskMeta{}.TableName(), where, args...)
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

// TaskMetaGet 根据ID获取任务元信息，会用到缓存
func TaskMetaGetByID(ctx *ctx.Context, id int64) (*TaskMeta, error) {
	meta, err := TaskMetaCacheGet(ctx, id)
	if err == nil {
		return meta, nil
	}

	meta, err = TaskMetaGet(ctx, "id=?", id)
	if err != nil {
		return nil, err
	}

	if meta == nil {
		return nil, nil
	}

	_, err = ctx.Redis.Set(context.Background(), taskMetaCacheKey(id), meta, storage.DEFAULT).Result()

	return meta, err
}

func TaskMetaCacheGet(ctx *ctx.Context, id int64) (*TaskMeta, error) {
	res := ctx.Redis.Get(context.Background(), taskMetaCacheKey(id))
	meta := new(TaskMeta)
	err := res.Scan(meta)
	return meta, err
}

func (m *TaskMeta) CleanFields() error {
	if m.Batch < 0 {
		return fmt.Errorf("arg(batch) should be nonnegative")
	}

	if m.Tolerance < 0 {
		return fmt.Errorf("arg(tolerance) should be nonnegative")
	}

	if m.Timeout < 0 {
		return fmt.Errorf("arg(timeout) should be nonnegative")
	}

	if m.Timeout > 3600*24*5 {
		return fmt.Errorf("arg(timeout) longer than five days")
	}

	if m.Timeout == 0 {
		m.Timeout = 30
	}

	m.Pause = strings.Replace(m.Pause, "，", ",", -1)
	m.Pause = strings.Replace(m.Pause, " ", "", -1)
	m.Args = strings.Replace(m.Args, "，", ",", -1)

	if m.Title == "" {
		return fmt.Errorf("arg(title) is required")
	}

	if str.Dangerous(m.Title) {
		return fmt.Errorf("arg(title) is dangerous")
	}

	if m.Script == "" {
		return fmt.Errorf("arg(script) is required")
	}

	if str.Dangerous(m.Args) {
		return fmt.Errorf("arg(args) is dangerous")
	}

	if str.Dangerous(m.Pause) {
		return fmt.Errorf("arg(pause) is dangerous")
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

func (taskMeta *TaskMeta) Cache(ctx *ctx.Context, host string) error {

	tx := ctx.Redis.TxPipeline()
	tx.Set(ctx.Ctx, taskMetaCacheKey(taskMeta.Id), taskMeta, storage.DEFAULT)
	tx.HSet(ctx.Ctx, IBEX_HOST_DOING, hostDoingCacheKey(taskMeta.Id, host), &TaskHostDoing{
		Id:     taskMeta.Id,
		Host:   host,
		Clock:  time.Now().Unix(),
		Action: "start",
	})

	_, err := tx.Exec(ctx.Ctx)

	return err
}

func (taskMeta *TaskMeta) Save(ctx *ctx.Context, hosts []string, action string) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(taskMeta).Error; err != nil {
			return err
		}

		id := taskMeta.Id

		if err := tx.Create(&TaskScheduler{Id: id}).Error; err != nil {
			return err
		}

		if err := tx.Create(&TaskAction{Id: id, Action: action, Clock: time.Now().Unix()}).Error; err != nil {
			return err
		}

		for i := 0; i < len(hosts); i++ {
			host := strings.TrimSpace(hosts[i])
			if host == "" {
				continue
			}

			err := tx.Exec("INSERT INTO "+tht(id)+" (id, host, status) VALUES (?, ?, ?)", id, host, "waiting").Error
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (m *TaskMeta) Action(ctx *ctx.Context) (*TaskAction, error) {
	return TaskActionGet(ctx, "id=?", m.Id)
}

func (m *TaskMeta) Hosts(ctx *ctx.Context) ([]TaskHost, error) {
	var ret []TaskHost
	err := DB(ctx).Table(tht(m.Id)).Where("id=?", m.Id).Select("id", "host", "status").Order("ii").Find(&ret).Error
	return ret, err
}

func (m *TaskMeta) KillHost(ctx *ctx.Context, host string) error {
	bean, err := TaskHostGet(ctx, m.Id, host)
	if err != nil {
		return err
	}

	if bean == nil {
		return fmt.Errorf("no such host")
	}

	if !(bean.Status == "running" || bean.Status == "timeout") {
		return fmt.Errorf("current status cannot kill")
	}

	if err := redoHost(ctx, m.Id, host, "kill"); err != nil {
		return err
	}

	return statusSet(ctx, m.Id, host, "killing")
}

func (m *TaskMeta) IgnoreHost(ctx *ctx.Context, host string) error {
	return statusSet(ctx, m.Id, host, "ignored")
}

func (m *TaskMeta) RedoHost(ctx *ctx.Context, host string) error {
	bean, err := TaskHostGet(ctx, m.Id, host)
	if err != nil {
		return err
	}

	if bean == nil {
		return fmt.Errorf("no such host")
	}

	if err := redoHost(ctx, m.Id, host, "start"); err != nil {
		return err
	}

	return statusSet(ctx, m.Id, host, "running")
}

func statusSet(ctx *ctx.Context, id int64, host, status string) error {
	return DB(ctx).Table(tht(id)).Where("id=? and host=?", id, host).Update("status", status).Error
}

func redoHost(ctx *ctx.Context, id int64, host, action string) error {
	count, err := IbexCount(DB(ctx).Model(&TaskHostDoing{}).Where("id=? and host=?", id, host))
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	if count == 0 {
		err = DB(ctx).Table("task_host_doing").Create(map[string]interface{}{
			"id":     id,
			"host":   host,
			"clock":  now,
			"action": action,
		}).Error
	} else {
		err = DB(ctx).Table("task_host_doing").Where("id=? and host=? and action <> ?", id, host, action).Updates(map[string]interface{}{
			"clock":  now,
			"action": action,
		}).Error
	}
	return err
}

func (m *TaskMeta) HostStrs(ctx *ctx.Context) ([]string, error) {
	var ret []string
	err := DB(ctx).Table(tht(m.Id)).Where("id=?", m.Id).Order("ii").Pluck("host", &ret).Error
	return ret, err
}

func (m *TaskMeta) Stdouts(ctx *ctx.Context) ([]TaskHost, error) {
	var ret []TaskHost
	err := DB(ctx).Table(tht(m.Id)).Where("id=?", m.Id).Select("id", "host", "status", "stdout").Order("ii").Find(&ret).Error
	return ret, err
}

func (m *TaskMeta) Stderrs(ctx *ctx.Context) ([]TaskHost, error) {
	var ret []TaskHost
	err := DB(ctx).Table(tht(m.Id)).Where("id=?", m.Id).Select("id", "host", "status", "stderr").Order("ii").Find(&ret).Error
	return ret, err
}

func TaskMetaTotal(ctx *ctx.Context, creator, query string, before time.Time) (int64, error) {
	session := DB(ctx).Model(&TaskMeta{})

	session = session.Where("created > '" + before.Format("2006-01-02 15:04:05") + "'")

	if creator != "" {
		session = session.Where("creator = ?", creator)
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

	return IbexCount(session)
}

func TaskMetaGets(ctx *ctx.Context, creator, query string, before time.Time, limit, offset int) ([]TaskMeta, error) {
	session := DB(ctx).Model(&TaskMeta{}).Order("created desc").Limit(limit).Offset(offset)

	session = session.Where("created > '" + before.Format("2006-01-02 15:04:05") + "'")

	if creator != "" {
		session = session.Where("creator = ?", creator)
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
	err := session.Find(&objs).Error
	return objs, err
}
