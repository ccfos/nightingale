package migrate

import (
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) {
	MigrateTables(db)
	MigrateEsIndexPatternTable(db)
}

func MigrateTables(db *gorm.DB) error {
	dts := []interface{}{&RecordingRule{}, &AlertRule{}, &AlertSubscribe{}, &AlertMute{}, &TaskRecord{}, &ChartShare{}, &Target{}}
	if !columnHasIndex(db, &AlertHisEvent{}, "last_eval_time") {
		dts = append(dts, &AlertHisEvent{})
	}
	err := db.AutoMigrate(dts...)
	if err != nil {
		logger.Errorf("failed to migrate table: %v", err)
		return err
	}

	if db.Migrator().HasColumn(&AlertingEngines{}, "cluster") {
		err = db.Migrator().RenameColumn(&AlertingEngines{}, "cluster", "engine_cluster")
		if err != nil {
			logger.Errorf("failed to renameColumn table: %v", err)
			return err
		}
	}
	if db.Migrator().HasColumn(&ChartShare{}, "dashboard_id") {
		err = db.Migrator().DropColumn(&ChartShare{}, "dashboard_id")
		if err != nil {
			logger.Errorf("failed to DropColumn table: %v", err)
		}
	}

	return nil
}

func columnHasIndex(db *gorm.DB, dst interface{}, indexColumn string) bool {
	indexes, err := db.Migrator().GetIndexes(dst)
	if err != nil {
		logger.Errorf("failed to table getIndexes: %v", err)
		return false
	}
	for i := range indexes {
		for j := range indexes[i].Columns() {
			if indexes[i].Columns()[j] == indexColumn {
				return true
			}
		}
	}
	return false
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
type AlertHisEvent struct {
	LastEvalTime int64 `gorm:"column:last_eval_time;bigint(20);not null;default:0;comment:for time filter;index:idx_last_eval_time"`
}
type Target struct {
	HostIp string `gorm:"column:host_ip;varchar(15);default:'';comment:IPv4 string;index:idx_host_ip""`
}
