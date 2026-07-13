package models_test

import (
	"context"
	"fmt"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// newUserNicknameCtx builds an in-memory users table seeded so that:
//   - alice has a nickname
//   - bob exists but has an empty nickname
//   - carol is absent entirely
func newUserNicknameCtx(t *testing.T) *ctx.Context {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	if err := db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT,
		nickname TEXT,
		roles TEXT
	)`).Error; err != nil {
		t.Fatalf("create users table: %v", err)
	}

	if err := db.Exec(`INSERT INTO users (username, nickname, roles) VALUES
		('alice', 'Alice A', ''),
		('bob', '', '')`).Error; err != nil {
		t.Fatalf("seed users: %v", err)
	}

	return ctx.NewContext(context.Background(), db, true)
}

func TestNicknameOrName(t *testing.T) {
	nm := map[string]string{"alice": "Alice A", "bob": ""}

	cases := []struct {
		username string
		want     string
	}{
		{"alice", "Alice A"}, // has nickname
		{"bob", "bob"},       // exists but empty nickname -> username
		{"carol", "carol"},   // absent from map -> username
		{"", ""},             // no username -> stays empty
	}
	for _, tc := range cases {
		if got := models.NicknameOrName(nm, tc.username); got != tc.want {
			t.Errorf("NicknameOrName(%q) = %q, want %q", tc.username, got, tc.want)
		}
	}
}

// FillUpdateByNicknames resolves the nickname and falls back to the username when the
// user has no nickname or does not exist, so update_by_nickname is never blank for a
// known operator.
func TestFillUpdateByNicknames_FallsBackToUsername(t *testing.T) {
	c := newUserNicknameCtx(t)

	type item struct {
		UpdateBy         string
		UpdateByNickname string
	}
	items := []*item{
		{UpdateBy: "alice"},
		{UpdateBy: "bob"},
		{UpdateBy: "carol"},
		{UpdateBy: ""},
	}

	models.FillUpdateByNicknames(c, items)

	want := []string{"Alice A", "bob", "carol", ""}
	for i, it := range items {
		if it.UpdateByNickname != want[i] {
			t.Errorf("items[%d].UpdateByNickname = %q, want %q", i, it.UpdateByNickname, want[i])
		}
	}
}

// FillCreateByNicknames keys off CreateBy/CreateByNickname (not UpdateBy); this guards
// against a silent no-op should the reflected field names drift.
func TestFillCreateByNicknames_FallsBackToUsername(t *testing.T) {
	c := newUserNicknameCtx(t)

	type item struct {
		CreateBy         string
		CreateByNickname string
	}
	items := []*item{
		{CreateBy: "alice"},
		{CreateBy: "bob"},
		{CreateBy: "carol"},
	}

	models.FillCreateByNicknames(c, items)

	want := []string{"Alice A", "bob", "carol"}
	for i, it := range items {
		if it.CreateByNickname != want[i] {
			t.Errorf("items[%d].CreateByNickname = %q, want %q", i, it.CreateByNickname, want[i])
		}
	}
}
