package models

import (
	"fmt"
	"time"
)

type OperationLog struct {
	Id       int64  `json:"id"`
	Username string `json:"username"`
	Clock    int64  `json:"clock"`
	ResCl    string `json:"res_cl"`
	ResId    string `json:"res_id"`
	Detail   string `json:"detail"`
}

func (ol *OperationLog) New() error {
	ol.Clock = time.Now().Unix()
	ol.Id = 0
	_, err := DB["rdb"].Insert(ol)
	return err
}

func OperationLogNew(username, rescl string, resid interface{}, detail string) error {
	now := time.Now().Unix()
	obj := OperationLog{
		Username: username,
		Clock:    now,
		ResCl:    rescl,
		ResId:    fmt.Sprint(resid),
		Detail:   detail,
	}

	_, err := DB["rdb"].Insert(obj)
	return err
}

func OperationLogGetsByRes(rescl, resid string, btime, etime int64, limit, offset int) ([]OperationLog, error) {
	var objs []OperationLog
	err := DB["rdb"].Where("clock between ? and ? and rescl=? and resid=?", btime, etime, rescl, resid).Desc("clock").Limit(limit, offset).Find(&objs)
	return objs, err
}

func OperationLogTotalByRes(rescl, resid string, btime, etime int64) (int64, error) {
	return DB["rdb"].Where("clock between ? and ? and rescl=? and resid=?", btime, etime, rescl, resid).Count(new(OperationLog))
}

func OperationLogQuery(query string, btime, etime int64, limit, offset int) ([]OperationLog, error) {
	session := DB["rdb"].Where("clock between ? and ?", btime, etime).Desc("clock").Limit(limit, offset)
	if query != "" {
		session = session.Where("detail like ?", "%"+query+"%")
	}
	var objs []OperationLog
	err := session.Find(&objs)
	return objs, err
}

func OperationLogTotal(query string, btime, etime int64) (int64, error) {
	session := DB["rdb"].Where("clock between ? and ?", btime, etime)
	if query != "" {
		session = session.Where("detail like ?", "%"+query+"%")
	}

	return session.Count(new(OperationLog))
}
