package migrate

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ormx"

	imodels "github.com/flashcatcloud/ibex/src/models"
	"github.com/toolkits/pkg/logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// recoverMigratePanic keeps a panic inside a migration entry point from killing
// the process. Migration failures are already non-fatal (errors are only
// logged), and the schema will be repaired by another instance or the next
// restart.
//
// The remaining panic source is cross-instance and cannot be fixed on our side:
// the mysql driver's ColumnTypes stitches "SELECT * LIMIT 1" and an
// information_schema query together, so an ALTER TABLE run by another instance
// between the two leaves SQLColumnType nil and ColumnType.Length() dereferences
// it. (The other historical source, a *gorm.DB whose Error was set by an
// earlier statement, is now prevented structurally -- see migrationDB.)
func recoverMigratePanic(scene string) {
	if r := recover(); r != nil {
		logger.Errorf("recovered panic during %s: %v\n%s", scene, r, debug.Stack())
	}
}

// migrationDB returns a handle that is safe to reuse across statements and
// goroutines, carrying tableOptions if the dialect needs them.
//
// A chain method such as Set() returns a clone==0 handle: getInstance() hands
// back that same *DB, so every later statement shares one Statement and one
// Error field. Both have bitten us:
//
//   - Shared Statement: SQL stays on it while the statement is in flight
//     (processor.Execute only resets it afterwards) and Statement.clone()
//     copies non-empty SQL, so a concurrent caller re-issues someone else's
//     statement. A duplicated CREATE INDEX on a large alert_his_event blocks
//     on the metadata lock forever and hangs process startup.
//   - Shared Error: a failed statement sets db.Error on the shared handle for
//     good; gorm's row callback then skips execution, returns a nil *sql.Row,
//     and the next Scan panics.
//
// Deriving a Session (with a non-nil Context, which clones the Statement while
// Settings such as gorm:table_options survive) makes getInstance() build a
// fresh *DB per statement, so neither SQL nor Error leaks between callers.
func migrationDB(db *gorm.DB, tableOptions string) *gorm.DB {
	if tableOptions != "" {
		db = db.Set("gorm:table_options", tableOptions)
	}
	return db.Session(&gorm.Session{Context: context.Background()})
}

func Migrate(db *gorm.DB) {
	defer recoverMigratePanic("migrate tables")

	MigrateTables(db)
	MigrateEsIndexPatternTable(db)
}

func MigrateIbexTables(db *gorm.DB) {
	defer recoverMigratePanic("migrate ibex tables")

	var tableOptions string
	switch db.Dialector.(type) {
	case *mysql.Dialector:
		tableOptions = "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci"
	}

	db = migrationDB(db, tableOptions)

	fixTaskHostDoingPrimaryKey(db)

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

// fixTaskHostDoingPrimaryKey repairs MySQL tables created by older releases that
// declared id as the sole auto-increment primary key: inserting a second host for
// the same task violated the primary key. AutoMigrate never alters an existing
// table's primary key, so drop it manually. The table is deliberately left
// without a primary key, same as tables created from the SQL init files.
func fixTaskHostDoingPrimaryKey(db *gorm.DB) {
	if _, ok := db.Dialector.(*mysql.Dialector); !ok {
		return
	}

	// The caller hands over the chained session built by Set (clone=0), on
	// which every statement shares one instance: an error from any Raw/Exec
	// below would stick to db.Error forever. The caller's later AutoMigrate
	// would then inherit that error, gorm's row callback would skip execution
	// and return a nil *sql.Row, and Scan would panic with a nil dereference.
	// A fresh Session gives each statement its own instance, keeping errors
	// local to this function.
	db = db.Session(&gorm.Session{})

	if !db.Migrator().HasTable("task_host_doing") {
		return
	}

	var pkCols []string
	err := db.Raw(`SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'task_host_doing'
		AND CONSTRAINT_NAME = 'PRIMARY' ORDER BY ORDINAL_POSITION`).Scan(&pkCols).Error
	if err != nil {
		logger.Errorf("failed to check task_host_doing primary key: %v", err)
		return
	}

	if len(pkCols) != 1 || pkCols[0] != "id" {
		return
	}

	// MODIFY without AUTO_INCREMENT strips the auto-increment attribute (it must
	// go before DROP PRIMARY KEY); COLUMN_TYPE keeps the original signedness.
	var colType string
	err = db.Raw(`SELECT COLUMN_TYPE FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'task_host_doing'
		AND COLUMN_NAME = 'id'`).Scan(&colType).Error
	if err != nil || colType == "" {
		logger.Errorf("failed to get task_host_doing id column type: %v err: %v", colType, err)
		return
	}

	err = db.Exec(fmt.Sprintf("ALTER TABLE task_host_doing MODIFY id %s NOT NULL, DROP PRIMARY KEY", colType)).Error
	if err != nil {
		logger.Errorf("failed to drop task_host_doing legacy primary key: %v", err)
		return
	}

	logger.Info("dropped task_host_doing legacy primary key on id")
}

func isPostgres(db *gorm.DB) bool {
	dialect := db.Dialector.Name()
	return dialect == "postgres"
}

// notificationRecordIndexes 通知记录查询与清理所需的索引，见 models.NotificationRecord 的说明。
// cols 必须保持「逗号分隔的裸列名」这一形态：它既被拼进 CREATE INDEX，也被
// notificationRecordIndexColumnsReady 拆开逐列判存在性，写成 `created_at DESC`
// 或表达式索引会让后者永远判为缺列、索引再也建不出来
var notificationRecordIndexes = []struct {
	name string
	cols string
}{
	{"idx_nr_rule_created_evt", "notify_rule_id, created_at, event_id"},
	{"idx_nr_created_at", "created_at"},
}

// migrateNotificationRecordIndexes 给存量 notification_record 补索引。
// 不交给 AutoMigrate 按 tag 建：通知记录是持续高频写入的大表，而 PostgreSQL 的普通
// CREATE INDEX 会在整个构建期间持 SHARE 锁，把该表的 INSERT 全部挡住——放进 goroutine
// 只是不阻塞启动线程，数据库锁依旧存在，通知记录会在内存队列里积压直至被丢弃。
// 故 PostgreSQL 必须走 CONCURRENTLY，MySQL 显式要求 INPLACE/LOCK=NONE
func migrateNotificationRecordIndexes(db *gorm.DB) {
	for _, idx := range notificationRecordIndexes {
		if !notificationRecordIndexColumnsReady(db, idx.cols) {
			logger.Warningf("skip index %s on notification_record: column(%s) not migrated yet, will retry on next start", idx.name, idx.cols)
			continue
		}

		var err error
		switch {
		case isPostgres(db):
			// CREATE INDEX CONCURRENTLY 中途失败会留下 indisvalid=false 的残次索引，
			// 而 IF NOT EXISTS 会认为它已存在从而跳过重建：这个索引永远不被查询选用，
			// 却仍要承担每次写入的维护开销，必须先清掉
			if err = dropInvalidPgIndex(db, idx.name); err != nil {
				logger.Errorf("failed to drop invalid index %s on notification_record: %v", idx.name, err)
				continue
			}
			// CONCURRENTLY 不能在事务块里执行，gorm 的 Exec 不会自动开启事务
			err = db.Exec(fmt.Sprintf("CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON notification_record(%s)", idx.name, idx.cols)).Error
		case db.Dialector.Name() == "mysql":
			if db.Migrator().HasIndex("notification_record", idx.name) {
				continue
			}
			// 显式声明 LOCK=NONE：不支持在线加索引时宁可报错交给 DBA 用 gh-ost /
			// pt-online-schema-change 处理，也不要静默退化成阻塞写入的 DDL
			err = db.Exec(fmt.Sprintf("ALTER TABLE notification_record ADD INDEX %s (%s), ALGORITHM=INPLACE, LOCK=NONE", idx.name, idx.cols)).Error
		default:
			// SQLite 单写入者、数据量有限，普通建索引即可
			err = db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON notification_record(%s)", idx.name, idx.cols)).Error
		}

		if err != nil {
			logger.Errorf("failed to create index %s on notification_record, please create it manually with an online DDL tool (gh-ost / pt-online-schema-change): %v", idx.name, err)
		}
	}
}

// notificationRecordIndexColumnsReady 判断建该索引所需的列是否都已就位。
// 建索引跑在 MigrateTables 的异步 goroutine 里，而给 notification_record 补列的
// AutoMigrate 在主协程的同步循环里，两者没有先后保证：从 notify rule 特性之前升级、
// 或用旧版 docker/sqlite.sql 初始化出来的库，此刻可能还没有 notify_rule_id 列，
// 抢在补列之前建索引会报 Unknown column，只留下一行看起来像真故障的错误日志。
// 逐个索引门控而不是整体跳过：只依赖 created_at 的那个索引不受缺列影响，照常创建；
// 缺列的那个等下次启动列已补齐时自然建上
func notificationRecordIndexColumnsReady(db *gorm.DB, cols string) bool {
	for _, col := range strings.Split(cols, ",") {
		if !db.Migrator().HasColumn(&models.NotificationRecord{}, strings.TrimSpace(col)) {
			return false
		}
	}
	return true
}

func dropInvalidPgIndex(db *gorm.DB, name string) error {
	var invalid int64
	err := db.Raw("select count(1) from pg_index i join pg_class c on c.oid = i.indexrelid where c.relname = ? and not i.indisvalid", name).Scan(&invalid).Error
	if err != nil || invalid == 0 {
		return err
	}

	logger.Infof("dropping invalid index %s left behind by a failed CREATE INDEX CONCURRENTLY", name)
	return db.Exec("DROP INDEX CONCURRENTLY IF EXISTS " + name).Error
}
func MigrateTables(db *gorm.DB) error {
	var tableOptions string
	switch db.Dialector.(type) {
	case *mysql.Dialector:
		tableOptions = "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci"
	}
	db = migrationDB(db, tableOptions)
	dts := []interface{}{&RecordingRule{}, &AlertRule{}, &AlertSubscribe{}, &AlertMute{},
		&TaskRecord{}, &TaskTpl{}, &ChartShare{}, &Target{}, &Configs{}, &Datasource{}, &NotifyTpl{},
		&Board{}, &BoardBusigroup{}, &Users{}, &SsoConfig{}, &models.BuiltinMetric{},
		&models.MetricFilter{}, &models.NotificationRecord{}, &models.TargetBusiGroup{},
		&models.UserToken{}, &models.DashAnnotation{}, MessageTemplate{}, NotifyRule{}, NotifyChannelConfig{}, &EsIndexPatternMigrate{},
		&models.EventPipeline{}, &models.EmbeddedProduct{}, &models.SourceToken{},
		&models.SavedView{}, &models.UserViewFavorite{},
		&models.AILLMConfig{}, &models.AIAgent{}, &models.AISkill{},
		&models.AssistantChatRow{}}

	if isPostgres(db) {
		dts = append(dts, &models.AssistantMessageRow{}) // PostgreSQL: text is unlimited
		dts = append(dts, &models.PostgresBuiltinComponent{})
		dts = append(dts, &models.PostgresAISkillFile{})
		dts = append(dts, &models.PostgresEventPipelineExecution{})
		DropUniqueFiledLimit(db, &models.PostgresBuiltinComponent{}, "idx_ident", "idx_ident")
	} else {
		dts = append(dts, &models.MysqlAssistantMessageRow{}) // MySQL: mediumtext; SQLite: treated as text
		dts = append(dts, &models.BuiltinComponent{})
		dts = append(dts, &models.AISkillFile{})
		dts = append(dts, &models.EventPipelineExecution{})
		DropUniqueFiledLimit(db, &models.BuiltinComponent{}, "idx_ident", "idx_ident")
	}

	if !db.Migrator().HasColumn(&imodels.TaskSchedulerHealth{}, "scheduler") {
		dts = append(dts, &imodels.TaskSchedulerHealth{})
	}

	asyncDts := []interface{}{&AlertHisEvent{}, &AlertCurEvent{}}
	// migrationDB 已经保证每条语句都拿到独立的 Statement，这里再派生一个专供
	// goroutine 使用的 handle：这段异步迁移会在大表上跑数十分钟的 CREATE INDEX，
	// 一旦哪天 db 又退化成链式方法返回的 clone==0 实例，主流程就会把这条在飞的
	// SQL 复制走重复执行、卡死在 metadata lock 上，整个启动挂死。宁可多一层。
	asyncDB := db.Session(&gorm.Session{Context: context.Background()})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("panic to migrate table: %v", r)
			}
		}()

		for _, dt := range asyncDts {
			if err := asyncDB.AutoMigrate(dt); err != nil {
				logger.Errorf("failed to migrate table %+v err:%v", dt, err)
			}
		}

		// 索引用原生 SQL 创建，不在部分结构体上声明 group_id 列：
		// 存量库该列是 bigint unsigned 且无默认值，声明列会让 AutoMigrate
		// 发出 MODIFY COLUMN，在大表上全表重建锁写
		if !asyncDB.Migrator().HasIndex("alert_his_event", "idx_group_last_eval_time") {
			if err := asyncDB.Exec("CREATE INDEX idx_group_last_eval_time ON alert_his_event(group_id, last_eval_time)").Error; err != nil {
				logger.Errorf("failed to create index idx_group_last_eval_time on alert_his_event: %v", err)
			}
		}

		migrateNotificationRecordIndexes(asyncDB)
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

// AssistantChat / AssistantMessage row structs live in models package
// (models.AssistantChatRow / models.AssistantMessageRow) so both
// migrate and storage can share them.

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
	TimeZone          string                   `gorm:"type:varchar(64);column:time_zone;not null;default:''"`
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
	MuteType   int    `gorm:"column:mute_type;type:int;not null;default:0;comment:0-mute event and notify,1-mute notify only"`
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
	EventId      int64  `gorm:"column:event_id;bigint(20);not null;default:0;comment:event id;index:idx_event_id"`
	AuthLevel    int    `gorm:"column:auth_level;type:int;not null;default:0;comment:ai task auth level, 0=off 1/2/3=level"`
	SystemCaller string `gorm:"column:system_caller;type:varchar(64);not null;default:'';comment:caller system, e.g. ai-agent"`
}
type TaskTpl struct {
	AuthLevel int `gorm:"column:auth_level;type:int;not null;default:0;comment:ai task auth level, 0=off 1/2/3=level"`
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

// TaskHostDoing holds one row per (task id, host), so id alone must NOT be the
// primary key. `primaryKey:false` does not work: GORM force-promotes a lone
// `id` column to primary key (and implicitly marks int primary keys
// auto-increment), so declare the natural composite key (id, host) instead.
type TaskHostDoing struct {
	Id             int64  `gorm:"column:id;primaryKey;autoIncrement:false;index"`
	Host           string `gorm:"column:host;size:128;not null;primaryKey;index"`
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
	Weight              int    `gorm:"column:weight;type:int;default:0"`
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
	Lang               string            `gorm:"column:lang;type:varchar(32);not null;default:''"`
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
