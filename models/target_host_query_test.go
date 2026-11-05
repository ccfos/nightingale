package models_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// TestFilterTargetsByHostQueriesMatchesSQL 保证 edge 的内存过滤与中心的 SQL 过滤结果一致
func TestFilterTargetsByHostQueriesMatchesSQL(t *testing.T) {
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
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.Target{}, &models.TargetBusiGroup{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	c := &ctx.Context{DB: db, IsCenter: true}

	targets := []struct {
		ident    string
		tags     string
		hostTags []string
		groups   []int64
	}{
		{"web-01", "env=prod app=web ", []string{"region=bj"}, []int64{1}},
		{"web-02", "env=test app=web ", nil, []int64{1, 2}},
		{"db-01", "env=prod app=db ", []string{"region=sh", "Role=Master"}, []int64{2}},
		{"db_02", "", []string{"region=bj"}, []int64{3}},
		{"cache-01", "env=prod ", nil, nil},
		{"cache-02", "", []string{}, nil},
	}
	for _, tg := range targets {
		if err := db.Create(&models.Target{Ident: tg.ident, Tags: tg.tags, HostTags: tg.hostTags}).Error; err != nil {
			t.Fatalf("create target: %v", err)
		}
		for _, gid := range tg.groups {
			if err := db.Create(&models.TargetBusiGroup{TargetIdent: tg.ident, GroupId: gid}).Error; err != nil {
				t.Fatalf("create binding: %v", err)
			}
		}
	}

	// edge 的 TargetCache 来自中心 /v1/n9e/targets 的 JSON，Tags 字段（json:"-"）不会下发
	all, err := models.TargetGetsAll(c)
	if err != nil {
		t.Fatalf("get all targets: %v", err)
	}
	b, _ := json.Marshal(all)
	var edgeTargets []*models.Target
	if err := json.Unmarshal(b, &edgeTargets); err != nil {
		t.Fatalf("unmarshal targets: %v", err)
	}

	q := func(key, op string, values ...interface{}) models.HostQuery {
		return models.HostQuery{Key: key, Op: op, Values: values}
	}
	cases := []struct {
		name    string
		queries []models.HostQuery
	}{
		{"empty", nil},
		{"group == real", []models.HostQuery{q("group_ids", "==", float64(1))}},
		{"group == multi", []models.HostQuery{q("group_ids", "==", float64(1), float64(3))}},
		{"group == ungrouped", []models.HostQuery{q("group_ids", "==", float64(0))}},
		{"group == mixed", []models.HostQuery{q("group_ids", "==", float64(0), float64(2))}},
		{"group != real", []models.HostQuery{q("group_ids", "!=", float64(1))}},
		{"group != ungrouped", []models.HostQuery{q("group_ids", "!=", float64(0))}},
		{"group != mixed", []models.HostQuery{q("group_ids", "!=", float64(0), float64(2))}},
		{"group invalid skipped", []models.HostQuery{q("group_ids", "==", "x")}},
		{"two group filters on same row", []models.HostQuery{
			q("group_ids", "==", float64(1)), q("group_ids", "==", float64(2)),
		}},
		{"tags == one", []models.HostQuery{q("tags", "==", "env=prod")}},
		{"tags == any of", []models.HostQuery{q("tags", "==", "app=db", "region=bj")}},
		{"tags == case insensitive", []models.HostQuery{q("tags", "==", "role=master")}},
		{"tags != ", []models.HostQuery{q("tags", "!=", "env=prod", "region=sh")}},
		// DB 中 tags 末尾带空格，_ 能匹配到它
		{"tags == trailing underscore", []models.HostQuery{q("tags", "==", "app=web_")}},
		{"hosts ==", []models.HostQuery{q("hosts", "==", "web-01", "db-01", "nope")}},
		{"hosts !=", []models.HostQuery{q("hosts", "!=", "web-01")}},
		{"hosts =~", []models.HostQuery{q("hosts", "=~", "web*", "cache-0*")}},
		{"hosts =~ underscore wildcard", []models.HostQuery{q("hosts", "=~", "db_0*")}},
		{"hosts !~", []models.HostQuery{q("hosts", "!~", "web*", "*01")}},
		{"unknown op", []models.HostQuery{q("hosts", "in", "web-01")}},
		{"combined", []models.HostQuery{
			q("group_ids", "==", float64(1), float64(2)),
			q("tags", "==", "env=prod"),
			q("hosts", "!~", "web*"),
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var dbLst []*models.Target
			session := models.TargetFilterQueryBuild(c, models.GetHostsQuery(tc.queries), 0, 0)
			if err := session.Find(&dbLst).Error; err != nil {
				t.Fatalf("sql filter: %v", err)
			}
			want := identsOf(dbLst)
			lst, err := models.FilterTargetsByHostQueries(edgeTargets, tc.queries)
			if err != nil {
				t.Fatalf("memory filter: %v", err)
			}
			got := identsOf(lst)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("memory filter = %v, sql filter = %v", got, want)
			}
		})
	}
}

// 超长的筛选值无法编译成正则时返回 error，不能 panic 让 edge 进程退出
func TestFilterTargetsByHostQueriesOversizedPattern(t *testing.T) {
	huge := strings.Repeat("a_", 2_000_000)
	targets := []*models.Target{{Ident: "web-01", TagsJSON: []string{"env=prod"}}}
	for _, key := range []string{"hosts", "tags"} {
		t.Run(key, func(t *testing.T) {
			op := "=="
			if key == "hosts" {
				op = "=~"
			}
			_, err := models.FilterTargetsByHostQueries(targets, []models.HostQuery{{Key: key, Op: op, Values: []interface{}{huge}}})
			if err == nil {
				t.Fatal("expected error for oversized pattern")
			}
			if len(err.Error()) > 200 {
				t.Fatalf("error message should not carry the pattern, got %d chars", len(err.Error()))
			}
		})
	}
}

func identsOf(lst []*models.Target) []string {
	res := make([]string, 0, len(lst))
	for _, t := range lst {
		res = append(res, t.Ident)
	}
	sort.Strings(res)
	return res
}
