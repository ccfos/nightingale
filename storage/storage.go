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

	if cfg.DBType == "mysql" {
		db = db.Set("gorm:table_options", "CHARSET=utf8mb4")
	}
	return db, nil
}
