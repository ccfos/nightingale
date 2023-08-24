package migrate

import (
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) {
	MigrateRecordingTable(db)
	MigrateEsIndexPatternTable(db)
}

func MigrateRecordingTable(db *gorm.DB) error {
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

	err = db.AutoMigrate(&AlertSubscribe{})
	if err != nil {
		logger.Errorf("failed to migrate recording rule table: %v", err)
		return err
	}

	err = db.AutoMigrate(&AlertMute{})
	if err != nil {
		logger.Errorf("failed to migrate recording rule table: %v", err)
		return err
	}
	if db.Migrator().HasColumn(&AlertingEngines{}, "cluster") {
		err = db.Migrator().RenameColumn(&AlertingEngines{}, "cluster", "engine_cluster")
		if err != nil {
			logger.Errorf("failed to migrate recording rule table: %v", err)
			return err
		}
	}

	err = db.AutoMigrate(TaskRecord{}, ChartShare{})
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

type RecordingRule struct {
	QueryConfigs  string `gorm:"type:text;not null;column:query_configs"` // query_configs
	DatasourceIds string `gorm:"column:datasource_ids;type:varchar(255);default:'';comment:datasource ids"`
}

type AlertingEngines struct {
	EngineCluster string `gorm:"column:engine_cluster;type:varchar(128);default:'';comment:n9e engine cluster"`
}

type ChartShare struct {
	DatasourceId int64 `gorm:"column:datasource_id;bigint(20);not null;default:0;comment:datasource id"`
}
type TaskRecord struct {
	EventId int64 `gorm:"column:event_id;bigint(20);not null;default:0;comment:event id;index:idx_event_id"`
}
