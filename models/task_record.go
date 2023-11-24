package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
)

type TaskRecord struct {
	Id           int64  `json:"id" gorm:"primaryKey"`
	EventId      int64  `json:"event_id"`
	GroupId      int64  `json:"group_id"`
	IbexAddress  string `json:"ibex_address"`
	IbexAuthUser string `json:"ibex_auth_user"`
	IbexAuthPass string `json:"ibex_auth_pass"`
	Title        string `json:"title"`
	Account      string `json:"account"`
	Batch        int    `json:"batch"`
	Tolerance    int    `json:"tolerance"`
	Timeout      int    `json:"timeout"`
	Pause        string `json:"pause"`
	Script       string `json:"script"`
	Args         string `json:"args"`
	CreateAt     int64  `json:"create_at"`
	CreateBy     string `json:"create_by"`
}

func (r *TaskRecord) TableName() string {
	return "task_record"
}

// create task
func (r *TaskRecord) Add(ctx *ctx.Context) error {
	if !ctx.IsCenter {
		err := poster.PostByUrls(ctx, "/v1/n9e/task-record-add", r)
		return err
	}

	return Insert(ctx, r)
}

// list task, filter by group_id, create_by
func TaskRecordTotal(ctx *ctx.Context, bgids []int64, beginTime int64, createBy, query string) (int64, error) {
	session := DB(ctx).Model(new(TaskRecord)).Where("create_at > ? and group_id in (?)", beginTime, bgids)

	if createBy != "" {
		session = session.Where("create_by = ?", createBy)
	}

	if query != "" {
		session = session.Where("title like ?", "%"+query+"%")
	}

	return Count(session)
}

func TaskRecordGets(ctx *ctx.Context, bgids []int64, beginTime int64, createBy, query string, limit, offset int) ([]*TaskRecord, error) {
	session := DB(ctx).Where("create_at > ? and group_id in (?)", beginTime, bgids).Order("create_at desc").Limit(limit).Offset(offset)

	if createBy != "" {
		session = session.Where("create_by = ?", createBy)
	}

	if query != "" {
		session = session.Where("title like ?", "%"+query+"%")
	}

	var lst []*TaskRecord
	err := session.Find(&lst).Error
	return lst, err
}

// update is_done field
func (r *TaskRecord) UpdateIsDone(ctx *ctx.Context, isDone int) error {
	return DB(ctx).Model(r).Update("is_done", isDone).Error
}
