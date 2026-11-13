package migrate

import (
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func sqliteMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return db
}

func TestMigrateTables_SQLiteUpgradesLegacyColumnsAndPreservesData(t *testing.T) {
	db := sqliteMigrationDB(t)
	require.NoError(t, db.AutoMigrate(&models.NotificationRecord{}, &models.AlertHisEvent{}))
	require.NoError(t, db.Exec("CREATE TABLE alerting_engines (cluster text)").Error)
	require.NoError(t, db.Exec("INSERT INTO alerting_engines (cluster) VALUES ('legacy-engine')").Error)
	require.NoError(t, db.Exec("CREATE TABLE chart_share (id integer primary key, dashboard_id integer)").Error)
	require.NoError(t, db.Exec("INSERT INTO chart_share (id, dashboard_id) VALUES (1, 7)").Error)

	// The final raw index statement marks completion of the finite background migration.
	finished := make(chan error, 1)
	require.NoError(t, db.Callback().Raw().After("gorm:raw").Register("round5:migration_done", func(tx *gorm.DB) {
		if strings.HasPrefix(tx.Statement.SQL.String(), "CREATE INDEX IF NOT EXISTS idx_nr_created_at ") {
			finished <- tx.Error
		}
	}))

	require.NoError(t, MigrateTables(db))
	require.NoError(t, <-finished)

	for _, table := range []string{
		"ai_assistant_chat", "ai_assistant_message", "ai_skill", "event_pipeline_execution", "builtin_payloads",
	} {
		require.True(t, db.Migrator().HasTable(table), table)
	}
	require.True(t, db.Migrator().HasColumn(&TaskRecord{}, "auth_level"))
	require.True(t, db.Migrator().HasColumn(&Target{}, "host_tags"))
	require.True(t, db.Migrator().HasIndex("alert_his_event", "idx_group_last_eval_time"))
	require.True(t, db.Migrator().HasIndex("notification_record", "idx_nr_rule_created_evt"))
	require.False(t, db.Migrator().HasColumn(&AlertingEngines{}, "cluster"))
	var engine string
	require.NoError(t, db.Table("alerting_engines").Pluck("engine_cluster", &engine).Error)
	require.Equal(t, "legacy-engine", engine)
	require.False(t, db.Migrator().HasColumn(&ChartShare{}, "dashboard_id"))
	var share struct {
		ID           int64
		DatasourceID int64
	}
	require.NoError(t, db.Table("chart_share").First(&share).Error)
	require.Equal(t, int64(1), share.ID)
	require.Zero(t, share.DatasourceID)

	Migrate(db)
	require.NoError(t, <-finished)

	var count int64
	require.NoError(t, db.Table("chart_share").Count(&count).Error)
	require.Equal(t, int64(1), count)
	require.NoError(t, db.Table("alerting_engines").Pluck("engine_cluster", &engine).Error)
	require.Equal(t, "legacy-engine", engine)
}

func TestMigrateEsIndexPatternTable_LeavesExistingDataAndIndexIntact(t *testing.T) {
	db := sqliteMigrationDB(t)
	require.NoError(t, db.Table("es_index_pattern").AutoMigrate(&EsIndexPattern{}))
	require.NoError(t, db.Table("es_index_pattern").Create(&EsIndexPattern{
		Id: 1, DatasourceId: 7, Name: "logs", TimeField: "@timestamp",
	}).Error)

	require.NoError(t, MigrateEsIndexPatternTable(db))
	require.NoError(t, MigrateEsIndexPatternTable(db))

	var row EsIndexPattern
	require.NoError(t, db.Table("es_index_pattern").First(&row, 1).Error)
	require.Equal(t, "@timestamp", row.TimeField)
	require.True(t, db.Migrator().HasIndex("es_index_pattern", "idx_ds_name"))
	require.Error(t, db.Table("es_index_pattern").Create(&EsIndexPattern{Id: 2, DatasourceId: 7, Name: "logs"}).Error)
}

func TestDropUniqueFiledLimit_RemovesOnlyRequestedIndexIdempotently(t *testing.T) {
	db := sqliteMigrationDB(t)
	require.NoError(t, db.AutoMigrate(&models.BuiltinMetric{}))
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_collector_typ_name ON builtin_metrics(collector, typ, name)").Error)
	require.True(t, columnHasIndex(db, &models.BuiltinMetric{}, "collector"))

	DropUniqueFiledLimit(db, &models.BuiltinMetric{}, "idx_collector_typ_name", "absent_constraint")
	DropUniqueFiledLimit(db, &models.BuiltinMetric{}, "idx_collector_typ_name", "absent_constraint")

	require.False(t, db.Migrator().HasIndex(&models.BuiltinMetric{}, "idx_collector_typ_name"))
	require.True(t, columnHasIndex(db, &models.BuiltinMetric{}, "collector"))
	require.False(t, columnHasIndex(db, &models.BuiltinMetric{}, "missing_column"))
	// Dropping a legacy index before its table exists must leave the database usable.
	DropUniqueFiledLimit(db, &Configs{}, "ckey", "configs_ckey_key")
	require.NoError(t, db.AutoMigrate(&Configs{}))
	require.True(t, db.Migrator().HasTable(&Configs{}))
}

func TestColumnHasIndex_DatabaseFailureReturnsFalse(t *testing.T) {
	db := sqliteMigrationDB(t)
	require.NoError(t, db.AutoMigrate(&models.BuiltinMetric{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	found := columnHasIndex(db, &models.BuiltinMetric{}, "collector")

	require.False(t, found)
}

func TestRecoverMigratePanic_ContainsPanicAtMigrationBoundary(t *testing.T) {
	// A driver panic must stay within the migration boundary so startup can continue.
	require.NotPanics(t, func() {
		defer recoverMigratePanic("round5 migration")
		panic("simulated driver panic")
	})
}
