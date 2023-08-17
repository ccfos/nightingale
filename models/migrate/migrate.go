package migrate

import (
	"github.com/ccfos/nightingale/v6/models/migrate/postgres"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB, dbType string) {
	MigrateRecordingTable(db, dbType)
	MigrateEsIndexPatternTable(db)
}

type RecordingRule struct {
	QueryConfigs string `gorm:"type:text;not null;column:query_configs"` // query_configs
}

func MigrateRecordingTable(db *gorm.DB, dbType string) error {
	err := db.AutoMigrate(&RecordingRule{})
	if err != nil {
		logger.Errorf("failed to migrate recording rule table: %v", err)
		return err
	}

	err = db.AutoMigrate(&AlertRule{})
	if err != nil {
		logger.Errorf("failed to migrate recording rule table: %v", err)
		return err
	}

	if dbType == "mysql" {
		err = db.AutoMigrate(&AlertSubscribe{})
		if err != nil {
			logger.Errorf("failed to migrate recording rule table: %v", err)
			return err
		}
	} else {
		err = postgres.MigratePgTable(db)
		if err != nil {
			logger.Errorf("failed to migrate recording rule table: %v", err)
			return err
		}
	}

	err = db.AutoMigrate(&AlertMute{})
	if err != nil {
		logger.Errorf("failed to migrate recording rule table: %v", err)
		return err
	}
	return nil
}

type AlertRule struct {
	ExtraConfig string `gorm:"type:text;not null;column:extra_config"` // extra config
}

type AlertSubscribe struct {
	ExtraConfig string       `gorm:"type:text;not null;column:extra_config"` // extra config
	Severities  string       `gorm:"column:severities;type:varchar(32);not null;default:''"`
	BusiGroups  ormx.JSONArr `gorm:"column:busi_groups;type:varchar(4096);not null;default:'[]'"`
}

type AlertMute struct {
	Severities string `gorm:"column:severities;type:varchar(32);not null;default:''"`
}
