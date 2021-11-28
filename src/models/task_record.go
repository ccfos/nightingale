package models

type TaskRecord struct {
	Id           int64  `json:"id" gorm:"primaryKey"`
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
func (r *TaskRecord) Add() error {
	return Insert(r)
}

// list task, filter by group_id, create_by
func TaskRecordTotal(bgid, beginTime int64, createBy, query string) (int64, error) {
	session := DB().Model(new(TaskRecord)).Where("create_at > ? and group_id = ?", beginTime, bgid)

	if createBy != "" {
		session = session.Where("create_by = ?", createBy)
	}

	if query != "" {
		session = session.Where("title like ?", "%"+query+"%")
	}

	return Count(session)
}

func TaskRecordGets(bgid, beginTime int64, createBy, query string, limit, offset int) ([]*TaskRecord, error) {
	session := DB().Where("create_at > ? and group_id = ?", beginTime, bgid).Order("create_at desc").Limit(limit).Offset(offset)

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
func (r *TaskRecord) UpdateIsDone(isDone int) error {
	return DB().Model(r).Update("is_done", isDone).Error
}
