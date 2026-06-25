package models_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

func newEsIndexPatternDB(t *testing.T) *gorm.DB {
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

	if err := db.Exec(`CREATE TABLE es_index_pattern (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		datasource_id INTEGER,
		name TEXT,
		time_field TEXT,
		allow_hide_system_indices INTEGER,
		fields_format TEXT,
		cross_cluster_enabled INTEGER,
		note TEXT,
		weight INTEGER,
		create_at INTEGER,
		create_by TEXT,
		update_at INTEGER,
		update_by TEXT
	)`).Error; err != nil {
		t.Fatalf("create es_index_pattern table: %v", err)
	}
	return db
}

// EsIndexPatternGets 应按 weight 升序返回，weight 相同时再按 id 升序，
// 保证管理列表页与日志查询选择器里的展示顺序稳定可控。
func TestEsIndexPatternGets_OrderByWeight(t *testing.T) {
	db := newEsIndexPatternDB(t)
	c := ctx.NewContext(context.Background(), db, true)

	rows := []struct {
		id     int64
		name   string
		weight int
	}{
		{id: 1, name: "a", weight: 30},
		{id: 2, name: "b", weight: 10},
		{id: 3, name: "c", weight: 10}, // 与 id=2 同权重，id 升序应排在其后
		{id: 4, name: "d", weight: 20},
	}
	for _, r := range rows {
		if err := db.Exec(
			"INSERT INTO es_index_pattern (id, datasource_id, name, time_field, weight) VALUES (?, ?, ?, ?, ?)",
			r.id, 1, r.name, "@timestamp", r.weight,
		).Error; err != nil {
			t.Fatalf("insert %s: %v", r.name, err)
		}
	}

	lst, err := models.EsIndexPatternGets(c, "")
	if err != nil {
		t.Fatalf("EsIndexPatternGets: %v", err)
	}

	got := make([]string, 0, len(lst))
	for _, p := range lst {
		got = append(got, p.Name)
	}
	want := []string{"b", "c", "d", "a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order mismatch: got %v want %v", got, want)
	}
}

// EsIndexPatternUpdateWeights 批量改权重后，列表应按新权重重新排序，且只影响 weight 列。
func TestEsIndexPatternUpdateWeights(t *testing.T) {
	db := newEsIndexPatternDB(t)
	c := ctx.NewContext(context.Background(), db, true)

	// 三条初始 weight 均为 0
	for _, r := range []struct {
		id   int64
		name string
	}{{1, "a"}, {2, "b"}, {3, "c"}} {
		if err := db.Exec(
			"INSERT INTO es_index_pattern (id, datasource_id, name, time_field, weight) VALUES (?, ?, ?, ?, ?)",
			r.id, 1, r.name, "@timestamp", 0,
		).Error; err != nil {
			t.Fatalf("insert %s: %v", r.name, err)
		}
	}

	// 批量更新：c 最前、a 居中、b 最后
	if err := models.EsIndexPatternUpdateWeights(c, []models.EsIndexPatternWeight{
		{Id: 3, Weight: 10},
		{Id: 1, Weight: 20},
		{Id: 2, Weight: 30},
	}); err != nil {
		t.Fatalf("EsIndexPatternUpdateWeights: %v", err)
	}

	lst, err := models.EsIndexPatternGets(c, "")
	if err != nil {
		t.Fatalf("EsIndexPatternGets: %v", err)
	}
	got := make([]string, 0, len(lst))
	for _, p := range lst {
		got = append(got, p.Name)
	}
	want := []string{"c", "a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order after batch update mismatch: got %v want %v", got, want)
	}

	// 空入参应是无副作用的 no-op
	if err := models.EsIndexPatternUpdateWeights(c, nil); err != nil {
		t.Fatalf("EsIndexPatternUpdateWeights(nil): %v", err)
	}
}
