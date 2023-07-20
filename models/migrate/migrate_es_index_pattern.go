package migrate

import (
	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
)

type EsIndexPattern struct {
	Id                     int64  `gorm:"primaryKey;type:bigint unsigned"`
	DatasourceId           int64  `gorm:"type:bigint not null default '0';uniqueIndex:idx_ds_name"`
	Name                   string `gorm:"type:varchar(191) not null default '';uniqueIndex:idx_ds_name"`
	TimeField              string `gorm:"type:varchar(128) not null default ''"`
	AllowHideSystemIndices int    `gorm:"type:tinyint(1) not null default 0"`
	FieldsFormat           string `gorm:"type:varchar(4096) not null default ''"`
	CreateAt               int64  `gorm:"type:bigint  default '0'"`
	CreateBy               string `gorm:"type:varchar(64) default ''"`
	UpdateAt               int64  `gorm:"type:bigint  default '0'"`
	UpdateBy               string `gorm:"type:varchar(64) default ''"`
}

func MigrateEsIndexPatternTable(db *gorm.DB) error {
	db = db.Set("gorm:table_options", "CHARSET=utf8mb4")
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
