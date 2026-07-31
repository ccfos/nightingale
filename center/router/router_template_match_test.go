package router

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func sorted(ss []string) []string {
	out := append([]string{}, ss...)
	sort.Strings(out)
	return out
}

func TestExtractMetricNames(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want []string
	}{
		{
			name: "plain metric with matcher",
			expr: `redis_uptime_in_seconds{instance=~"$instance"}`,
			want: []string{"redis_uptime_in_seconds"},
		},
		{
			name: "rate with grafana rate interval variable",
			expr: `rate(redis_total_commands_processed{instance=~"$instance"}[$__rate_interval])`,
			want: []string{"redis_total_commands_processed"},
		},
		{
			name: "template variable range vector",
			expr: `avg without (cpu) (irate(node_cpu_seconds_total{mode="steal"}[$interval])) * 100`,
			want: []string{"node_cpu_seconds_total"},
		},
		{
			name: "name via __name__ matcher",
			expr: `{__name__="cpu_usage_active", cpu="cpu-total"}`,
			want: []string{"cpu_usage_active"},
		},
		{
			name: "alert prom_ql",
			expr: `redis_ping_use_seconds > 0.1`,
			want: []string{"redis_ping_use_seconds"},
		},
		{
			name: "unparseable falls back to lexical, label values excluded",
			expr: `sum(rate(consul_raft_apply{job="consul_server"}[1m])) * $factor`,
			want: []string{"consul_raft_apply"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sorted(extractMetricNames(tc.expr))
			if !reflect.DeepEqual(got, sorted(tc.want)) {
				t.Errorf("extractMetricNames(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestCollectExprStrings(t *testing.T) {
	dashboard := `{
		"name": "Redis Overview",
		"configs": {
			"panels": [
				{"targets": [{"expr": "redis_uptime_in_seconds{instance=~\"$instance\"}"}]},
				{"targets": [{"expr": "rate(redis_keyspace_hits[5m])"}, {"expr": ""}]},
				{"type": "row"}
			]
		}
	}`
	var doc interface{}
	if err := json.Unmarshal([]byte(dashboard), &doc); err != nil {
		t.Fatal(err)
	}
	var exprs []string
	collectExprStrings(doc, &exprs)
	if len(exprs) != 2 {
		t.Fatalf("collectExprStrings dashboard = %v, want 2 exprs", exprs)
	}

	alert := `{"name": "Redis Down", "rule_config": {"queries": [{"prom_ql": "redis_uptime_in_seconds < 600"}]}}`
	var doc2 interface{}
	if err := json.Unmarshal([]byte(alert), &doc2); err != nil {
		t.Fatal(err)
	}
	var exprs2 []string
	collectExprStrings(doc2, &exprs2)
	if len(exprs2) != 1 || exprs2[0] != "redis_uptime_in_seconds < 600" {
		t.Fatalf("collectExprStrings alert = %v, want the prom_ql", exprs2)
	}
}

func TestPickSentinels(t *testing.T) {
	noShared := func(string) bool { return false }

	t.Run("uptime metric wins despite lower count", func(t *testing.T) {
		metrics := map[string]int{
			"redis_uptime_in_seconds":        1,
			"redis_total_commands_processed": 8,
			"redis_used_memory":              5,
			"redis_connected_clients":        3,
		}
		got := pickSentinels(metrics, noShared, 3)
		if len(got) != 3 || got[0] != "redis_uptime_in_seconds" {
			t.Errorf("pickSentinels = %v, want uptime first and 3 items", got)
		}
	})

	t.Run("blacklisted metrics never selected", func(t *testing.T) {
		metrics := map[string]int{
			"up":                       10,
			"go_goroutines":            9,
			"process_open_fds":         8,
			"scrape_duration_seconds":  7,
			"nginx_active_connections": 2,
		}
		got := pickSentinels(metrics, noShared, 3)
		if !reflect.DeepEqual(got, []string{"nginx_active_connections"}) {
			t.Errorf("pickSentinels = %v, want only nginx_active_connections", got)
		}
	})

	t.Run("shared metrics excluded, relax keeps one", func(t *testing.T) {
		allShared := func(string) bool { return true }
		metrics := map[string]int{"cpu_usage_active": 5, "mem_used_percent": 3}
		got := pickSentinels(metrics, allShared, 3)
		if len(got) != 1 {
			t.Errorf("pickSentinels relaxed = %v, want exactly 1 weak sentinel", got)
		}
	})

	t.Run("empty in empty out", func(t *testing.T) {
		if got := pickSentinels(map[string]int{}, noShared, 3); len(got) != 0 {
			t.Errorf("pickSentinels(empty) = %v, want empty", got)
		}
	})
}

func TestChunkSentinels(t *testing.T) {
	t.Run("空输入返回空", func(t *testing.T) {
		if got := chunkSentinels(nil, 100); len(got) != 0 {
			t.Errorf("chunkSentinels(nil) = %v, want empty", got)
		}
	})

	t.Run("不超长时单批", func(t *testing.T) {
		got := chunkSentinels([]string{"aaa", "bbb", "ccc"}, 100)
		if len(got) != 1 || len(got[0]) != 3 {
			t.Errorf("chunkSentinels = %v, want 1 batch of 3", got)
		}
	})

	t.Run("每批 join 后不超过 maxLen 且不丢项", func(t *testing.T) {
		// 模拟真实规模：800 个 25 字符哨兵，单条会到 ~20KB（实测触发 VM 422）
		var in []string
		for i := 0; i < 800; i++ {
			in = append(in, fmt.Sprintf("component_metric_name_%03d", i))
		}
		batches := chunkSentinels(in, tplMatchMaxSelectorLen)
		if len(batches) < 2 {
			t.Fatalf("期望切成多批，实际 %d 批", len(batches))
		}
		total := 0
		for i, b := range batches {
			joined := strings.Join(b, "|")
			if len(joined) > tplMatchMaxSelectorLen {
				t.Errorf("第 %d 批 join 后 %d 字符，超过上限 %d", i, len(joined), tplMatchMaxSelectorLen)
			}
			total += len(b)
		}
		if total != len(in) {
			t.Errorf("切批后共 %d 项，输入 %d 项，有丢失", total, len(in))
		}
	})

	t.Run("单项超长时独占一批不丢弃", func(t *testing.T) {
		long := strings.Repeat("x", 200)
		got := chunkSentinels([]string{"a", long, "b"}, 50)
		total := 0
		for _, b := range got {
			total += len(b)
		}
		if total != 3 {
			t.Errorf("切批后共 %d 项，want 3（超长项也要保留）", total)
		}
	})
}

func TestTplMatchIndexMatch(t *testing.T) {
	idx := &tplMatchIndex{
		entries: []*tplComponentEntry{
			{
				ComponentID: 1, Component: "Redis",
				Dashboards: []tplDashboard{
					{UUID: 11, Name: "redis_by_categraf", Sentinels: []string{"redis_uptime_in_seconds"}},
					{UUID: 12, Name: "redis_by_exporter", Sentinels: []string{"redis_up"}},
				},
				AlertGroups: []tplAlertGroup{
					{Cate: "redis_by_categraf", Sentinels: []string{"redis_uptime_in_seconds"}, Rules: []tplPayloadBrief{{UUID: 21, Name: "Redis Down"}}},
				},
			},
			{
				ComponentID: 2, Component: "Nginx",
				Dashboards: []tplDashboard{
					{UUID: 31, Name: "nginx", Sentinels: []string{"nginx_active_connections"}},
				},
			},
		},
	}

	hit := map[string]struct{}{"redis_uptime_in_seconds": {}}
	got := idx.match(hit)

	if len(got) != 1 || got[0].Component != "Redis" {
		t.Fatalf("match = %+v, want only Redis", got)
	}
	// categraf 变体命中、exporter 变体不出现 —— 变体自动选择的核心断言
	if len(got[0].Dashboards) != 1 || got[0].Dashboards[0].UUID != 11 {
		t.Errorf("dashboards = %+v, want only categraf variant", got[0].Dashboards)
	}
	if len(got[0].AlertGroups) != 1 || len(got[0].AlertGroups[0].Rules) != 1 {
		t.Errorf("alert groups = %+v, want categraf group with its rules", got[0].AlertGroups)
	}

	if got2 := idx.match(map[string]struct{}{}); len(got2) != 0 {
		t.Errorf("match(no hits) = %+v, want empty", got2)
	}
}
