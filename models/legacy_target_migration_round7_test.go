package models

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TargetBindBgids reads targets outside its transaction, so the migration needs a
// second connection to the same database: a file-backed SQLite instead of :memory:.
func migrationContext(t *testing.T) *ctx.Context {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "migrate.db") + "?_pragma=busy_timeout(5000)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(2)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&Target{}, &TargetBusiGroup{}, &BusiGroup{}))
	return ctx.NewContext(context.Background(), db, true)
}

func TestDoMigrateBg_MovesLegacyGroupIntoBindings(t *testing.T) {
	c := migrationContext(t)
	require.NoError(t, c.DB.Create(&BusiGroup{Id: 7, Name: "operations"}).Error)
	require.NoError(t, c.DB.Create(&[]Target{
		{Ident: "legacy", GroupId: 7, Tags: "env=prod "},
		{Ident: "unassigned", Tags: "env=dev "},
	}).Error)

	err := DoMigrateBg(c, "team")

	require.NoError(t, err)
	groups, err := TargetGroupIdsGetByIdent(c, "legacy")
	require.NoError(t, err)
	require.Equal(t, []int64{7}, groups)
	var legacy Target
	require.NoError(t, c.DB.First(&legacy, "ident = ?", "legacy").Error)
	require.Zero(t, legacy.GroupId)
	require.Equal(t, "env=prod ", legacy.Tags)
	groups, err = TargetGroupIdsGetByIdent(c, "unassigned")
	require.NoError(t, err)
	require.Empty(t, groups)
}

func TestDoMigrateBg_AddsEnabledGroupLabel(t *testing.T) {
	c := migrationContext(t)
	require.NoError(t, c.DB.Create(&BusiGroup{Id: 7, Name: "operations", LabelEnable: 1, LabelValue: "ops"}).Error)
	require.NoError(t, c.DB.Create(&Target{Ident: "legacy", GroupId: 7, Tags: "env=prod "}).Error)

	err := DoMigrateBg(c, "team")

	require.NoError(t, err)
	var row Target
	require.NoError(t, c.DB.First(&row, "ident = ?", "legacy").Error)
	require.Equal(t, []string{"env=prod", "team=ops"}, row.TagsJSON)
	require.Zero(t, row.GroupId)
}

func TestDoMigrateBg_PreservesExplicitGroupLabel(t *testing.T) {
	c := migrationContext(t)
	require.NoError(t, c.DB.Create(&BusiGroup{Id: 7, Name: "operations", LabelEnable: 1, LabelValue: "ops"}).Error)
	require.NoError(t, c.DB.Create(&Target{Ident: "legacy", GroupId: 7, Tags: "team=custom "}).Error)

	err := DoMigrateBg(c, "team")

	require.NoError(t, err)
	var row Target
	require.NoError(t, c.DB.First(&row, "ident = ?", "legacy").Error)
	require.Equal(t, "team=custom ", row.Tags)
	groups, err := TargetGroupIdsGetByIdent(c, "legacy")
	require.NoError(t, err)
	require.Equal(t, []int64{7}, groups)
}

func TestDoMigrateBg_QueryFailureLeavesLegacyAssignmentsIntact(t *testing.T) {
	for _, table := range []string{"target", "busi_group"} {
		t.Run(table, func(t *testing.T) {
			c := migrationContext(t)
			require.NoError(t, c.DB.Create(&Target{Ident: "legacy", GroupId: 7}).Error)
			failure := errors.New("migration read rejected")
			const callback = "round7:reject_migration_read"
			require.NoError(t, c.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					tx.AddError(failure)
				}
			}))
			t.Cleanup(func() { c.DB.Callback().Query().Remove(callback) })

			err := DoMigrateBg(c, "team")

			require.ErrorIs(t, err, failure)
			require.NoError(t, c.DB.Callback().Query().Remove(callback))
			var row Target
			require.NoError(t, c.DB.First(&row, "ident = ?", "legacy").Error)
			require.Equal(t, int64(7), row.GroupId)
			var bindings int64
			require.NoError(t, c.DB.Model(&TargetBusiGroup{}).Count(&bindings).Error)
			require.Zero(t, bindings)
		})
	}
}

func TestDoMigrateBg_BindingFailurePreservesLegacyGroupForRetry(t *testing.T) {
	c := migrationContext(t)
	require.NoError(t, c.DB.Create(&Target{Ident: "legacy", GroupId: 7, Tags: "env=prod "}).Error)
	failure := errors.New("binding write rejected")
	const callback = "round7:reject_migration_binding"
	require.NoError(t, c.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "target_busi_group" {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { c.DB.Callback().Create().Remove(callback) })

	err := DoMigrateBg(c, "team")

	// Migration is best effort; a failed host must retain its legacy assignment for a later retry.
	require.NoError(t, err)
	var row Target
	require.NoError(t, c.DB.First(&row, "ident = ?", "legacy").Error)
	require.Equal(t, int64(7), row.GroupId)
	require.Equal(t, "env=prod ", row.Tags)
	var bindings int64
	require.NoError(t, c.DB.Model(&TargetBusiGroup{}).Count(&bindings).Error)
	require.Zero(t, bindings)
}
