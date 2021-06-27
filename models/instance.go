package models

import (
	"time"

	"github.com/toolkits/pkg/logger"
	"xorm.io/builder"
)

type Instance struct {
	Service  string
	Endpoint string
	Clock    time.Time
}

func InstanceHeartbeat(service, endpoint string) error {
	cnt, err := DB.Where("service=? and endpoint = ?", service, endpoint).Count(new(Instance))
	if err != nil {
		logger.Errorf("mysql.error: InstanceHeartbeat count fail: %v", err)
		return err
	}

	if cnt == 0 {
		_, err = DB.Exec("INSERT INTO instance(service, endpoint, clock) VALUES(?, ?, now())", service, endpoint)
	} else {
		_, err = DB.Exec("UPDATE instance SET clock = now() WHERE service = ? and endpoint = ?", service, endpoint)
	}

	if err != nil {
		logger.Errorf("mysql.error: InstanceHeartbeat write fail: %v", err)
	}

	return err
}

func InstanceGetDead(service string) ([]string, error) {
	var arr []string
	err := DB.Table("instance").Where("service=? and clock < DATE_SUB(now(),INTERVAL 10 SECOND)", service).Select("endpoint").Find(&arr)
	if err != nil {
		logger.Errorf("mysql.error: InstanceGetDead fail: %v", err)
		return arr, err
	}

	if len(arr) == 0 {
		return []string{}, nil
	}

	return arr, nil
}

func InstanceGetAlive(service string) ([]string, error) {
	var arr []string
	err := DB.Table("instance").Where("service=? and clock >= DATE_SUB(now(),INTERVAL 10 SECOND)", service).Select("endpoint").Find(&arr)
	if err != nil {
		logger.Errorf("mysql.error: InstanceGetAlive fail: %v", err)
		return arr, err
	}

	if len(arr) == 0 {
		return []string{}, nil
	}

	return arr, nil
}

func InstanceDelDead(service string, endpoints []string) error {
	cond := builder.NewCond()
	cond = cond.And(builder.Eq{"service": service})
	cond = cond.And(builder.In("endpoint", endpoints))
	_, err := DB.Where(cond).Delete(new(Instance))
	if err != nil {
		logger.Errorf("mysql.error: InstanceDelDead fail: %v", err)
	}
	return err
}
