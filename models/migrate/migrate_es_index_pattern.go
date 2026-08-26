package migrate

import (
	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
)

type EsIndexPattern struct {
	Id                     int64  `gorm:"primaryKey"`
	DatasourceId           int64  `gorm:"not null;default:0;uniqueIndex:idx_ds_name"`
	Name                   string `gorm:"size:191;not null;default:'';uniqueIndex:idx_ds_name"`
	TimeField              string `gorm:"size:128;not null;default:''"`
	AllowHideSystemIndices int    `gorm:"type:tinyint;not null;default:0"`
	FieldsFormat           string `gorm:"size:4096;not null;default:''"`
	CreateAt               int64  `gorm:"default:0"`
	CreateBy               string `gorm:"size:64;default:''"`
	UpdateAt               int64  `gorm:"default:0"`
	UpdateBy               string `gorm:"size:64;default:''"`
}

func MigrateEsIndexPatternTable(db *gorm.DB) error {
	if db.Dialector.Name() == "mysql" {
		db = db.Set("gorm:table_options", "CHARSET=utf8mb4")
	}
	if db.Migrator().HasTable("es_index_pattern") {
		return nil
	}

	err := db.Table("es_index_pattern").AutoMigrate(&EsIndexPattern{})
	if err != nil {
		logger.Errorf("failed to migrate es index pattern table: %v", err)
		return err
	}

	return nil
}
