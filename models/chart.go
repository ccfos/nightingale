package models

import "github.com/ccfos/nightingale/v6/pkg/ctx"

type Chart struct {
	Id      int64  `json:"id" gorm:"primaryKey"`
	GroupId int64  `json:"group_id"`
	Configs string `json:"configs"`
	Weight  int    `json:"weight"`
}

func (c *Chart) TableName() string {
	return "chart"
}

func ChartsOf(ctx *ctx.Context, chartGroupId int64) ([]Chart, error) {
	var objs []Chart
	err := DB(ctx).Where("group_id = ?", chartGroupId).Order("weight").Find(&objs).Error
	return objs, err
}

func (c *Chart) Add(ctx *ctx.Context) error {
	return Insert(ctx, c)
}

func (c *Chart) Update(ctx *ctx.Context, selectField interface{}, selectFields ...interface{}) error {
	return DB(ctx).Model(c).Select(selectField, selectFields...).Updates(c).Error
}

func (c *Chart) Del(ctx *ctx.Context) error {
	return DB(ctx).Where("id=?", c.Id).Delete(&Chart{}).Error
}
