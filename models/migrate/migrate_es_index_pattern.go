package migrate

import (
	"github.com/ccfos/nightingale/v6/models"

	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
)

func MigrateEsIndexPatternTable(db *gorm.DB) error {
	db = db.Set("gorm:table_options", "CHARSET=utf8mb4")
	if db.Migrator().HasTable(&models.EsIndexPattern{}) {
		return nil
	}

	err := db.Table("es_index_pattern").AutoMigrate(&models.EsIndexPattern{})
	if err != nil {
		logger.Errorf("failed to migrate es index pattern table: %v", err)
		return err
	}

	return nil
}
