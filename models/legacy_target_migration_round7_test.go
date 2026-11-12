package models

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDoMigrateBg_QueryFailureLeavesLegacyAssignmentsIntact(t *testing.T) {
	for _, table := range []string{"target", "busi_group"} {
		t.Run(table, func(t *testing.T) {
			c := pipelineContext(t, &Target{}, &TargetBusiGroup{}, &BusiGroup{})
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
	c := pipelineContext(t, &Target{}, &TargetBusiGroup{}, &BusiGroup{})
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
