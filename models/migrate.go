package models

import (
	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) {
	MigrateRecordingTable(db)
}

func MigrateRecordingTable(db *gorm.DB) error {
	err := db.AutoMigrate(&RecordingRule{})
	if err != nil {
		logger.Errorf("failed to migrate recording rule table: %v", err)
		return err
	}
	return nil
}
