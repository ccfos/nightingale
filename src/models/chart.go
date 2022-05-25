package models

type Chart struct {
	Id      int64  `json:"id" gorm:"primaryKey"`
	Cid 	string `json:"cid"`
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

func GetChartByCid(cid string) (Chart, error) {
	var obj Chart
	err := DB().Where("cid = ?", cid).Limit(1).Find(&obj).Error
	return obj, err
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
