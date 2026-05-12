package models_test

import (
	"context"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

func newSqliteCtx(t *testing.T) *ctx.Context {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	if err := db.Exec(`CREATE TABLE target (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		group_id INTEGER,
		ident TEXT,
		note TEXT,
		tags TEXT,
		update_at INTEGER,
		host_ip TEXT,
		agent_version TEXT,
		engine_name TEXT,
		os TEXT,
		host_tags TEXT
	)`).Error; err != nil {
		t.Fatalf("create target table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE target_busi_group (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		target_ident TEXT,
		group_id INTEGER,
		update_at INTEGER
	)`).Error; err != nil {
		t.Fatalf("create target_busi_group table: %v", err)
	}

	return ctx.NewContext(context.Background(), db, true)
}

// Reproduces the bug where group_ids op == "==" with only the ungrouped
// sentinel (id == 0) produced a no-placeholder predicate paired with a nil
// value, which GORM's tx.Or(k, nil) silently appended as an extra bind
// variable, generating invalid SQL like `IS NULL?` that fails at execution.
func TestTargetFilterQueryBuild_GroupIDs_OnlyUngrouped(t *testing.T) {
	c := newSqliteCtx(t)

	cases := []struct {
		name string
		op   string
	}{
		{"eq only ungrouped", "=="},
		{"ne only ungrouped", "!="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := models.GetHostsQuery([]models.HostQuery{{
				Key:    "group_ids",
				Op:     tc.op,
				Values: []interface{}{float64(0)},
			}})
			if _, err := models.TargetCountByFilter(c, query); err != nil {
				t.Fatalf("filter query failed: %v", err)
			}
		})
	}
}

// Sanity-checks that the mixed and only-real branches still execute too.
func TestTargetFilterQueryBuild_GroupIDs_OtherBranches(t *testing.T) {
	c := newSqliteCtx(t)

	cases := []struct {
		name   string
		op     string
		values []interface{}
	}{
		{"eq mixed", "==", []interface{}{float64(0), float64(1)}},
		{"eq only real", "==", []interface{}{float64(1), float64(2)}},
		{"ne mixed", "!=", []interface{}{float64(0), float64(1)}},
		{"ne only real", "!=", []interface{}{float64(1), float64(2)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := models.GetHostsQuery([]models.HostQuery{{
				Key:    "group_ids",
				Op:     tc.op,
				Values: tc.values,
			}})
			if _, err := models.TargetCountByFilter(c, query); err != nil {
				t.Fatalf("filter query failed: %v", err)
			}
		})
	}
}

// Empty group_ids must be a no-op rather than producing `group_id IN ()`,
// which MySQL/GORM would reject. The filter is skipped entirely and the
// outer query returns all targets unconstrained by this clause.
func TestGetHostsQuery_GroupIDs_EmptyValuesSkipped(t *testing.T) {
	for _, op := range []string{"==", "!="} {
		t.Run(op, func(t *testing.T) {
			out := models.GetHostsQuery([]models.HostQuery{{
				Key:    "group_ids",
				Op:     op,
				Values: []interface{}{},
			}})
			if len(out) != 0 {
				t.Fatalf("expected empty query slice when values are empty, got %#v", out)
			}
		})
	}
}

// End-to-end check that mixed `!=` correctly filters with real
// target_busi_group rows. Documents the LEFT JOIN contract: A is in
// {3,4}, B is in {1,3}, C is ungrouped. `!= [0,1]` should match only A:
// C is excluded by IS NOT NULL, B is excluded by NOT EXISTS over group 1,
// and A appears exactly once thanks to the outer DISTINCT.
func TestTargetFilterQueryBuild_GroupIDs_NotEqualMixed_RealRows(t *testing.T) {
	c := newSqliteCtx(t)
	db := models.DB(c)

	if err := db.Exec(`INSERT INTO target (ident) VALUES ('a'),('b'),('c')`).Error; err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if err := db.Exec(`INSERT INTO target_busi_group (target_ident, group_id) VALUES
		('a',3),('a',4),
		('b',1),('b',3)`).Error; err != nil {
		t.Fatalf("seed target_busi_group: %v", err)
	}

	query := models.GetHostsQuery([]models.HostQuery{{
		Key:    "group_ids",
		Op:     "!=",
		Values: []interface{}{float64(0), float64(1)},
	}})

	session := models.TargetFilterQueryBuild(c, query, 0, 0)
	var idents []string
	if err := session.Order("ident").Pluck("ident", &idents).Error; err != nil {
		t.Fatalf("execute filter query: %v", err)
	}
	if len(idents) != 1 || idents[0] != "a" {
		t.Fatalf("expected only ident=a, got %v", idents)
	}
}

// Guards against regressions in how TargetFilterQueryBuild renders a
// no-placeholder predicate: the rendered SQL must not contain a stray "?"
// inside the IS NULL / IS NOT NULL clause.
func TestTargetFilterQueryBuild_NoStrayBinding(t *testing.T) {
	c := newSqliteCtx(t)

	for _, op := range []string{"==", "!="} {
		query := models.GetHostsQuery([]models.HostQuery{{
			Key:    "group_ids",
			Op:     op,
			Values: []interface{}{float64(0)},
		}})
		session := models.TargetFilterQueryBuild(c, query, 0, 0)
		stmt := session.Session(&gorm.Session{DryRun: true}).Find(&[]models.Target{}).Statement
		sql := stmt.SQL.String()
		if op == "==" {
			if !strings.Contains(sql, "target_busi_group.target_ident IS NULL") {
				t.Fatalf("expected IS NULL predicate in SQL: %s", sql)
			}
			if strings.Contains(sql, "IS NULL?") {
				t.Fatalf("stray placeholder rendered next to IS NULL: %s", sql)
			}
		} else {
			if !strings.Contains(sql, "target_busi_group.target_ident IS NOT NULL") {
				t.Fatalf("expected IS NOT NULL predicate in SQL: %s", sql)
			}
			if strings.Contains(sql, "IS NOT NULL?") {
				t.Fatalf("stray placeholder rendered next to IS NOT NULL: %s", sql)
			}
		}
	}
}
