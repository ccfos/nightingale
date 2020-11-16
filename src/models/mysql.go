package models

import (
	"log"
	"path"
	"time"

	"xorm.io/core"
	"xorm.io/xorm"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/runner"
)

type MySQLConf struct {
	Addr  string `yaml:"addr"`
	Max   int    `yaml:"max"`
	Idle  int    `yaml:"idle"`
	Debug bool   `yaml:"debug"`
}

var DB = map[string]*xorm.Engine{}

func InitMySQL(names ...string) {
	confdir := path.Join(runner.Cwd, "etc")

	mysqlYml := path.Join(confdir, "mysql.local.yml")
	if !file.IsExist(mysqlYml) {
		mysqlYml = path.Join(confdir, "mysql.yml")
	}

	confs := make(map[string]MySQLConf)
	err := file.ReadYaml(mysqlYml, &confs)
	if err != nil {
		log.Fatalf("cannot read yml[%s]: %v", mysqlYml, err)
	}

	count := len(names)
	for i := 0; i < count; i++ {
		conf, has := confs[names[i]]
		if !has {
			log.Fatalf("no such mysql conf: %s", names[i])
		}

		db, err := xorm.NewEngine("mysql", conf.Addr)
		if err != nil {
			log.Fatalf("cannot connect mysql[%s]: %v", conf.Addr, err)
		}

		db.SetMaxIdleConns(conf.Idle)
		db.SetMaxOpenConns(conf.Max)
		db.SetConnMaxLifetime(time.Hour)
		db.ShowSQL(conf.Debug)
		db.Logger().SetLevel(core.LOG_INFO)

		DB[names[i]] = db
	}
}
