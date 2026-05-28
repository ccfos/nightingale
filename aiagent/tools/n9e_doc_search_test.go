package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTokenizeKeywords(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"  ", nil},
		{"alert rule", []string{"alert", "rule"}},
		{"  alert    rule  ", []string{"alert", "rule"}},
		{"告警 规则", []string{"告警", "规则"}},
		{"AlertRule alert RULE", []string{"alertrule", "alert", "rule"}}, // dedup case-insensitive
		{"a a a b", []string{"a", "b"}},
	}
	for _, c := range cases {
		got := tokenizeKeywords(c.in)
		if !equalStrSlice(got, c.want) {
			t.Errorf("tokenizeKeywords(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestScoreDocEntry(t *testing.T) {
	e := &docEntry{
		Title:       "告警规则模板",
		Description: "如何创建告警规则模板",
		Contents:    "告警规则 是 n9e 的核心。告警规则可以基于 PromQL 配置。告警规则触发时通知。告警规则示例。",
	}
	cases := []struct {
		name  string
		terms []string
		want  int
	}{
		// title=+5, desc=+3, contents counted with cap 3 → +3
		{"hit all fields", []string{"告警规则"}, 5 + 3 + 3},
		// hit only contents (and contents has it 4 times → capped at 3)
		{"contents only no other", []string{"promql"}, 1},
		// no hit
		{"no hit", []string{"unrelated"}, 0},
		// two terms, both hit title+desc+contents (cap)
		{"two hits both everywhere", []string{"告警", "规则"}, (5 + 3 + 3) * 2},
	}
	for _, c := range cases {
		got := scoreDocEntry(e, c.terms)
		if got != c.want {
			t.Errorf("%s: scoreDocEntry = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestScoreDocEntryEmpty(t *testing.T) {
	if got := scoreDocEntry(&docEntry{}, []string{"x"}); got != 0 {
		t.Errorf("empty entry should score 0, got %d", got)
	}
}

func TestTruncateRunes(t *testing.T) {
	// 短内容 — 原样返回
	if got := truncateRunes("短文", 100); got != "短文" {
		t.Errorf("short content should pass through, got %q", got)
	}
	// 空内容 — 空字符串
	if got := truncateRunes("", 100); got != "" {
		t.Errorf("empty content should be empty, got %q", got)
	}
	// 长内容 — 截到 max + 省略号
	long := strings.Repeat("a", 200)
	got := truncateRunes(long, 100)
	if len([]rune(got)) != 101 { // 100 a + 1 省略号
		t.Errorf("truncated length = %d, want 101", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated result should end with …, got %q", got[len(got)-10:])
	}
	// 中文按 rune 截，不会半截
	chinese := strings.Repeat("中", 200)
	got2 := truncateRunes(chinese, 50)
	if len([]rune(got2)) != 51 { // 50 中 + 1 省略号
		t.Errorf("chinese truncated length = %d runes, want 51", len([]rune(got2)))
	}
}

func TestSearchN9eDocsValidation(t *testing.T) {
	// keywords missing
	if _, err := searchN9eDocs(context.Background(), nil, map[string]interface{}{}, nil); err == nil {
		t.Error("expected error for missing keywords")
	}
	// keywords blank
	if _, err := searchN9eDocs(context.Background(), nil, map[string]interface{}{"keywords": "   "}, nil); err == nil {
		t.Error("expected error for blank keywords")
	}
}

func TestSearchN9eDocsRanking(t *testing.T) {
	// 手工塞索引、绕过 sync 路径
	//
	// SCORE_FLOOR=5 的过滤行为要求 /c 必须有 description 命中, 否则只 contents
	// 单次命中 (+1 分) 会被丢弃, top_n=2 ranking 测不到。
	docIndexMu.Lock()
	docIndex = []docEntry{
		{Title: "告警规则模板", Permalink: "https://example/a", Description: "创建告警规则模板", Contents: "告警规则 promql 触发"},
		{Title: "无关条目", Permalink: "https://example/b", Description: "data source", Contents: "随便写点"},
		// /c: description +3, contents +3 (3 次命中, 封顶 3) = 6 分, 过 floor
		{Title: "告警 入门", Permalink: "https://example/c", Description: "alerting basics 告警规则", Contents: "告警规则 部署 告警规则 告警规则"},
	}
	docIndexLoaded = true
	docIndexMu.Unlock()
	defer func() {
		docIndexMu.Lock()
		docIndex = nil
		docIndexLoaded = false
		docIndexMu.Unlock()
	}()

	out, err := searchN9eDocs(context.Background(), nil, map[string]interface{}{
		"keywords": "告警规则",
		"top_n":    float64(2),
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		Total int `json:"total"`
		Items []struct {
			Permalink string `json:"permalink"`
			Score     int    `json:"score"`
			Contents  string `json:"contents"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json unmarshal: %v\nraw=%s", err, out)
	}
	if resp.Total != 2 {
		t.Errorf("want total=2 (top_n clamp), got %d", resp.Total)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].Permalink != "https://example/a" {
		t.Errorf("expected /a first (highest score), got %s", resp.Items[0].Permalink)
	}
	if resp.Items[0].Score <= resp.Items[1].Score {
		t.Errorf("scores not sorted desc: %d, %d", resp.Items[0].Score, resp.Items[1].Score)
	}
	// contents 字段必须返回（而不是 snippet——之前的 200 rune 摘录已废弃，
	// 现在返回完整正文截断到 6000 rune，让 LLM 看全文不脑补）
	if resp.Items[0].Contents == "" {
		t.Error("contents field should be present in result")
	}
	// 检查 /b（无命中）确实没出现
	for _, it := range resp.Items {
		if it.Permalink == "https://example/b" {
			t.Error("zero-score entry should not appear in results")
		}
	}
}

func TestSearchN9eDocsNotLoaded(t *testing.T) {
	docIndexMu.Lock()
	docIndex = nil
	docIndexLoaded = false
	docIndexMu.Unlock()

	_, err := searchN9eDocs(context.Background(), nil, map[string]interface{}{"keywords": "x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "warming up") {
		t.Errorf("expected warming-up error, got %v", err)
	}
}

func TestIsOldNightingaleDoc(t *testing.T) {
	cases := []struct {
		permalink string
		old       bool
	}{
		// V9 — 必须保留
		{"https://flashcat.cloud/docs/content/flashcat-monitor/nightingale-v9/usecase/api/", false},
		{"https://flashcat.cloud/docs/content/flashcat-monitor/nightingale-v9/usage/integrations/", false},
		// V8/V7/V6 — 必须过滤
		{"https://flashcat.cloud/docs/content/flashcat-monitor/nightingale-v8/usecase/api/", true},
		{"https://flashcat.cloud/docs/content/flashcat-monitor/nightingale-v7/install/edge/", true},
		{"https://flashcat.cloud/docs/content/flashcat-monitor/nightingale-v6/api/api/", true},
		// V5 — 无版本号后缀路径，必须过滤
		{"https://flashcat.cloud/docs/content/flashcat-monitor/nightingale/api/webapi/", true},
		{"https://flashcat.cloud/docs/content/flashcat-monitor/nightingale/introduction/", true},
		// 不分版本的辅助文档 — 必须保留
		{"https://flashcat.cloud/docs/content/flashcat-monitor/categraf/2-installation/", false},
		{"https://flashcat.cloud/docs/content/flashcat-partner/prometheus/quickstart/overview/", false},
		{"https://flashcat.cloud/docs/content/flashcat/overview/", false},
		// 边界：路径里碰巧含 nightingale 但不在 flashcat-monitor 下
		{"https://example.com/nightingale-v6/whatever/", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isOldNightingaleDoc(c.permalink); got != c.old {
			t.Errorf("isOldNightingaleDoc(%q) = %v, want %v", c.permalink, got, c.old)
		}
	}
}

func equalStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
