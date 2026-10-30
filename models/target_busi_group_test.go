package models_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// TestTargetOverrideBgidsEmptyBumpsUpdateAt covers resetting a target to no
// busi group: the target row itself is unchanged, so update_at must be bumped
// for TargetCache (and everything synced from it) to drop the old GroupIds.
// Repeating the reset (heartbeats with overwrite_gids and an empty gid) must
// not bump it again, or every cycle would force a full target reload.
func TestTargetOverrideBgidsEmptyBumpsUpdateAt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1) // keep the single in-memory database across transactions
	if err := db.AutoMigrate(&models.Target{}, &models.TargetBusiGroup{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	c := &ctx.Context{DB: db, IsCenter: true}

	if err := db.Create(&models.Target{Ident: "host-1", UpdateAt: 100}).Error; err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := db.Create(&models.TargetBusiGroup{TargetIdent: "host-1", GroupId: 1, UpdateAt: 100}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}

	updateAt := func() int64 {
		var tgt models.Target
		if err := db.Where("ident = ?", "host-1").First(&tgt).Error; err != nil {
			t.Fatalf("get target: %v", err)
		}
		return tgt.UpdateAt
	}

	if err := models.TargetOverrideBgids(c, []string{"host-1"}, nil, nil); err != nil {
		t.Fatalf("override to empty: %v", err)
	}
	var bindings int64
	db.Model(&models.TargetBusiGroup{}).Where("target_ident = ?", "host-1").Count(&bindings)
	if bindings != 0 {
		t.Fatalf("bindings = %d, want 0", bindings)
	}
	bumped := updateAt()
	if bumped <= 100 {
		t.Fatalf("update_at = %d, want bumped past 100 after clearing groups", bumped)
	}

	// Already ungrouped: nothing changes, so update_at must stay put.
	if err := db.Model(&models.Target{}).Where("ident = ?", "host-1").Update("update_at", 200).Error; err != nil {
		t.Fatalf("reset update_at: %v", err)
	}
	if err := models.TargetOverrideBgids(c, []string{"host-1"}, nil, nil); err != nil {
		t.Fatalf("override to empty again: %v", err)
	}
	if got := updateAt(); got != 200 {
		t.Fatalf("update_at = %d, want 200 unchanged when no binding was removed", got)
	}
}
