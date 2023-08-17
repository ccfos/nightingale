package postgres

import (
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"gorm.io/gorm"
)

type AlertSubscribe struct {
	ExtraConfig string       `gorm:"type:text;not null;column:extra_config"` // extra config
	Severities  string       `gorm:"column:severities;type:varchar(32);not null;default:''"`
	BusiGroups  ormx.JSONArr `gorm:"column:busi_groups;type:jsonb;not null;default:'[]'"`
}

func MigratePgTable(db *gorm.DB) error {
	err := db.AutoMigrate(&AlertSubscribe{})
	return err
}
