package migrate

import (
	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) {
	MigrateRecordingTable(db)
}

type RecordingRule struct {
	QueryConfigs string `gorm:"type:text;not null;column:query_configs"` // query_configs
}

func MigrateRecordingTable(db *gorm.DB) error {
	err := db.AutoMigrate(&RecordingRule{})
	if err != nil {
		logger.Errorf("failed to migrate recording rule table: %v", err)
		return err
	}
	return nil
}
