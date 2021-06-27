package models

import (
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type ChartGroup struct {
	Id          int64  `json:"id"`
	DashboardId int64  `json:"dashboard_id"`
	Name        string `json:"name"`
	Weight      int    `json:"weight"`
}

func (cg *ChartGroup) TableName() string {
	return "chart_group"
}

func (cg *ChartGroup) Validate() error {
	if str.Dangerous(cg.Name) {
		return _e("ChartGroup name has invalid characters")
	}
	return nil
}

func (cg *ChartGroup) Add() error {
	if err := cg.Validate(); err != nil {
		return err
	}

	return DBInsertOne(cg)
}

func (cg *ChartGroup) Update(cols ...string) error {
	if err := cg.Validate(); err != nil {
		return err
	}

	_, err := DB.Where("id=?", cg.Id).Cols(cols...).Update(cg)
	if err != nil {
		logger.Errorf("mysql.error: update chart_group(id=%d) fail: %v", cg.Id, err)
		return internalServerError
	}

	return nil
}

func (cg *ChartGroup) Del() error {
	_, err := DB.Where("group_id=?", cg.Id).Delete(new(Chart))
	if err != nil {
		logger.Errorf("mysql.error: delete chart by group_id(%d) fail: %v", cg.Id, err)
		return internalServerError
	}

	_, err = DB.Where("id=?", cg.Id).Delete(new(ChartGroup))
	if err != nil {
		logger.Errorf("mysql.error: delete chart_group(id=%d) fail: %v", cg.Id, err)
		return internalServerError
	}

	return nil
}

func ChartGroupGet(where string, args ...interface{}) (*ChartGroup, error) {
	var obj ChartGroup
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: get chart_group(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func ChartGroupGets(dashboardId int64) ([]ChartGroup, error) {
	var objs []ChartGroup
	err := DB.Where("dashboard_id=?", dashboardId).OrderBy("weight").Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: ChartGroupGets(dashboardId=%d) fail: %v", dashboardId, err)
		return nil, internalServerError
	}
	return objs, nil
}
