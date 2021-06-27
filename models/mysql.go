package models

import (
	"fmt"
	"os"
	"time"

	"xorm.io/xorm"
	"xorm.io/xorm/log"

	"github.com/toolkits/pkg/logger"
)

var DB *xorm.Engine

type MysqlSection struct {
	Addr  string `yaml:"addr"`
	Max   int    `yaml:"max"`
	Idle  int    `yaml:"idle"`
	Debug bool   `yaml:"debug"`
}

var MySQL MysqlSection

func InitMySQL(MySQL MysqlSection) {
	conf := MySQL

	db, err := xorm.NewEngine("mysql", conf.Addr)
	if err != nil {
		fmt.Printf("cannot connect mysql[%s]: %v", conf.Addr, err)
		os.Exit(1)
	}

	db.SetMaxIdleConns(conf.Idle)
	db.SetMaxOpenConns(conf.Max)
	db.SetConnMaxLifetime(time.Hour)
	db.ShowSQL(conf.Debug)
	db.Logger().SetLevel(log.LOG_INFO)
	DB = db
}

func DBInsertOne(bean interface{}) error {
	_, err := DB.InsertOne(bean)
	if err != nil {
		logger.Errorf("mysql.error: insert fail: %v, to insert object: %+v", err, bean)
		return internalServerError
	}

	return nil
}
