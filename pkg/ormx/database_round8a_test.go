package ormx

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func r8aDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return db
}

func r8aAssertSeeds(t *testing.T, db *gorm.DB) {
	t.Helper()
	var user InitUser
	require.NoError(t, db.First(&user, 1).Error)
	require.Equal(t, "root", user.Username)
	require.Equal(t, "Admin", user.Roles)
	require.Equal(t, "root.2020", user.Password)
	var roles []InitRole
	require.NoError(t, db.Find(&roles).Error)
	require.Len(t, roles, 3)
	var group InitUserGroup
	require.NoError(t, db.First(&group, 1).Error)
	require.Equal(t, "demo-root-group", group.Name)
	var member InitUserGroupMember
	require.NoError(t, db.First(&member).Error)
	require.Equal(t, uint64(1), member.UserID)
	require.Equal(t, int64(1), member.GroupID)
	var views []InitAlertAggrView
	require.NoError(t, db.Find(&views).Error)
	require.Len(t, views, 2)
	var ops []InitRoleOperation
	require.NoError(t, db.Find(&ops).Error)
	require.NotEmpty(t, ops)
	var guest int64
	require.NoError(t, db.Model(&InitRoleOperation{}).Where("role_name = ? AND operation = ?", "Guest", "/metric/explorer").Count(&guest).Error)
	require.Equal(t, int64(1), guest)
	for _, table := range []string{"users", "alert_rule", "alert_cur_event", "target", "datasource", "task_host_0", "task_host_1", "task_host_99"} {
		require.True(t, db.Migrator().HasTable(table), table)
	}
	for i := 1; i <= 99; i++ {
		require.True(t, db.Migrator().HasTable(fmt.Sprintf("task_host_%d", i)))
	}
}

func TestR8aDatabaseInitializers(t *testing.T) {
	// The Postgres initializer is not exercised here: running it against SQLite
	// would only test SQLite's index naming, not Postgres behavior.
	for _, kind := range []string{"sqlite", "MySQL"} {
		t.Run(kind, func(t *testing.T) {
			db := r8aDB(t)
			err := DataBaseInit(DBConfig{DBType: kind}, db)
			require.NoError(t, err)
			r8aAssertSeeds(t, db)
		})
	}
	require.ErrorContains(t, DataBaseInit(DBConfig{DBType: "other"}, nil), "unsupported database type")
}

func TestR8aNewSQLite(t *testing.T) {
	r8aIsolateMetrics(t)
	for _, preexisting := range []bool{false, true} {
		t.Run(fmt.Sprint(preexisting), func(t *testing.T) {
			config := DBConfig{DBType: "SQLite", DSN: filepath.Join(t.TempDir(), "nightingale.db"), Debug: true}
			exists, err := checkDatabaseExist(config)
			require.NoError(t, err)
			require.False(t, exists)
			if preexisting {
				require.NoError(t, createDatabase(config, &gorm.Config{}))
			}
			db, err := New(config)
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			r8aAssertSeeds(t, db)
			exists, err = checkDatabaseExist(config)
			require.NoError(t, err)
			require.True(t, exists)
			reopened, err := New(config)
			require.NoError(t, err)
			second, err := reopened.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, second.Close()) })
			var count int64
			require.NoError(t, reopened.Model(&InitUser{}).Count(&count).Error)
			require.Equal(t, int64(1), count)
		})
	}
}

func TestR8aDatabaseValidation(t *testing.T) {
	_, err := New(DBConfig{DBType: "unsupported"})
	require.ErrorContains(t, err, "not supported")
	require.ErrorContains(t, createDatabase(DBConfig{DBType: "unsupported"}, &gorm.Config{}), "not supported")
	_, err = checkDatabaseExist(DBConfig{DBType: "unsupported"})
	require.ErrorContains(t, err, "not supported")
	config := DBConfig{DBType: "sqlite", DSN: filepath.Join(t.TempDir(), "missing", "db")}
	_, err = New(config)
	require.ErrorContains(t, err, "failed to create database")
	for kind, want := range map[string]string{"MySQL": "SHOW DATABASES", "POSTGRES": "SELECT datname FROM pg_database", "sqlite": "", "other": ""} {
		require.Equal(t, want, genQuery(DBConfig{DBType: kind}))
	}
}
