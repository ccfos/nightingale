package models_test

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// memsto's user group cache reloads members only when UserGroupStatistics
// changes, so every path that writes user_group_member on its own has to touch
// the group afterwards or the new member never reaches alert notification.
func TestUserGroupTouchMovesStatistics(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.UserGroup{}, &models.UserGroupMember{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	c := &ctx.Context{DB: db, IsCenter: true}

	ug := &models.UserGroup{Name: "ops", CreateAt: 1000, UpdateAt: 1000, CreateBy: "root", UpdateBy: "root"}
	if err := db.Create(ug).Error; err != nil {
		t.Fatalf("create user group: %v", err)
	}

	before, err := models.UserGroupStatistics(c)
	if err != nil {
		t.Fatalf("statistics before: %v", err)
	}

	if err := models.UserGroupMemberAdd(c, ug.Id, 42); err != nil {
		t.Fatalf("UserGroupMemberAdd: %v", err)
	}

	mid, err := models.UserGroupStatistics(c)
	if err != nil {
		t.Fatalf("statistics after member add: %v", err)
	}
	if mid.Total != before.Total || mid.LastUpdated != before.LastUpdated {
		t.Fatalf("statistics moved on member add alone: %+v -> %+v", before, mid)
	}

	if err := models.UserGroupTouch(c, ug.Id); err != nil {
		t.Fatalf("UserGroupTouch: %v", err)
	}

	after, err := models.UserGroupStatistics(c)
	if err != nil {
		t.Fatalf("statistics after touch: %v", err)
	}
	if after.LastUpdated <= before.LastUpdated {
		t.Fatalf("UserGroupTouch did not bump update_at: %+v -> %+v", before, after)
	}
}

func TestUserGroupTouchNoIds(t *testing.T) {
	if err := models.UserGroupTouch(&ctx.Context{}); err != nil {
		t.Fatalf("UserGroupTouch with no ids: %v", err)
	}
}

// FeiShu login joins its default groups through AddToUserGroups, so the touch
// has to happen there too.
func TestAddToUserGroupsMovesStatistics(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.UserGroup{}, &models.UserGroupMember{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	c := &ctx.Context{DB: db, IsCenter: true}

	for _, name := range []string{"ops", "dev"} {
		ug := &models.UserGroup{Name: name, CreateAt: 1000, UpdateAt: 1000, CreateBy: "root", UpdateBy: "root"}
		if err := db.Create(ug).Error; err != nil {
			t.Fatalf("create user group %s: %v", name, err)
		}
	}

	before, err := models.UserGroupStatistics(c)
	if err != nil {
		t.Fatalf("statistics before: %v", err)
	}

	u := &models.User{Id: 7}
	if err := u.AddToUserGroups(c, []int64{1, 2}); err != nil {
		t.Fatalf("AddToUserGroups: %v", err)
	}

	after, err := models.UserGroupStatistics(c)
	if err != nil {
		t.Fatalf("statistics after: %v", err)
	}
	if after.LastUpdated <= before.LastUpdated {
		t.Fatalf("AddToUserGroups did not bump update_at: %+v -> %+v", before, after)
	}

	ids, err := models.MyGroupIds(c, u.Id)
	if err != nil || len(ids) != 2 {
		t.Fatalf("MyGroupIds=%v err=%v, want two groups", ids, err)
	}
}
