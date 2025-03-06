package models

import (
	"encoding/json"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type DashAnnotation struct {
	Id          int64    `json:"id" gorm:"primaryKey"`
	DashboardId int64    `json:"dashboard_id"`
	PanelId     string   `json:"panel_id"`
	Tags        string   `json:"-"`
	TagsJSON    []string `json:"tags" gorm:"-"`
	Description string   `json:"description"`
	Config      string   `json:"config"`
	TimeStart   int64    `json:"time_start"`
	TimeEnd     int64    `json:"time_end"`
	CreateAt    int64    `json:"create_at"`
	CreateBy    string   `json:"create_by"`
	UpdateAt    int64    `json:"update_at"`
	UpdateBy    string   `json:"update_by"`
}

func (da *DashAnnotation) TableName() string {
	return "dash_annotation"
}

func (da *DashAnnotation) DB2FE() error {
	return json.Unmarshal([]byte(da.Tags), &da.TagsJSON)
}

func (da *DashAnnotation) FE2DB() error {
	b, err := json.Marshal(da.TagsJSON)
	if err != nil {
		return err
	}
	da.Tags = string(b)
	return nil
}

func (da *DashAnnotation) Add(ctx *ctx.Context) error {
	if err := da.FE2DB(); err != nil {
		return err
	}
	return Insert(ctx, da)
}

func (da *DashAnnotation) Update(ctx *ctx.Context) error {
	if err := da.FE2DB(); err != nil {
		return err
	}
	return DB(ctx).Model(da).Select("dashboard_id", "panel_id", "tags", "description", "config", "time_start", "time_end", "update_at", "update_by").Updates(da).Error
}

func DashAnnotationDel(ctx *ctx.Context, id int64) error {
	return DB(ctx).Where("id = ?", id).Delete(&DashAnnotation{}).Error
}

func DashAnnotationGet(ctx *ctx.Context, where string, args ...interface{}) (*DashAnnotation, error) {
	var lst []*DashAnnotation
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	err = lst[0].DB2FE()
	return lst[0], err
}

func DashAnnotationGets(ctx *ctx.Context, dashboardId int64, from, to int64, limit int) ([]DashAnnotation, error) {
	session := DB(ctx).Where("dashboard_id = ? AND time_start <= ? AND time_end >= ?", dashboardId, to, from)

	var lst []DashAnnotation
	err := session.Order("id").Limit(limit).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(lst); i++ {
		lst[i].DB2FE()
	}

	return lst, nil
}
