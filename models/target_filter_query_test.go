package models_test

import (
	"context"
	"reflect"
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

func TestGetHostsQuery_EmptyConditionsSkipped(t *testing.T) {
	for _, filter := range []models.HostQuery{
		{Key: "hosts", Op: "!~"},
		{Key: "hosts", Op: "=~"},
		{Key: "tags", Op: "!="},
		{Key: "unknown", Op: "=="},
		{Key: "hosts", Op: "unknown"},
	} {
		for _, values := range []struct {
			name string
			list []interface{}
		}{
			{"nil", nil},
			{"empty", []interface{}{}},
			{"nil element", []interface{}{nil}},
		} {
			t.Run(filter.Key+"/"+filter.Op+"/"+values.name, func(t *testing.T) {
				filter.Values = values.list
				out := models.GetHostsQuery([]models.HostQuery{filter})
				if len(out) != 0 {
					t.Fatalf("expected no conditions, got %#v", out)
				}
			})
		}
	}
	for _, filter := range []models.HostQuery{
		{Key: "unknown", Op: "==", Values: []interface{}{"a"}},
		{Key: "hosts", Op: "unknown", Values: []interface{}{"a"}},
	} {
		t.Run(filter.Key+"/"+filter.Op+"/nonempty", func(t *testing.T) {
			if out := models.GetHostsQuery([]models.HostQuery{filter}); len(out) != 0 {
				t.Fatalf("expected no conditions, got %#v", out)
			}
		})
	}
}

func TestTargetFilterQueryBuild_EmptyConditionsSkipped(t *testing.T) {
	c := newSqliteCtx(t)
	if err := models.DB(c).Exec(`INSERT INTO target (ident) VALUES ('a'),('b'),('c')`).Error; err != nil {
		t.Fatalf("seed target: %v", err)
	}

	for _, filter := range []models.HostQuery{
		{Key: "hosts", Op: "!~"},
		{Key: "tags", Op: "!="},
	} {
		for _, values := range []struct {
			name string
			list []interface{}
		}{
			{"nil", nil},
			{"empty", []interface{}{}},
		} {
			for _, restricted := range []bool{false, true} {
				name := filter.Key + "/" + filter.Op + "/" + values.name
				if restricted {
					name += "/with equality"
				}
				t.Run(name, func(t *testing.T) {
					filter.Values = values.list
					filters := []models.HostQuery{filter}
					want := []string{"a", "b", "c"}
					if restricted {
						filters = append(filters, models.HostQuery{
							Key: "hosts", Op: "==", Values: []interface{}{"a", "b"},
						})
						want = []string{"a", "b"}
					}
					query := models.GetHostsQuery(filters)
					session := models.TargetFilterQueryBuild(c, query, 0, 0)
					var idents []string
					if err := session.Order("ident").Pluck("ident", &idents).Error; err != nil {
						t.Fatalf("execute filter query: %v", err)
					}
					if !reflect.DeepEqual(idents, want) {
						t.Fatalf("expected idents %v, got %v", want, idents)
					}
				})
			}
		}
	}
}

func TestTargetFilterQueryBuild_Tags_EmptyEqualityMatchesNothing(t *testing.T) {
	c := newSqliteCtx(t)
	if err := models.DB(c).Exec(`INSERT INTO target (ident, tags, host_tags) VALUES
		('a','env=prod',NULL),('b','','env=prod'),('c','env=dev','env=dev')`).Error; err != nil {
		t.Fatalf("seed target: %v", err)
	}

	for _, values := range []struct {
		name string
		list []interface{}
	}{
		{"nil", nil},
		{"empty", []interface{}{}},
		{"all nil", []interface{}{nil, nil}},
	} {
		for _, restricted := range []bool{false, true} {
			name := values.name
			if restricted {
				name += "/with host equality"
			}
			t.Run(name, func(t *testing.T) {
				filters := []models.HostQuery{{Key: "tags", Op: "==", Values: values.list}}
				if restricted {
					filters = append(filters, models.HostQuery{
						Key: "hosts", Op: "==", Values: []interface{}{"a", "b"},
					})
				}
				query := models.GetHostsQuery(filters)
				if len(query) != len(filters) || !reflect.DeepEqual(query[0], map[string]interface{}{"1 = 0": nil}) {
					t.Fatalf("expected an explicit false condition for empty tag equality, got %#v", query)
				}
				session := models.TargetFilterQueryBuild(c, query, 0, 0)
				stmt := session.Session(&gorm.Session{DryRun: true}).Find(&[]models.Target{}).Statement
				if sql := stmt.SQL.String(); !strings.Contains(sql, "1 = 0") {
					t.Fatalf("expected false predicate in SQL: %s", sql)
				}
				var idents []string
				if err := session.Order("ident").Pluck("ident", &idents).Error; err != nil {
					t.Fatalf("execute filter query: %v", err)
				}
				if len(idents) != 0 {
					t.Fatalf("expected no targets for empty tag equality, got %v", idents)
				}
			})
		}
	}
}

func TestTargetFilterQueryBuild_HostsAndTags_Regression(t *testing.T) {
	c := newSqliteCtx(t)
	if err := models.DB(c).Exec(`INSERT INTO target (ident, tags, host_tags) VALUES
		('a','env=prod',NULL),('b','','env=prod'),('c','env=dev','env=dev')`).Error; err != nil {
		t.Fatalf("seed target: %v", err)
	}

	cases := []struct {
		name   string
		key    string
		op     string
		values []interface{}
		want   []string
	}{
		{"hosts eq", "hosts", "==", []interface{}{"a", "b"}, []string{"a", "b"}},
		{"hosts ne", "hosts", "!=", []interface{}{"a", "b"}, []string{"c"}},
		{"hosts like", "hosts", "=~", []interface{}{"a*", "b*"}, []string{"a", "b"}},
		{"hosts not like", "hosts", "!~", []interface{}{"a*"}, []string{"b", "c"}},
		{"hosts not like multiple", "hosts", "!~", []interface{}{"a*", "b*"}, []string{"c"}},
		{"tags eq", "tags", "==", []interface{}{"env=prod"}, []string{"a", "b"}},
		{"tags ne", "tags", "!=", []interface{}{"env=prod"}, []string{"c"}},
		{"tags ne multiple", "tags", "!=", []interface{}{"env=prod", "env=dev"}, []string{}},
		{"hosts eq empty", "hosts", "==", []interface{}{}, []string{}},
		{"hosts eq nil", "hosts", "==", nil, []string{}},
		{"hosts ne empty", "hosts", "!=", []interface{}{}, []string{}},
		{"hosts ne nil", "hosts", "!=", nil, []string{}},
		{"hosts not like blank string", "hosts", "!~", []interface{}{""}, []string{"a", "b", "c"}},
		{"tags ne blank string", "tags", "!=", []interface{}{""}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := models.GetHostsQuery([]models.HostQuery{{Key: tc.key, Op: tc.op, Values: tc.values}})
			if len(query) != 1 || len(query[0]) == 0 {
				t.Fatalf("expected a nonempty condition, got %#v", query)
			}
			session := models.TargetFilterQueryBuild(c, query, 0, 0)
			if len(tc.values) == 0 {
				stmt := session.Session(&gorm.Session{DryRun: true}).Find(&[]models.Target{}).Statement
				predicate := "ident in (NULL)"
				if tc.op == "!=" {
					predicate = "ident not in (NULL)"
				}
				sql := session.Dialector.Explain(stmt.SQL.String(), stmt.Vars...)
				if !strings.Contains(sql, predicate) {
					t.Fatalf("expected %q in SQL: %s", predicate, sql)
				}
			}
			var idents []string
			if err := session.Order("ident").Pluck("ident", &idents).Error; err != nil {
				t.Fatalf("execute filter query: %v", err)
			}
			if !reflect.DeepEqual(idents, tc.want) {
				t.Fatalf("expected idents %v, got %v", tc.want, idents)
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
