package models

import "github.com/ccfos/nightingale/v6/pkg/ctx"

type ChartShare struct {
	Id           int64  `json:"id" gorm:"primaryKey"`
	Cluster      string `json:"cluster"`
	DatasourceId int64  `json:"datasource_id"`
	Configs      string `json:"configs"`
	CreateBy     string `json:"create_by"`
	CreateAt     int64  `json:"create_at"`
}

func (cs *ChartShare) TableName() string {
	return "chart_share"
}

func (cs *ChartShare) Add(ctx *ctx.Context) error {
	return Insert(ctx, cs)
}

func ChartShareGetsByIds(ctx *ctx.Context, ids []int64) ([]ChartShare, error) {
	var lst []ChartShare
	if len(ids) == 0 {
		return lst, nil
	}

	err := DB(ctx).Where("id in ?", ids).Order("id").Find(&lst).Error
	return lst, err
}
