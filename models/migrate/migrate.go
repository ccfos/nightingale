package migrate

import (
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) {
	MigrateTables(db)
	MigrateEsIndexPatternTable(db)
}

func MigrateTables(db *gorm.DB) error {
	dts := []interface{}{&RecordingRule{}, &AlertRule{}, &AlertSubscribe{}, &AlertMute{},
		&TaskRecord{}, &ChartShare{}, &Target{}, &Configs{}, &Datasource{}}
	if !columnHasIndex(db, &AlertHisEvent{}, "last_eval_time") {
		dts = append(dts, &AlertHisEvent{})
	}
	err := db.AutoMigrate(dts...)
	if err != nil {
		logger.Errorf("failed to migrate table: %v", err)
		return err
	}
	if !db.Migrator().HasColumn(&Configs{}, "encrypted") {
		err := db.AutoMigrate(&Configs{})
		if err != nil {
			logger.Errorf("failed to migrate configs table: %v", err)
			return err
		}
		//updates the database table by adding default values to existing rows.
		err = db.Model(&Configs{}).Select("external", "encrypted").Where("1=1").Updates(Configs{Encrypted: 0, External: 0}).Error
		if err != nil {
			logger.Errorf("update configs default value failed, %v", err)
		}
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
	if db.Migrator().HasIndex(&Configs{}, "ckey") {
		err = db.Migrator().DropIndex(&Configs{}, "ckey")
		if err != nil {
			logger.Errorf("failed to DropIndex ckey error: %v", err)
		}
	}
	InsertPermPoints(db)
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

func InsertPermPoints(db *gorm.DB) {
	var ops []models.RoleOperation
	ops = append(ops, models.RoleOperation{
		RoleName:  "Standard",
		Operation: "/alert-mutes/put",
	})

	ops = append(ops, models.RoleOperation{
		RoleName:  "Standard",
		Operation: "/log/index-patterns",
	})

	for _, op := range ops {
		exists, err := models.Exists(db.Model(&models.RoleOperation{}).Where("operation = ? and role_name = ?", op.Operation, op.RoleName))
		if err != nil {
			logger.Errorf("check role operation exists failed, %v", err)
			continue
		}
		if exists {
			continue
		}
		err = db.Create(&op).Error
		if err != nil {
			logger.Errorf("insert role operation failed, %v", err)
		}
	}
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
	HostIp string `gorm:"column:host_ip;varchar(15);default:'';comment:IPv4 string;index:idx_host_ip"`
}

type Datasource struct {
	IsDefault bool `gorm:"column:is_default;int;not null;default:0;comment:is default datasource"`
}

type Configs struct {
	Note string `gorm:"column:note;type:varchar(1024);default:'';comment:note"`
	//mysql tinyint//postgresql smallint
	External  int    `gorm:"column:external;type:int;default:0;comment:0\\:built-in 1\\:external"`
	Encrypted int    `gorm:"column:encrypted;type:int;default:0;comment:0\\:plaintext 1\\:ciphertext"`
	CreateAt  int64  `gorm:"column:create_at;type:int;default:0;comment:create_at"`
	CreateBy  string `gorm:"column:create_by;type:varchar(64);default:'';comment:cerate_by"`
	UpdateAt  int64  `gorm:"column:update_at;type:int;default:0;comment:update_at"`
	UpdateBy  string `gorm:"column:update_by;type:varchar(64);default:'';comment:update_by"`
}
