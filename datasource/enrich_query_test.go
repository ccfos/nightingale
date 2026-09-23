package datasource

import (
	"testing"
	"time"
)

func TestNormalizeEnrichTimeRange(t *testing.T) {
	t.Run("interval 推算窗口", func(t *testing.T) {
		before := time.Now().Unix()
		from, to := NormalizeEnrichTimeRange(0, 0, 300, 0)
		after := time.Now().Unix()

		if to < before || to > after {
			t.Fatalf("to=%d 不在 [%d,%d] 内", to, before, after)
		}
		if to-from != 300 {
			t.Fatalf("窗口跨度应为 300，实际 %d", to-from)
		}
	})

	t.Run("interval 缺省按 60 秒", func(t *testing.T) {
		from, to := NormalizeEnrichTimeRange(0, 0, 0, 0)
		if to-from != 60 {
			t.Fatalf("窗口跨度应为 60，实际 %d", to-from)
		}
	})

	t.Run("已有 from/to 时只按 offset 前移", func(t *testing.T) {
		from, to := NormalizeEnrichTimeRange(1000, 2000, 300, 100)
		if from != 900 || to != 1900 {
			t.Fatalf("期望 (900,1900)，实际 (%d,%d)", from, to)
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
