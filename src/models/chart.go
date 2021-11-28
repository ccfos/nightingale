package models

type Chart struct {
	Id      int64  `json:"id" gorm:"primaryKey"`
	GroupId int64  `json:"group_id"`
	Configs string `json:"configs"`
	Weight  int    `json:"weight"`
}

func (c *Chart) TableName() string {
	return "chart"
}

func ChartsOf(chartGroupId int64) ([]Chart, error) {
	var objs []Chart
	err := DB().Where("group_id = ?", chartGroupId).Order("weight").Find(&objs).Error
	return objs, err
}

func (c *Chart) Add() error {
	return Insert(c)
}

func (c *Chart) Update(selectField interface{}, selectFields ...interface{}) error {
	return DB().Model(c).Select(selectField, selectFields...).Updates(c).Error
}

func (c *Chart) Del() error {
	return DB().Where("id=?", c.Id).Delete(&Chart{}).Error
}
