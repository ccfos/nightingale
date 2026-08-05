package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/toolkits/pkg/runner"
)

// TestMain 先把 docSyncOnce 用掉：否则任何一个走 searchN9eDocs 的用例都会顺手
// 起真实的同步 goroutine，跑单测时往 flashcat.cloud 发请求，还会异步改写
// docIndex 把别的用例搞挂。需要触发路径的用例自己用 resetDocSyncOnce 复位。
func TestMain(m *testing.M) {
	docSyncOnce.Do(func() {})
	os.Exit(m.Run())
}

// resetDocSyncOnce 让 triggerDocIndexSync 在本用例里能真正执行一次，
// 用例结束后重新置为「已消费」，不影响其他用例。
func resetDocSyncOnce(t *testing.T) {
	t.Helper()
	docSyncOnce = sync.Once{}
	t.Cleanup(func() {
		docSyncOnce = sync.Once{}
		docSyncOnce.Do(func() {})
	})
}

// resetDocSyncConfig 装载进程级同步配置，用例结束后恢复默认（开启 + 官方地址）。
func resetDocSyncConfig(t *testing.T, disabled bool, indexURL string) {
	t.Helper()
	InitDocIndexSync(disabled, indexURL)
	t.Cleanup(func() { InitDocIndexSync(false, "") })
}

// resetDocIndexState 清空索引与同步状态，用例结束后再清一次。
func resetDocIndexState(t *testing.T) {
	t.Helper()
	clear := func() {
		docIndexMu.Lock()
		docIndex, docIndexLoaded, docSyncDisabled, docSyncFails = nil, false, false, 0
		docIndexMu.Unlock()
	}
	clear()
	t.Cleanup(clear)
}

// withIntegrationsFixture 造一个最小的 integrations/ 目录并把 runner.Cwd 指过去，
// 模拟「断网但本地有采集配置样例」的部署。
func withIntegrationsFixture(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	comp := filepath.Join(dir, "integrations", "mysql")
	mustMkdir(t, filepath.Join(comp, "markdown"))
	mustMkdir(t, filepath.Join(comp, "collect", "conf"))
	mustWrite(t, filepath.Join(comp, "markdown", "README.md"), "# MySQL\n\nMySQL 采集插件说明\n")
	mustWrite(t, filepath.Join(comp, "collect", "conf", "mysql.toml"),
		"# mysql 采集配置\n[[instances]]\naddress = \"127.0.0.1:3306\"\n")

	prev := runner.Cwd
	runner.Cwd = dir
	t.Cleanup(func() { runner.Cwd = prev })
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
}

func mustWrite(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

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
	resetDocIndexState(t)

	_, err := searchN9eDocs(context.Background(), nil, map[string]interface{}{"keywords": "x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "warming up") {
		t.Errorf("expected warming-up error, got %v", err)
	}
}

// 同步被配置关掉、本地又没有语料时，给 LLM 的必须是「已关闭 + 别重试」，
// 而不是含糊的「预热中，稍后重试」——后者会让模型在一轮对话里反复空转。
func TestSearchN9eDocsSyncDisabledMessage(t *testing.T) {
	resetDocIndexState(t)
	docIndexMu.Lock()
	docSyncDisabled = true
	docIndexMu.Unlock()

	_, err := searchN9eDocs(context.Background(), nil, map[string]interface{}{"keywords": "x"}, nil)
	if err == nil {
		t.Fatal("expected an error when the index is empty and sync is disabled")
	}
	if strings.Contains(err.Error(), "warming up") {
		t.Errorf("disabled sync must not report a transient warming-up state: %v", err)
	}
	for _, want := range []string{"turned off by config", "DisableDocIndexSync", "do not retry"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message should contain %q, got %v", want, err)
		}
	}
}

func TestDocIndexUnavailableErr(t *testing.T) {
	// 远程持续失败也是稳态：到阈值后同样要让模型停手
	terminal := docIndexUnavailableErr(false, n9eDocSyncFailsTerminal)
	if !strings.Contains(terminal.Error(), "do not retry") {
		t.Errorf("terminal failure should tell the model to stop retrying, got %v", terminal)
	}
	// 阈值以下按预热处理，允许模型稍后再试
	warming := docIndexUnavailableErr(false, n9eDocSyncFailsTerminal-1)
	if !strings.Contains(warming.Error(), "warming up") {
		t.Errorf("below the threshold should still read as warming up, got %v", warming)
	}
}

func TestDocSyncRetryDelay(t *testing.T) {
	cases := []struct {
		fails int
		want  time.Duration
	}{
		{1, n9eDocSyncRetryDelay},
		{2, 2 * n9eDocSyncRetryDelay},
		{3, 4 * n9eDocSyncRetryDelay},
		{100, n9eDocSyncRetryMaxDelay}, // 封顶，且不会因移位溢出成负数
	}
	for _, c := range cases {
		if got := docSyncRetryDelay(c.fails); got != c.want {
			t.Errorf("docSyncRetryDelay(%d) = %s, want %s", c.fails, got, c.want)
		}
	}
	if got := docSyncRetryDelay(100); got > n9eDocSyncRetryMaxDelay {
		t.Errorf("delay must never exceed the cap, got %s", got)
	}
}

func TestInitDocIndexSync(t *testing.T) {
	resetDocSyncConfig(t, false, "")
	if disabled, u := docSyncConfig(); disabled || u != n9eDocIndexURL {
		t.Errorf("defaults should be sync-on + official URL, got disabled=%v url=%s", disabled, u)
	}
	mirror := "http://10.1.2.3/index.json"
	InitDocIndexSync(false, mirror)
	if _, u := docSyncConfig(); u != mirror {
		t.Errorf("configured mirror should win, got %s", u)
	}
	// 清空地址要退回官方地址，而不是留着上一次的镜像
	InitDocIndexSync(true, "")
	if disabled, u := docSyncConfig(); !disabled || u != n9eDocIndexURL {
		t.Errorf("empty URL should fall back to the official one, got disabled=%v url=%s", disabled, u)
	}
}

// 关闭远程同步后：不出网，但本地 integrations 语料照样建索引，检索可用。
//
// 这里刻意用 deps=nil 触发（ProcessorAdapter 那条路构造 Agent 时不注入
// ToolDeps）：配置是进程级的，谁第一个调用工具都必须照它执行，否则「彻底停止
// 出网」的承诺会被第一个不带配置的调用方废掉。
func TestTriggerDocIndexSyncDisabledUsesLocalCorpus(t *testing.T) {
	resetDocIndexState(t)
	resetDocSyncOnce(t)
	withIntegrationsFixture(t)
	// 地址指向必然连不通的端口：一旦走了远程分支，索引不可能被填上，用例即失败
	resetDocSyncConfig(t, true, "http://127.0.0.1:1/index.json")

	out, err := searchN9eDocs(context.Background(), nil, map[string]interface{}{"keywords": "mysql 配置"}, nil)
	if err != nil {
		t.Fatalf("search should work off the local corpus: %v", err)
	}
	if !strings.Contains(out, "[[instances]]") {
		t.Errorf("expected the local categraf sample in the result, got %s", out)
	}

	docIndexMu.RLock()
	loaded, disabled, n := docIndexLoaded, docSyncDisabled, len(docIndex)
	docIndexMu.RUnlock()
	if !disabled {
		t.Error("docSyncDisabled should be recorded so the tool can explain itself")
	}
	if !loaded || n == 0 {
		t.Fatalf("local integrations corpus should still populate the index, loaded=%v entries=%d", loaded, n)
	}

	// 后续来自 router 的调用（带 ToolDeps）不得让同步「复活」——docSyncOnce
	// 已消费，配置依旧是关闭态。
	if _, err := searchN9eDocs(context.Background(), &aiagent.ToolDeps{},
		map[string]interface{}{"keywords": "mysql 配置"}, nil); err != nil {
		t.Fatalf("second call should keep working off the local corpus: %v", err)
	}
	docIndexMu.RLock()
	stillDisabled := docSyncDisabled
	docIndexMu.RUnlock()
	if !stillDisabled {
		t.Error("a later call must not flip the process back to remote sync")
	}
}

// 镜像地址里的凭据（userinfo / query token）不能进日志或错误。
func TestRedactIndexURL(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"https://flashcat.cloud/index.json", "https://flashcat.cloud/index.json"},
		{"https://user:s3cret@mirror.intra/index.json", "https://mirror.intra/index.json"},
		{"https://mirror.intra/index.json?token=abc#frag", "https://mirror.intra/index.json"},
		{"://bad url", "(unparsable doc index url)"},
	}
	for _, c := range cases {
		if got := redactIndexURL(c.raw); got != c.want {
			t.Errorf("redactIndexURL(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

// 拉取失败时返回的 error 会进日志，必须已经脱敏 —— net/http 只抹 password，
// query 里的 token 会原样留在 *url.Error 里。
func TestFetchRemoteDocIndexErrorIsRedacted(t *testing.T) {
	_, err := fetchRemoteDocIndex("http://user:s3cret@127.0.0.1:1/index.json?token=t0ken")
	if err == nil {
		t.Fatal("expected a connection failure")
	}
	for _, leak := range []string{"s3cret", "t0ken"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("credential %q leaked into the error: %v", leak, err)
		}
	}
}

// 远程拉取失败但本地有语料：索引必须建起来（错误照常返回给退避逻辑），
// 而不是像以前那样直接 return 导致检索工具永远停在「预热中」。
func TestRefreshDocIndexRemoteFailureFallsBackToLocalCorpus(t *testing.T) {
	resetDocIndexState(t)
	withIntegrationsFixture(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := refreshDocIndex(srv.URL)
	if err == nil {
		t.Fatal("remote failure must still be reported so the caller backs off")
	}
	docIndexMu.RLock()
	loaded, n := docIndexLoaded, len(docIndex)
	docIndexMu.RUnlock()
	if !loaded || n == 0 {
		t.Fatalf("index should be served from the local corpus, loaded=%v entries=%d", loaded, n)
	}

	// 第二次刷新仍然失败：已有索引不能被半量结果覆盖
	if err := refreshDocIndex(srv.URL); err == nil {
		t.Error("second failure should still be reported")
	}
	docIndexMu.RLock()
	stillLoaded, still := docIndexLoaded, len(docIndex)
	docIndexMu.RUnlock()
	if !stillLoaded || still != n {
		t.Errorf("existing index must survive a failed refresh: loaded=%v entries=%d (was %d)", stillLoaded, still, n)
	}
}

// 远程可用时：条目与本地语料合并，旧版本文档被过滤；顺带覆盖 DocIndexURL
// 指向自建镜像（私网地址）的链路——官方地址之外不该被 SSRF 兜底拦掉。
func TestRefreshDocIndexMergesRemoteAndLocal(t *testing.T) {
	resetDocIndexState(t)
	withIntegrationsFixture(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"title":"告警规则","permalink":"https://x/docs/content/flashcat-monitor/nightingale-v9/alert/","description":"v9","contents":"告警规则说明"},
			{"title":"旧版告警","permalink":"https://x/docs/content/flashcat-monitor/nightingale-v6/alert/","description":"v6","contents":"旧版说明"},
			{"title":"博客","permalink":"https://x/blog/hello/","description":"blog","contents":"noise"}
		]`))
	}))
	defer srv.Close()

	if err := refreshDocIndex(srv.URL); err != nil {
		t.Fatalf("refresh should succeed against the mirror: %v", err)
	}

	docIndexMu.RLock()
	entries := docIndex
	docIndexMu.RUnlock()

	var remote, local int
	for _, e := range entries {
		switch {
		case strings.Contains(e.Permalink, "nightingale-v6"), strings.Contains(e.Permalink, "/blog/"):
			t.Errorf("old-version / non-doc entry should have been filtered: %s", e.Permalink)
		case strings.HasPrefix(e.Title, integrationConfigTitlePrefix), strings.HasPrefix(e.Title, integrationDocTitlePrefix):
			local++
		default:
			remote++
		}
	}
	if remote != 1 {
		t.Errorf("want 1 remote entry kept, got %d", remote)
	}
	if local == 0 {
		t.Error("local integrations entries should be merged in as well")
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
