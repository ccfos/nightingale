package migrate

import (
	"fmt"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ormx"

	imodels "github.com/flashcatcloud/ibex/src/models"
	"github.com/toolkits/pkg/logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) {
	MigrateTables(db)
	MigrateEsIndexPatternTable(db)
}

func MigrateIbexTables(db *gorm.DB) {
	var tableOptions string
	switch db.Dialector.(type) {
	case *mysql.Dialector:
		tableOptions = "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
	}

	if tableOptions != "" {
		db = db.Set("gorm:table_options", tableOptions)
	}

	dts := []interface{}{&imodels.TaskMeta{}, &imodels.TaskScheduler{}, &TaskHostDoing{}, &imodels.TaskAction{}}
	for _, dt := range dts {
		err := db.AutoMigrate(dt)
		if err != nil {
			logger.Errorf("failed to migrate table:%v %v", dt, err)
		}
	}

	for i := 0; i < 100; i++ {
		tableName := fmt.Sprintf("task_host_%d", i)
		exists := db.Migrator().HasTable(tableName)
		if exists {
			continue
		} else {
			err := db.Table(tableName).AutoMigrate(&imodels.TaskHost{})
			if err != nil {
				logger.Errorf("failed to migrate table:%s %v", tableName, err)
			}
		}
	}
}

func isPostgres(db *gorm.DB) bool {
	dialect := db.Dialector.Name()
	return dialect == "postgres"
}
func MigrateTables(db *gorm.DB) error {
	var tableOptions string
	switch db.Dialector.(type) {
	case *mysql.Dialector:
		tableOptions = "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
	}
	if tableOptions != "" {
		db = db.Set("gorm:table_options", tableOptions)
	}
	dts := []interface{}{&RecordingRule{}, &AlertRule{}, &AlertSubscribe{}, &AlertMute{},
		&TaskRecord{}, &ChartShare{}, &Target{}, &Configs{}, &Datasource{}, &NotifyTpl{},
		&Board{}, &BoardBusigroup{}, &Users{}, &SsoConfig{}, &models.BuiltinMetric{},
		&models.MetricFilter{}, &models.NotificationRecord{}, &models.TargetBusiGroup{},
		&models.UserToken{}, &models.DashAnnotation{}, MessageTemplate{}, NotifyRule{}, NotifyChannelConfig{}, &EsIndexPatternMigrate{},
		&models.EventPipeline{}, &models.EventPipelineExecution{}, &models.EmbeddedProduct{}, &models.SourceToken{},
		&models.SavedView{}, &models.UserViewFavorite{}}

	if isPostgres(db) {
		dts = append(dts, &models.PostgresBuiltinComponent{})
		DropUniqueFiledLimit(db, &models.PostgresBuiltinComponent{}, "idx_ident", "idx_ident")
	} else {
		dts = append(dts, &models.BuiltinComponent{})
		DropUniqueFiledLimit(db, &models.BuiltinComponent{}, "idx_ident", "idx_ident")
	}

	if !db.Migrator().HasColumn(&imodels.TaskSchedulerHealth{}, "scheduler") {
		dts = append(dts, &imodels.TaskSchedulerHealth{})
	}

	asyncDts := []interface{}{&AlertHisEvent{}, &AlertCurEvent{}}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("panic to migrate table: %v", r)
			}
		}()

		for _, dt := range asyncDts {
			if err := db.AutoMigrate(dt); err != nil {
				logger.Errorf("failed to migrate table %+v err:%v", dt, err)
			}
		}
	}()

	if !db.Migrator().HasTable(&models.BuiltinPayload{}) {
		if isPostgres(db) {
			dts = append(dts, &models.PostgresBuiltinPayload{})
		} else {
			dts = append(dts, &models.BuiltinPayload{})
		}
	} else {
		dts = append(dts, &BuiltinPayloads{})
	}

	for _, dt := range dts {
		err := db.AutoMigrate(dt)
		if err != nil {
			logger.Errorf("failed to migrate table:%v %v", dt, err)
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
	// 删除 builtin_metrics 表的 idx_collector_typ_name 唯一索引
	DropUniqueFiledLimit(db, &models.BuiltinMetric{}, "idx_collector_typ_name", "idx_collector_typ_name")

	return nil
}

func DropUniqueFiledLimit(db *gorm.DB, dst interface{}, uniqueFiled string, pgUniqueFiled string) { // UNIQUE KEY (`ckey`)
	// 先检查表是否存在，如果不存在则直接返回
	if !db.Migrator().HasTable(dst) {
		return
	}

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

type AlertRule struct {
	ExtraConfig       string                   `gorm:"type:text;column:extra_config"`
	CronPattern       string                   `gorm:"type:varchar(64);column:cron_pattern"`
	DatasourceQueries []models.DatasourceQuery `gorm:"datasource_queries;type:text;serializer:json"` // datasource queries
	NotifyRuleIds     []int64                  `gorm:"column:notify_rule_ids;type:varchar(1024)"`
	NotifyVersion     int                      `gorm:"column:notify_version;type:int;default:0"`
	PipelineConfigs   []models.PipelineConfig  `gorm:"column:pipeline_configs;type:text;serializer:json"`
}

type AlertSubscribe struct {
	ExtraConfig   string       `gorm:"type:text;column:extra_config"` // extra config
	Severities    string       `gorm:"column:severities;type:varchar(32);not null;default:''"`
	BusiGroups    ormx.JSONArr `gorm:"column:busi_groups;type:varchar(4096)"`
	Note          string       `gorm:"column:note;type:varchar(1024);default:'';comment:note"`
	RuleIds       []int64      `gorm:"column:rule_ids;type:varchar(1024)"`
	NotifyRuleIds []int64      `gorm:"column:notify_rule_ids;type:varchar(1024)"`
	NotifyVersion int          `gorm:"column:notify_version;type:int;default:0"`
}

type AlertMute struct {
	Severities string `gorm:"column:severities;type:varchar(32);not null;default:''"`
	Tags       string `gorm:"column:tags;type:varchar(4096);default:'[]';comment:json,map,tagkey->regexp|value"`
}

type RecordingRule struct {
	QueryConfigs      string                   `gorm:"type:text;not null;column:query_configs"` // query_configs
	DatasourceIds     string                   `gorm:"column:datasource_ids;type:varchar(255);default:'';comment:datasource ids"`
	CronPattern       string                   `gorm:"column:cron_pattern;type:varchar(255);default:'';comment:cron pattern"`
	DatasourceQueries []models.DatasourceQuery `json:"datasource_queries" gorm:"datasource_queries;type:text;serializer:json"` // datasource queries
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
	LastEvalTime  int64   `gorm:"column:last_eval_time;bigint(20);not null;default:0;comment:for time filter;index:idx_last_eval_time"`
	OriginalTags  string  `gorm:"column:original_tags;type:text;comment:labels key=val,,k2=v2"`
	NotifyRuleIds []int64 `gorm:"column:notify_rule_ids;type:text;serializer:json;comment:notify rule ids"`
}

type AlertCurEvent struct {
	OriginalTags  string  `gorm:"column:original_tags;type:text;comment:labels key=val,,k2=v2"`
	NotifyRuleIds []int64 `gorm:"column:notify_rule_ids;type:text;serializer:json;comment:notify rule ids"`
}

type Target struct {
	HostIp       string   `gorm:"column:host_ip;type:varchar(15);default:'';comment:IPv4 string;index:idx_host_ip"`
	AgentVersion string   `gorm:"column:agent_version;type:varchar(255);default:'';comment:agent version;index:idx_agent_version"`
	EngineName   string   `gorm:"column:engine_name;type:varchar(255);default:'';comment:engine name;index:idx_engine_name"`
	OS           string   `gorm:"column:os;type:varchar(31);default:'';comment:os type;index:idx_os"`
	HostTags     []string `gorm:"column:host_tags;type:text;comment:global labels set in conf file;serializer:json"`
}

type Datasource struct {
	IsDefault  bool   `gorm:"column:is_default;type:boolean;comment:is default datasource"`
	Identifier string `gorm:"column:identifier;type:varchar(255);default:'';comment:identifier"`
	Weight     int    `gorm:"column:weight;type:int;default:0;comment:weight for sorting"`
}

type Configs struct {
	Note string `gorm:"column:note;type:varchar(1024);default:'';comment:note"`
	Cval string `gorm:"column:cval;type:text;comment:config value"`
	//mysql tinyint//postgresql smallint
	External  int    `gorm:"column:external;type:int;default:0;comment:0\\:built-in 1\\:external"`
	Encrypted int    `gorm:"column:encrypted;type:int;default:0;comment:0\\:plaintext 1\\:ciphertext"`
	CreateAt  int64  `gorm:"column:create_at;type:int;default:0;comment:create_at"`
	CreateBy  string `gorm:"column:create_by;type:varchar(64);default:'';comment:create_by"`
	UpdateAt  int64  `gorm:"column:update_at;type:int;default:0;comment:update_at"`
	UpdateBy  string `gorm:"column:update_by;type:varchar(64);default:'';comment:update_by"`
}

type NotifyTpl struct {
	CreateAt int64  `gorm:"column:create_at;type:int;default:0;comment:create_at"`
	CreateBy string `gorm:"column:create_by;type:varchar(64);default:'';comment:create_by"`
	UpdateAt int64  `gorm:"column:update_at;type:int;default:0;comment:update_at"`
	UpdateBy string `gorm:"column:update_by;type:varchar(64);default:'';comment:update_by"`
}

type Board struct {
	PublicCate int    `gorm:"column:public_cate;int;not null;default:0;comment:0 anonymous 1 login 2 busi"`
	Note       string `gorm:"column:note;type:varchar(1024);not null;default:'';comment:note"`
}

type BoardBusigroup struct {
	BusiGroupId int64 `gorm:"column:busi_group_id;bigint(20);not null;default:0;comment:busi group id"`
	BoardId     int64 `gorm:"column:board_id;bigint(20);not null;default:0;comment:board id"`
}

type Users struct {
	Belong         string `gorm:"column:belong;type:varchar(16);default:'';comment:belong"`
	LastActiveTime int64  `gorm:"column:last_active_time;type:int;default:0;comment:last_active_time"`
	Phone          string `gorm:"column:phone;type:varchar(1024);not null;default:''"`
}

type SsoConfig struct {
	UpdateAt int64 `gorm:"column:update_at;type:int;default:0;comment:update_at"`
}

type BuiltinPayloads struct {
	UUID        int64  `json:"uuid" gorm:"type:bigint;not null;index:idx_uuid;comment:'uuid of payload'"`
	ComponentID int64  `json:"component_id" gorm:"type:bigint;index:idx_component,sort:asc;not null;default:0;comment:'component_id of payload'"`
	Note        string `json:"note" gorm:"type:varchar(1024);not null;default:'';comment:'note of payload'"`
}

type TaskHostDoing struct {
	Id             int64  `gorm:"column:id;index;primaryKey:false"`
	Host           string `gorm:"column:host;size:128;not null;index"`
	Clock          int64  `gorm:"column:clock;not null;default:0"`
	Action         string `gorm:"column:action;size:16;not null"`
	AlertTriggered bool   `gorm:"-"`
}

func (TaskHostDoing) TableName() string {
	return "task_host_doing"
}

type EsIndexPatternMigrate struct {
	CrossClusterEnabled int    `gorm:"column:cross_cluster_enabled;type:int;default:0"`
	Note                string `gorm:"column:note;type:varchar(1024);default:''"`
}

func (EsIndexPatternMigrate) TableName() string {
	return "es_index_pattern"
}

type DashAnnotation struct {
	Id          int64  `gorm:"column:id;primaryKey;autoIncrement"`
	DashboardId int64  `gorm:"column:dashboard_id;not null"`
	PanelId     string `gorm:"column:panel_id;type:varchar(191);not null"`
	Tags        string `gorm:"column:tags;type:text"`
	Description string `gorm:"column:description;type:text"`
	Config      string `gorm:"column:config;type:text"`
	TimeStart   int64  `gorm:"column:time_start;not null;default:0"`
	TimeEnd     int64  `gorm:"column:time_end;not null;default:0"`
	CreateAt    int64  `gorm:"column:create_at;not null;default:0"`
	CreateBy    string `gorm:"column:create_by;type:varchar(64);not null;default:''"`
	UpdateAt    int64  `gorm:"column:update_at;not null;default:0"`
	UpdateBy    string `gorm:"column:update_by;type:varchar(64);not null;default:''"`
}

func (DashAnnotation) TableName() string {
	return "dash_annotation"
}

type MessageTemplate struct {
	ID                 int64             `gorm:"column:id;primaryKey;autoIncrement"`
	Name               string            `gorm:"column:name;type:varchar(64);not null"`
	Ident              string            `gorm:"column:ident;type:varchar(64);not null"`
	Content            map[string]string `gorm:"column:content;type:text"`
	UserGroupIds       []int64           `gorm:"column:user_group_ids;type:varchar(64)"`
	NotifyChannelIdent string            `gorm:"column:notify_channel_ident;type:varchar(64);not null;default:''"`
	Private            int               `gorm:"column:private;type:int;not null;default:0"`
	Weight             int               `gorm:"column:weight;type:int;not null;default:0"`
	CreateAt           int64             `gorm:"column:create_at;not null;default:0"`
	CreateBy           string            `gorm:"column:create_by;type:varchar(64);not null;default:''"`
	UpdateAt           int64             `gorm:"column:update_at;not null;default:0"`
	UpdateBy           string            `gorm:"column:update_by;type:varchar(64);not null;default:''"`
}

func (t *MessageTemplate) TableName() string {
	return "message_template"
}

type NotifyRule struct {
	ID              int64                   `gorm:"column:id;primaryKey;autoIncrement"`
	Name            string                  `gorm:"column:name;type:varchar(255);not null"`
	Description     string                  `gorm:"column:description;type:text"`
	Enable          bool                    `gorm:"column:enable;not null;default:false"`
	UserGroupIds    []int64                 `gorm:"column:user_group_ids;type:varchar(255)"`
	NotifyConfigs   []models.NotifyConfig   `gorm:"column:notify_configs;type:text"`
	PipelineConfigs []models.PipelineConfig `gorm:"column:pipeline_configs;type:text"`
	ExtraConfig     interface{}             `gorm:"column:extra_config;type:text"`
	CreateAt        int64                   `gorm:"column:create_at;not null;default:0"`
	CreateBy        string                  `gorm:"column:create_by;type:varchar(64);not null;default:''"`
	UpdateAt        int64                   `gorm:"column:update_at;not null;default:0"`
	UpdateBy        string                  `gorm:"column:update_by;type:varchar(64);not null;default:''"`
}

func (r *NotifyRule) TableName() string {
	return "notify_rule"
}

type NotifyChannelConfig struct {
	ID            int64                    `gorm:"column:id;primaryKey;autoIncrement"`
	Name          string                   `gorm:"column:name;type:varchar(255);not null"`
	Ident         string                   `gorm:"column:ident;type:varchar(255);not null"`
	Description   string                   `gorm:"column:description;type:text"`
	Enable        bool                     `gorm:"column:enable;not null;default:false"`
	ParamConfig   models.NotifyParamConfig `gorm:"column:param_config;type:text"`
	RequestType   string                   `gorm:"column:request_type;type:varchar(50);not null"`
	RequestConfig *models.RequestConfig    `gorm:"column:request_config;type:text"`
	Weight        int                      `gorm:"column:weight;type:int;not null;default:0"`
	CreateAt      int64                    `gorm:"column:create_at;not null;default:0"`
	CreateBy      string                   `gorm:"column:create_by;type:varchar(64);not null;default:''"`
	UpdateAt      int64                    `gorm:"column:update_at;not null;default:0"`
	UpdateBy      string                   `gorm:"column:update_by;type:varchar(64);not null;default:''"`
}

func (c *NotifyChannelConfig) TableName() string {
	return "notify_channel"
}
