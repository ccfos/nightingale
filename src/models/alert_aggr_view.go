package models

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// AlertAggrView 在告警聚合视图查看的时候，要存储一些聚合规则
type AlertAggrView struct {
	Id       int64  `json:"id" gorm:"primaryKey"`
	Name     string `json:"name"`
	Rule     string `json:"rule"`
	Cate     int    `json:"cate"`
	UserId   int64  `json:"user_id"`
	CreateAt int64  `json:"create_at"`
	CreateBy string `json:"create_by"`
	UpdateAt int64  `json:"update_at"`
}

func (v *AlertAggrView) TableName() string {
	return "alert_aggr_view"
}

func (v *AlertAggrView) Verify() error {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" {
		return errors.New("name is blank")
	}

	v.Rule = strings.TrimSpace(v.Rule)
	if v.Rule == "" {
		return errors.New("rule is blank")
	}

	return nil
}

func (v *AlertAggrView) Add() error {
	if err := v.Verify(); err != nil {
		return err
	}

	now := time.Now().Unix()
	v.CreateAt = now
	v.UpdateAt = now
	v.Cate = 1
	return Insert(v)
}

func (v *AlertAggrView) Update(name, rule string) error {
	if err := v.Verify(); err != nil {
		return err
	}

	v.UpdateAt = time.Now().Unix()
	v.Name = name
	v.Rule = rule

	return DB().Model(v).Select("name", "rule", "update_at").Updates(v).Error
}

// AlertAggrViewDel: userid for safe delete
func AlertAggrViewDel(ids []int64, userId interface{}) error {
	if len(ids) == 0 {
		return nil
	}

	return DB().Where("id in ? and user_id = ?", ids, userId).Delete(new(AlertAggrView)).Error
}

func AlertAggrViewGets(userId interface{}) ([]AlertAggrView, error) {
	var lst []AlertAggrView
	err := DB().Where("user_id = ? or cate = 0", userId).Find(&lst).Error
	if err == nil && len(lst) > 0 {
		sort.Slice(lst, func(i, j int) bool {
			return lst[i].Name < lst[j].Name
		})
	}
	return lst, err
}

func AlertAggrViewGet(where string, args ...interface{}) (*AlertAggrView, error) {
	var lst []*AlertAggrView
	err := DB().Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}
