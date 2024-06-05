package migrate

import (
	"fmt"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ormx"

	imodels "github.com/flashcatcloud/ibex/src/models"
	"github.com/toolkits/pkg/logger"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) {
	MigrateTables(db)
	MigrateEsIndexPatternTable(db)
}

func MigrateIbexTables(db *gorm.DB) {
	dts := []interface{}{&imodels.TaskMeta{}, &imodels.TaskScheduler{}, &imodels.TaskSchedulerHealth{}, &imodels.TaskHostDoing{}, &imodels.TaskAction{}}
	for _, dt := range dts {
		err := db.AutoMigrate(dt)
		if err != nil {
			logger.Errorf("failed to migrate table:%v %v", dt, err)
		}
	}

	for i := 0; i < 100; i++ {
		tableName := fmt.Sprintf("task_host_%d", i)
		err := db.Table(tableName).AutoMigrate(&imodels.TaskHost{})
		if err != nil {
			logger.Errorf("failed to migrate table:%s %v", tableName, err)
		}
	}
}

func MigrateTables(db *gorm.DB) error {
	var tableOptions string
	switch db.Dialector.(type) {
	case *mysql.Dialector:
		tableOptions = "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
	case *postgres.Dialector:
		tableOptions = "ENCODING='UTF8'"
	}
	if tableOptions != "" {
		db = db.Set("gorm:table_options", tableOptions)
	}

	dts := []interface{}{&RecordingRule{}, &AlertRule{}, &AlertSubscribe{}, &AlertMute{},
		&TaskRecord{}, &ChartShare{}, &Target{}, &Configs{}, &Datasource{}, &NotifyTpl{},
		&Board{}, &BoardBusigroup{}, &Users{}, &SsoConfig{}, &models.BuiltinMetric{},
		&models.MetricFilter{}, &models.BuiltinComponent{}, &models.BuiltinPayload{}}

	if !columnHasIndex(db, &AlertHisEvent{}, "last_eval_time") {
		dts = append(dts, &AlertHisEvent{})
	}

	for _, dt := range dts {
		err := db.AutoMigrate(dt)
		if err != nil {
			logger.Errorf("failed to migrate table: %v", err)
		}
	}

	if db.Migrator().HasColumn(&AlertingEngines{}, "cluster") {
		err := db.Migrator().RenameColumn(&AlertingEngines{}, "cluster", "engine_cluster")
		if err != nil {
			logger.Errorf("failed to renameColumn table: %v", err)
		}
	}

	if db.Migrator().HasColumn(&ChartShare{}, "dashboard_id") {
		err := db.Migrator().DropColumn(&ChartShare{}, "dashboard_id")
		if err != nil {
			logger.Errorf("failed to DropColumn table: %v", err)
		}
	}
	DropUniqueFiledLimit(db, &Configs{}, "ckey", "configs_ckey_key")
	InsertPermPoints(db)
	return nil
}

func DropUniqueFiledLimit(db *gorm.DB, dst interface{}, uniqueFiled string, pgUniqueFiled string) { // UNIQUE KEY (`ckey`)
	if db.Migrator().HasIndex(dst, uniqueFiled) {
		err := db.Migrator().DropIndex(dst, uniqueFiled) //mysql  DROP INDEX
		if err != nil {
			logger.Errorf("failed to DropIndex(%s) error: %v", uniqueFiled, err)
		}
	}
	if db.Migrator().HasConstraint(dst, pgUniqueFiled) {
		err := db.Migrator().DropConstraint(dst, pgUniqueFiled) //pg  DROP CONSTRAINT
		if err != nil {
			logger.Errorf("failed to DropConstraint(%s) error: %v", pgUniqueFiled, err)
		}
	}
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

	ops = append(ops, models.RoleOperation{
		RoleName:  "Standard",
		Operation: "/help/variable-configs",
	})

	ops = append(ops, models.RoleOperation{
		RoleName:  "Admin",
		Operation: "/permissions",
	})

	ops = append(ops, models.RoleOperation{
		RoleName:  "Standard",
		Operation: "/ibex-settings",
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
	ExtraConfig string `gorm:"type:text;column:extra_config"` // extra config
}

type AlertSubscribe struct {
	ExtraConfig string       `gorm:"type:text;column:extra_config"` // extra config
	Severities  string       `gorm:"column:severities;type:varchar(32);not null;default:''"`
	BusiGroups  ormx.JSONArr `gorm:"column:busi_groups;type:varchar(4096);not null;default:'[]'"`
	Note        string       `gorm:"column:note;type:varchar(1024);default:'';comment:note"`
	RuleIds     []int64      `gorm:"column:rule_ids;type:varchar(1024);default:'';comment:rule_ids"`
}

type AlertMute struct {
	Severities string `gorm:"column:severities;type:varchar(32);not null;default:''"`
	Tags       string `gorm:"column:tags;type:varchar(4096);default:'[]';comment:json,map,tagkey->regexp|value"`
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
	HostIp       string `gorm:"column:host_ip;varchar(15);default:'';comment:IPv4 string;index:idx_host_ip"`
	AgentVersion string `gorm:"column:agent_version;varchar(255);default:'';comment:agent version;index:idx_agent_version"`
	EngineName   string `gorm:"column:engine_name;varchar(255);default:'';comment:engine name;index:idx_engine_name"`
}

type Datasource struct {
	IsDefault bool `gorm:"column:is_default;int;not null;default:0;comment:is default datasource"`
}

type Configs struct {
	Note string `gorm:"column:note;type:varchar(1024);default:'';comment:note"`
	Cval string `gorm:"column:cval;type:text;comment:config value"`
	//mysql tinyint//postgresql smallint
	External  int    `gorm:"column:external;type:int;default:0;comment:0\\:built-in 1\\:external"`
	Encrypted int    `gorm:"column:encrypted;type:int;default:0;comment:0\\:plaintext 1\\:ciphertext"`
	CreateAt  int64  `gorm:"column:create_at;type:int;default:0;comment:create_at"`
	CreateBy  string `gorm:"column:create_by;type:varchar(64);default:'';comment:cerate_by"`
	UpdateAt  int64  `gorm:"column:update_at;type:int;default:0;comment:update_at"`
	UpdateBy  string `gorm:"column:update_by;type:varchar(64);default:'';comment:update_by"`
}

type NotifyTpl struct {
	CreateAt int64  `gorm:"column:create_at;type:int;default:0;comment:create_at"`
	CreateBy string `gorm:"column:create_by;type:varchar(64);default:'';comment:cerate_by"`
	UpdateAt int64  `gorm:"column:update_at;type:int;default:0;comment:update_at"`
	UpdateBy string `gorm:"column:update_by;type:varchar(64);default:'';comment:update_by"`
}

type Board struct {
	PublicCate int `gorm:"column:public_cate;int;not null;default:0;comment:0 anonymous 1 login 2 busi"`
}

type BoardBusigroup struct {
	BusiGroupId int64 `gorm:"column:busi_group_id;bigint(20);not null;default:0;comment:busi group id"`
	BoardId     int64 `gorm:"column:board_id;bigint(20);not null;default:0;comment:board id"`
}

type Users struct {
	Belong         string `gorm:"column:belong;varchar(16);default:'';comment:belong"`
	LastActiveTime int64  `gorm:"column:last_active_time;type:int;default:0;comment:last_active_time"`
}

type SsoConfig struct {
	UpdateAt int64 `gorm:"column:update_at;type:int;default:0;comment:update_at"`
}
