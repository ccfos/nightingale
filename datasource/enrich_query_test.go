package datasource

import (
	"testing"
	"time"
)

func TestNormalizeQueryTimeRange(t *testing.T) {
	t.Run("interval 推算窗口", func(t *testing.T) {
		before := time.Now().Unix()
		from, to := NormalizeQueryTimeRange(0, 0, 300, 0)
		after := time.Now().Unix()

		if to < before || to > after {
			t.Fatalf("to=%d 不在 [%d,%d] 内", to, before, after)
		}
		if to-from != 300 {
			t.Fatalf("窗口跨度应为 300，实际 %d", to-from)
		}
	})

	t.Run("没有 interval 也没有 from/to 时不补窗口", func(t *testing.T) {
		// 与 doris 一致：不替调用方编一个窗口，$__timeFilter 仍按空窗口报错
		from, to := NormalizeQueryTimeRange(0, 0, 0, 0)
		if from != 0 || to != 0 {
			t.Fatalf("期望 (0,0)，实际 (%d,%d)", from, to)
		}
	})

	t.Run("已有 from/to 时只按 offset 前移", func(t *testing.T) {
		from, to := NormalizeQueryTimeRange(1000, 2000, 300, 100)
		if from != 900 || to != 1900 {
			t.Fatalf("期望 (900,1900)，实际 (%d,%d)", from, to)
		}
	})

	t.Run("没有 offset 时 from/to 原样返回", func(t *testing.T) {
		// 仪表盘、探索页走的就是这条路
		from, to := NormalizeQueryTimeRange(1000, 2000, 0, 0)
		if from != 1000 || to != 2000 {
			t.Fatalf("期望 (1000,2000)，实际 (%d,%d)", from, to)
		}
	})
}

func TestRowsToStringMaps(t *testing.T) {
	ts := time.Date(2026, 9, 23, 10, 30, 0, 0, time.Local)

	items := []interface{}{
		map[string]interface{}{
			"name":    "web01",
			"bytes":   []byte("raw"),
			"ts":      ts,
			"ok":      true,
			"cnt":     int64(12),
			"big":     float64(12345678901),
			"nested":  map[string]interface{}{"a": 1},
			"ignored": nil,
		},
		map[string]interface{}{"only_nil": nil}, // 整行为空，应被丢掉
		"not a row",                             // 类型不对，应被丢掉
	}

	rows := RowsToStringMaps(items)
	if len(rows) != 1 {
		t.Fatalf("应只剩 1 行，实际 %d 行：%+v", len(rows), rows)
	}

	want := map[string]string{
		"name":   "web01",
		"bytes":  "raw",
		"ts":     "2026-09-23 10:30:00",
		"ok":     "true",
		"cnt":    "12",
		"big":    "12345678901", // 不能是 1.2345678901e+10
		"nested": `{"a":1}`,
	}

	row := rows[0]
	if len(row) != len(want) {
		t.Fatalf("列数不符，期望 %d 实际 %d：%+v", len(want), len(row), row)
	}
	for k, v := range want {
		if row[k] != v {
			t.Errorf("列 %s 期望 %q，实际 %q", k, v, row[k])
		}
	}
}
