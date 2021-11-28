package models

type ChartShare struct {
	Id       int64  `json:"id" gorm:"primaryKey"`
	Cluster  string `json:"cluster"`
	Configs  string `json:"configs"`
	CreateBy string `json:"create_by"`
	CreateAt int64  `json:"create_at"`
}

func (cs *ChartShare) TableName() string {
	return "chart_share"
}

func (cs *ChartShare) Add() error {
	return Insert(cs)
}

func ChartShareGetsByIds(ids []int64) ([]ChartShare, error) {
	var lst []ChartShare
	if len(ids) == 0 {
		return lst, nil
	}

	err := DB().Where("id in ?", ids).Order("id").Find(&lst).Error
	return lst, err
}
