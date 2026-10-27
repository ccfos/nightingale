package tools

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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
	setDocIndexForTest([]docEntry{
		{Title: "告警规则模板", Permalink: "https://example/a", Description: "创建告警规则模板", Contents: "告警规则 promql 触发"},
		{Title: "无关条目", Permalink: "https://example/b", Description: "data source", Contents: "随便写点"},
		// /c: description +3, contents +3 (3 次命中, 封顶 3) = 6 分, 过 floor
		{Title: "告警 入门", Permalink: "https://example/c", Description: "alerting basics 告警规则", Contents: "告警规则 部署 告警规则 告警规则"},
	})
	defer setDocIndexForTest(nil)

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

func TestSearchN9eDocsUnavailable(t *testing.T) {
	setDocIndexForTest(nil)

	_, err := searchN9eDocs(context.Background(), nil, map[string]interface{}{"keywords": "x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "not available in this build") {
		t.Errorf("expected not-available error, got %v", err)
	}
}

func TestSearchN9eDocsDedupsLanguages(t *testing.T) {
	// 同一页的中英两份都命中时只返回得分高的一份；integrations 条目没有 pageKey，不参与去重
	setDocIndexForTest([]docEntry{
		{Title: "Metric rules", Permalink: "https://n9e.github.io/docs/alerting/metric-rules", Description: "PromQL threshold", Contents: "promql", pageKey: "docs/alerting/metric-rules"},
		{Title: "指标规则 PromQL", Permalink: "https://n9e.github.io/zh/docs/alerting/metric-rules", Description: "PromQL 阈值", Contents: "promql", pageKey: "docs/alerting/metric-rules"},
		{Title: "[integration-doc] PromQL", Permalink: "https://example/i1", Contents: "promql"},
		{Title: "[integration-doc] PromQL 2", Permalink: "https://example/i2", Contents: "promql"},
	})
	defer setDocIndexForTest(nil)

	out, err := searchN9eDocs(context.Background(), nil, map[string]interface{}{"keywords": "promql", "top_n": float64(10)}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		Items []struct {
			Permalink string `json:"permalink"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	var got []string
	for _, it := range resp.Items {
		got = append(got, it.Permalink)
	}
	want := []string{
		"https://n9e.github.io/zh/docs/alerting/metric-rules", // title 也命中, 分更高
		"https://example/i1",
		"https://example/i2",
	}
	if !equalStrSlice(got, want) {
		t.Errorf("permalinks = %v, want %v", got, want)
	}
}

func TestParseDocPage(t *testing.T) {
	page := "# Metric rules\n\nMetric rules put the threshold inside the PromQL expression.\n\nLast reviewed 2026-10-10\n\n" +
		"## The query list[\u200b](#the-query-list \"Direct link to The query list\")\n\nStep 3 holds a list.\n"
	e, ok := parseDocPage("docs/alerting/metric-rules.md", page)
	if !ok {
		t.Fatal("page should parse")
	}
	if e.Title != "Metric rules" {
		t.Errorf("title = %q", e.Title)
	}
	if e.Description != "Metric rules put the threshold inside the PromQL expression." {
		t.Errorf("description = %q", e.Description)
	}
	if e.Permalink != "https://n9e.github.io/docs/alerting/metric-rules" {
		t.Errorf("permalink = %q", e.Permalink)
	}
	if e.pageKey != "docs/alerting/metric-rules" {
		t.Errorf("pageKey = %q", e.pageKey)
	}
	if strings.Contains(e.Contents, "Direct link") || !strings.Contains(e.Contents, "## The query list\n") {
		t.Errorf("heading anchor not stripped: %q", e.Contents)
	}
	if strings.Contains(e.Contents, "threshold inside") {
		t.Errorf("description should not be repeated in contents: %q", e.Contents)
	}

	zh, ok := parseDocPage("zh/docs/alerting/metric-rules.md", "# 指标规则\n\n指标规则把阈值写在 PromQL 里。\n\n正文")
	if !ok || zh.Permalink != "https://n9e.github.io/zh/docs/alerting/metric-rules" || zh.pageKey != "docs/alerting/metric-rules" {
		t.Errorf("zh page = %+v, ok=%v", zh, ok)
	}

	// 分类页标题上面有面包屑
	cat, _ := parseDocPage("docs/concepts.md", "* [](https://n9e.github.io/index.md)\n* Architecture and concepts\n\n# Architecture and concepts\n\nThe mental model.\n\n## [System architecture](x)")
	if cat.Title != "Architecture and concepts" || cat.Description != "The mental model." || strings.Contains(cat.Contents, "index.md") {
		t.Errorf("breadcrumb not stripped: %+v", cat)
	}
	// 正文就是列表、后面没有标题时原样保留
	if got := stripDocBreadcrumb("* a\n* b\n\ntext"); got != "* a\n* b\n\ntext" {
		t.Errorf("non-breadcrumb list changed: %q", got)
	}

	// 站内 .md 链接改回 HTML 页地址，锚点保留
	link, _ := parseDocPage("docs/x.md", "# X\n\nAbout.\n\nSee [A](https://n9e.github.io/docs/a.md) and [B](https://n9e.github.io/zh/docs/b.md#sec), not [C](https://example.com/c.md).")
	if want := "See [A](https://n9e.github.io/docs/a) and [B](https://n9e.github.io/zh/docs/b#sec), not [C](https://example.com/c.md)."; link.Contents != want {
		t.Errorf("links = %q, want %q", link.Contents, want)
	}

	// 首段是代码块时不当 description
	code, _ := parseDocPage("docs/x.md", "# X\n\n```\nfoo\n```\n\nbar")
	if code.Description != "" || !strings.HasPrefix(code.Contents, "```") {
		t.Errorf("code block taken as description: %+v", code)
	}

	// 站点根页 docs.md -> /docs；docs/ 以外的路径（如集成页）不收
	if root, ok := parseDocPage("docs.md", "# Docs\n\nAll docs."); !ok || root.Permalink != "https://n9e.github.io/docs" {
		t.Errorf("root page = %+v, ok=%v", root, ok)
	}
	if _, ok := parseDocPage("integrations/Linux.md", "# Linux"); ok {
		t.Error("non-docs page should be skipped")
	}
}

func TestParseDocsTarball(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	add := func(name, body string) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	add("docs/a.md", "# A\n\nAbout A.\n\nbody")
	add("zh/docs/a.md", "# 甲\n\n关于甲。\n\n正文")
	add("docs/img.png", "binary")
	tw.Close()
	gz.Close()

	entries, err := parseDocsTarball(buf.Bytes())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].Permalink != "https://n9e.github.io/docs/a" || entries[1].Permalink != "https://n9e.github.io/zh/docs/a" {
		t.Errorf("permalinks = %q, %q", entries[0].Permalink, entries[1].Permalink)
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
