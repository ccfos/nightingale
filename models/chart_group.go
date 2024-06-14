package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"
)

type ChartGroup struct {
	Id          int64  `json:"id" gorm:"primaryKey"`
	DashboardId int64  `json:"dashboard_id"`
	Name        string `json:"name"`
	Weight      int    `json:"weight"`
}

func (cg *ChartGroup) TableName() string {
	return "chart_group"
}

func (cg *ChartGroup) Verify() error {
	if cg.DashboardId <= 0 {
		return errors.New("Arg(dashboard_id) invalid")
	}

	if str.Dangerous(cg.Name) {
		return errors.New("Name has invalid characters")
	}

	return nil
}

func (cg *ChartGroup) Add(ctx *ctx.Context) error {
	if err := cg.Verify(); err != nil {
		return err
	}

	return Insert(ctx, cg)
}

func (cg *ChartGroup) Update(ctx *ctx.Context, selectField interface{}, selectFields ...interface{}) error {
	if err := cg.Verify(); err != nil {
		return err
	}

	return DB(ctx).Model(cg).Select(selectField, selectFields...).Updates(cg).Error
}

func (cg *ChartGroup) Del(ctx *ctx.Context) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id=?", cg.Id).Delete(&Chart{}).Error; err != nil {
			return err
		}

		if err := tx.Where("id=?", cg.Id).Delete(&ChartGroup{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func NewDefaultChartGroup(ctx *ctx.Context, dashId int64) error {
	return Insert(ctx, &ChartGroup{
		DashboardId: dashId,
		Name:        "Default chart group",
		Weight:      0,
	})
}

func ChartGroupIdsOf(ctx *ctx.Context, dashId int64) ([]int64, error) {
	var ids []int64
	err := DB(ctx).Model(&ChartGroup{}).Where("dashboard_id = ?", dashId).Pluck("id", &ids).Error
	return ids, err
}

func ChartGroupsOf(ctx *ctx.Context, dashId int64) ([]ChartGroup, error) {
	var objs []ChartGroup
	err := DB(ctx).Where("dashboard_id = ?", dashId).Order("weight").Find(&objs).Error
	return objs, err
}
