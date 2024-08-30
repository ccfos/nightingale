package storage

import (
	"github.com/ccfos/nightingale/v6/pkg/ormx"

	"gorm.io/gorm"
)

func New(cfg ormx.DBConfig) (*gorm.DB, error) {
	db, err := ormx.New(cfg)
	if err != nil {
		return nil, err
	}

	return db, nil
}

var IbexDB *gorm.DB

func InitIbexDB(cfg ormx.DBConfig) (err error) {
	IbexDB, err = ormx.New(cfg)
	return
}
