package models

import "github.com/toolkits/pkg/logger"

type ChartTmp struct {
	Id       int64  `json:"id"`
	Configs  string `json:"configs"`
	CreateBy string `json:"create_by"`
	CreateAt int64  `json:"create_at"`
}

func (t *ChartTmp) Add() error {
	_, err := DB.InsertOne(t)
	return err
}

func ChartTmpGet(where string, args ...interface{}) (*ChartTmp, error) {
	var obj ChartTmp
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: get chart_tmp(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}
