package router

import (
	"strconv"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

// proxy 只读白名单：分享页所需的只读查询路径放行，写/管理端点与非只读方法拒绝
func TestIsReadOnlyProxyPath(t *testing.T) {
	allow := []struct{ method, path string }{
		{"GET", "/api/v1/query"},
		{"POST", "/api/v1/query_range"},
		{"GET", "/api/v1/labels"},
		{"GET", "/api/v1/label/job/values"},
		{"GET", "/api/v1/series"},
		{"GET", "/api/v1/status/buildinfo"},
		{"POST", "/myindex/_search"},
		{"POST", "/_msearch"},
		{"GET", "/select/logsql/query"},
		// 名字里含两个点不是路径穿越，不应被按段判定误伤
		{"POST", "/myindex..old/_search"},
		// ES 版本探测：面板据此决定 date_histogram 用 interval 还是 fixed_interval
		{"GET", "/"},
	}
	for _, c := range allow {
		if !IsReadOnlyProxyPath(c.method, c.path) {
			t.Errorf("expected read-only allow: %s %s", c.method, c.path)
		}
	}

	deny := []struct{ method, path string }{
		{"DELETE", "/myindex/_doc/1"},
		{"PUT", "/myindex/_doc/1"},
		{"POST", "/myindex/_bulk"},
		{"POST", "/_bulk"},
		{"POST", "/myindex/_delete_by_query"},
		{"GET", "/-/reload"},
		{"POST", "/api/v1/admin/tsdb/delete_series"},
		{"GET", "/api/v2/write"},

		// 路径穿越：gin 不 clean URL.Path，%2e%2e 到 c.Param 时已解码成 ..，
		// director 又把这段原样转发给上游。若只做前缀匹配，下面这些会以
		// /api/v1/query 前缀骗过白名单，而上游（前置 nginx 会先归一化）看到的
		// 是 /-/reload 之类白名单外的端点
		{"GET", "/api/v1/query/../../../-/reload"},
		{"POST", "/api/v1/query/../../../api/v1/admin/tsdb/delete_series"},
		{"GET", "/api/v1/query/../.."},
		{"POST", "/myindex/../../_bulk/_search"},
		{"GET", "/select/logsql/../../-/reload"},

		// 根路径只放行 GET：写方法即便打在根上也不该透传
		{"POST", "/"},
		{"DELETE", "/"},
		{"GET", "/../"},
	}
	for _, c := range deny {
		if IsReadOnlyProxyPath(c.method, c.path) {
			t.Errorf("expected deny: %s %s", c.method, c.path)
		}
	}
}

// 板内数据源集合提取：字面量 datasourceValue（含 row 嵌套）、"${var}" 引用
// 经 datasource/datasourceIdentifier 变量按 cate 展开、非法 payload 兜底
func TestCollectBoardDatasourceIds(t *testing.T) {
	expand := func(cate string) []int64 {
		switch cate {
		case "prometheus":
			return []int64{1, 2}
		case "elasticsearch":
			return []int64{7}
		}
		return nil
	}

	tests := []struct {
		name    string
		payload string
		want    []int64
	}{
		{
			name: "literal datasourceValue, flat and nested under row",
			payload: `{
				"panels": [
					{"type": "timeseries", "datasourceValue": 3},
					{"type": "row", "panels": [{"type": "stat", "datasourceValue": 4}]}
				]
			}`,
			want: []int64{3, 4},
		},
		{
			name: "datasource variable expands by cate, literal merged",
			payload: `{
				"var": [{"type": "datasource", "name": "ds", "definition": "prometheus"}],
				"panels": [
					{"type": "timeseries", "datasourceValue": "${ds}"},
					{"type": "logs", "datasourceValue": 9}
				]
			}`,
			want: []int64{1, 2, 9},
		},
		{
			name: "datasourceIdentifier variable expands by cate",
			payload: `{
				"var": [{"type": "datasourceIdentifier", "name": "dsi", "definition": "elasticsearch"}],
				"panels": []
			}`,
			want: []int64{7},
		},
		{
			name:    "query variable does not expand by cate but its literal datasource.value is collected",
			payload: `{"var": [{"type": "query", "name": "q", "definition": "label_values(up, job)", "datasource": {"value": 6}}], "panels": []}`,
			want:    []int64{6},
		},
		{
			name:    "query variable datasource.value as numeric string is collected",
			payload: `{"var": [{"type": "query", "name": "q", "datasource": {"value": "8"}}], "panels": []}`,
			want:    []int64{8},
		},
		{
			name:    "datasource.value as variable reference is skipped",
			payload: `{"var": [{"type": "query", "name": "q", "datasource": {"value": "${ds}"}}], "panels": []}`,
			want:    nil,
		},
		{
			name:    "invalid json yields empty set",
			payload: `{invalid`,
			want:    nil,
		},
		{
			name:    "empty payload yields empty set",
			payload: "  ",
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectBoardDatasourceIds(tt.payload, expand)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v ids, want %v: %v", len(got), len(tt.want), got)
			}
			for _, id := range tt.want {
				if _, ok := got[id]; !ok {
					t.Errorf("missing datasource id %d in %v", id, got)
				}
			}
		})
	}
}

// 分享令牌的作用域判定：只有绑定本板的有效令牌才放行；绑别的板的有效令牌必须
// Deny 而不是 Pass——否则 fall-through 到 boardGet 的 public 判定后，配了「匿名访问」
// 的板会被任意一条有效分享链接顺带打开
func TestJudgeBoardToken(t *testing.T) {
	future := time.Now().Unix() + 3600
	past := time.Now().Unix() - 3600

	cases := []struct {
		name    string
		st      *models.SourceToken
		boardId int64
		want    boardTokenVerdict
	}{
		{
			name:    "令牌查不到（不存在/已注销）→ 按无令牌处理",
			st:      nil,
			boardId: 89,
			want:    boardTokenPass,
		},
		{
			name:    "令牌已过期（即便绑的就是本板）→ 按无令牌处理，保持跳登录页的既有行为",
			st:      &models.SourceToken{SourceId: "89", ExpireAt: past},
			boardId: 89,
			want:    boardTokenPass,
		},
		{
			name:    "令牌已过期且绑别的板 → 仍按无令牌处理，不升级成拒绝",
			st:      &models.SourceToken{SourceId: "89", ExpireAt: past},
			boardId: 16,
			want:    boardTokenPass,
		},
		{
			name:    "有效令牌绑本板 → 放行",
			st:      &models.SourceToken{SourceId: "89", ExpireAt: future},
			boardId: 89,
			want:    boardTokenAllow,
		},
		{
			name:    "永不过期的令牌绑本板 → 放行（expire_at=0 表示不过期）",
			st:      &models.SourceToken{SourceId: "89", ExpireAt: 0},
			boardId: 89,
			want:    boardTokenAllow,
		},
		{
			name:    "有效令牌绑别的板 → 拒绝（本次修复的核心用例）",
			st:      &models.SourceToken{SourceId: "89", ExpireAt: future},
			boardId: 16,
			want:    boardTokenDeny,
		},
		{
			name:    "永不过期的令牌绑别的板 → 同样拒绝",
			st:      &models.SourceToken{SourceId: "89", ExpireAt: 0},
			boardId: 16,
			want:    boardTokenDeny,
		},
		{
			name:    "source_id 不是数字 → 按不匹配拒绝，不放行",
			st:      &models.SourceToken{SourceId: "abc", ExpireAt: future},
			boardId: 16,
			want:    boardTokenDeny,
		},
		{
			name:    "source_id 为空 → 按不匹配拒绝",
			st:      &models.SourceToken{SourceId: "", ExpireAt: future},
			boardId: 16,
			want:    boardTokenDeny,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := judgeBoardToken(c.st, c.boardId); got != c.want {
				t.Errorf("judgeBoardToken(%+v, %d) = %d, want %d", c.st, c.boardId, got, c.want)
			}
		})
	}
}

// source_id 的存储形态由 sourceTokenAdd 用 strconv.FormatInt 规范化，判定侧按数值
// 比对，两端对齐；这里钉住「规范化后的形态一定能被判定侧还原成同一个 id」
func TestJudgeBoardTokenSourceIdRoundTrip(t *testing.T) {
	for _, id := range []int64{1, 16, 89, 1 << 40} {
		st := &models.SourceToken{SourceId: strconv.FormatInt(id, 10), ExpireAt: time.Now().Unix() + 3600}
		if got := judgeBoardToken(st, id); got != boardTokenAllow {
			t.Errorf("board id %d: got %d, want allow", id, got)
		}
		if got := judgeBoardToken(st, id+1); got != boardTokenDeny {
			t.Errorf("board id %d vs %d: got %d, want deny", id, id+1, got)
		}
	}
}
