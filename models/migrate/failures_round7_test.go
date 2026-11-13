package migrate

import (
	"errors"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateEsIndexPatternTable_ClosedDatabaseReturnsError(t *testing.T) {
	db := sqliteMigrationDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	err = MigrateEsIndexPatternTable(db)

	require.ErrorContains(t, err, "database is closed")
}

func TestMigrateTables_SQLiteTableFailureDoesNotBlockLaterTables(t *testing.T) {
	db := sqliteMigrationDB(t)
	require.NoError(t, db.AutoMigrate(&models.NotificationRecord{}, &models.AlertHisEvent{}))
	failure := errors.New("recording rule DDL rejected")
	finished := make(chan error, 1)
	const callback = "round7:migration_ddl"
	require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register(callback, func(tx *gorm.DB) {
		if strings.HasPrefix(tx.Statement.SQL.String(), "CREATE TABLE `recording_rules`") {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { db.Callback().Raw().Remove(callback) })
	const completion = "round7:migration_finished"
	require.NoError(t, db.Callback().Raw().After("gorm:raw").Register(completion, func(tx *gorm.DB) {
		if strings.HasPrefix(tx.Statement.SQL.String(), "CREATE INDEX IF NOT EXISTS idx_nr_created_at ") {
			finished <- tx.Error
		}
	}))
	t.Cleanup(func() { db.Callback().Raw().Remove(completion) })

	err := MigrateTables(db)
	require.NoError(t, <-finished)

	require.NoError(t, err)
	require.False(t, db.Migrator().HasTable(&RecordingRule{}))
	require.True(t, db.Migrator().HasTable(&AlertRule{}))
	require.True(t, db.Migrator().HasTable(&models.AssistantChatRow{}))
	// The failed statement must not poison subsequent database operations.
	require.NoError(t, db.Callback().Raw().Remove(callback))
	require.NoError(t, db.AutoMigrate(&RecordingRule{}))
	require.True(t, db.Migrator().HasTable(&RecordingRule{}))
}
