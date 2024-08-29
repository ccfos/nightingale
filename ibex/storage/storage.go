package storage

import (
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(cfg ormx.DBConfig) (err error) {
	DB, err = ormx.New(cfg)
	return
}
