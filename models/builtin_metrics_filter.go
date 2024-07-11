package models

import (
	"errors"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type MetricFilter struct {
	ID         int64       `json:"id" gorm:"primaryKey;type:bigint;autoIncrement;comment:'unique identifier'"`
	Name       string      `json:"name" gorm:"type:varchar(191);not null;index:idx_metricfilter_name,sort:asc;comment:'name of metric filter'"`
	Configs    string      `json:"configs" gorm:"type:varchar(4096);not null;comment:'configuration of metric filter'"`
	GroupsPerm []GroupPerm `json:"groups_perm" gorm:"type:text;serializer:json;"`
	CreateAt   int64       `json:"create_at" gorm:"type:bigint;not null;default:0;comment:'create time'"`
	CreateBy   string      `json:"create_by" gorm:"type:varchar(191);not null;default:'';comment:'creator'"`
	UpdateAt   int64       `json:"update_at" gorm:"type:bigint;not null;default:0;comment:'update time'"`
	UpdateBy   string      `json:"update_by" gorm:"type:varchar(191);not null;default:'';comment:'updater'"`
}

type GroupPerm struct {
	Gid   int64 `json:"gid"`
	Write bool  `json:"write"` // write permission
}

func (f *MetricFilter) TableName() string {
	return "metric_filter"
}

func (f *MetricFilter) Verify() error {
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" {
		return errors.New("name is blank")
	}
	f.Configs = strings.TrimSpace(f.Configs)
	if f.Configs == "" {
		return errors.New("configs is blank")
	}
	return nil
}

func (f *MetricFilter) Add(ctx *ctx.Context) error {
	if err := f.Verify(); err != nil {
		return err
	}
	now := time.Now().Unix()
	f.CreateAt = now
	f.UpdateAt = now
	return Insert(ctx, f)
}

func (f *MetricFilter) Update(ctx *ctx.Context) error {
	if err := f.Verify(); err != nil {
		return err
	}
	f.UpdateAt = time.Now().Unix()
	return DB(ctx).Model(f).Select("name", "configs", "groups_perm", "update_at", "update_by").Updates(f).Error
}

func MetricFilterDel(ctx *ctx.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	return DB(ctx).Where("id in ?", ids).Delete(new(MetricFilter)).Error
}

func MetricFilterGets(ctx *ctx.Context, where string, args ...interface{}) ([]MetricFilter, error) {
	var lst []MetricFilter
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	return lst, err
}

// get by id
func MetricFilterGet(ctx *ctx.Context, id int64) (*MetricFilter, error) {
	var f MetricFilter
	err := DB(ctx).Where("id = ?", id).First(&f).Error
	return &f, err
}
