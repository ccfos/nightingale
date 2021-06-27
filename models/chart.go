package models

import "github.com/toolkits/pkg/logger"

type Chart struct {
	Id      int64  `json:"id"`
	GroupId int64  `json:"group_id"`
	Configs string `json:"configs"`
	Weight  int    `json:"weight"`
}

func (c *Chart) TableName() string {
	return "chart"
}

func (c *Chart) Add() error {
	return DBInsertOne(c)
}

func (c *Chart) Update(cols ...string) error {
	_, err := DB.Where("id=?", c.Id).Cols(cols...).Update(c)
	if err != nil {
		logger.Errorf("mysql.error: update chart(id=%d) fail: %v", c.Id, err)
		return internalServerError
	}

	return nil
}

func (c *Chart) Del() error {
	_, err := DB.Where("id=?", c.Id).Delete(new(Chart))
	if err != nil {
		logger.Errorf("mysql.error: delete chart(id=%d) fail: %v", c.Id, err)
		return internalServerError
	}

	return nil
}

func ChartGets(groupId int64) ([]Chart, error) {
	var objs []Chart
	err := DB.Where("group_id=?", groupId).OrderBy("weight").Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: ChartGets(groupId=%d) fail: %v", groupId, err)
		return nil, internalServerError
	}

	if len(objs) == 0 {
		return []Chart{}, nil
	}

	return objs, nil
}

func ChartGet(where string, args ...interface{}) (*Chart, error) {
	var obj Chart
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: get chart(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}
