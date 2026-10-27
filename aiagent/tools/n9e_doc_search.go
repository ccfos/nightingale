package tools

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/skill/embedded"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/toolkits/pkg/logger"
)

func init() {
	register(defs.SearchN9eDocs, searchN9eDocs)
}

const (
	// 文档站。内嵌页面的 permalink = 站点地址 + 归档内路径去掉 ".md"
	n9eDocSiteURL = "https://n9e.github.io"

	// 内嵌文档的解析上限。现在约 400 页（中英各 ~200）、解压后 ~3 MB，留足余量；
	// 只防构建脚本把错的目录打进来。
	n9eDocMaxPages      = 5000
	n9eDocMaxTotalBytes = 64 << 20

	// 返回给 LLM 的单篇 contents 截断上限（rune）。
	//
	// 之前用 200 rune 的"首次命中 ±100"片段，结果踩坑：V9 API 文档关键信息分散在
	// 不同段落（"修改配置文件"在前面、"X-User-Token Header"在中段、cURL 示例在后段），
	// 200 rune snippet 只截到前面一段，LLM 看不到具体认证 Header 就凭训练记忆
	// 脑补成"Authorization: Bearer"——把答案彻底答反。
	//
	// 6000 rune 能装下整篇 V9 API 文档（~3500 rune），同时不至于让长尾长文档
	// （33KB 那种）撑爆 LLM 上下文。top 3 × 6000 ≈ 18000 rune ≈ 9000 tokens，
	// 在 100k+ context 模型里完全够用。
	n9eDocContentsMaxRunes = 6000

	// 召回质量分级阈值。SCORE_FLOOR 以下的命中视为无效噪声直接过滤；调用方按
	// quality 字段决定是否拒答 / 加置信度提示。
	//
	// 标定：scoreDocEntry 给 title +5、description +3、contents 每次 +1（封顶 3）。
	// 一个 term 的"明确相关"召回至少能拿到 title 5 分或 description 3 分 + 内容 1-3
	// 分, 合起来 ≥5。LOW_CONF_FLOOR=10 对应"title + description 都中"或"title +
	// 多次 contents 命中"的强相关；integration-config boost 单独加 10 也能跨过门槛。
	n9eDocScoreFloor    = 5  // 低于此分丢弃
	n9eDocLowConfFloor  = 10 // 低于此分但 >= floor 标记为 low quality
	n9eDocHighConfFloor = 20 // 大于此分视为 high
)

// docEntry is one searchable document: a docs site page or an integrations/
// sample (see integrations_loader.go).
type docEntry struct {
	Title       string
	Permalink   string
	Description string
	Contents    string

	// pageKey identifies a docs page independent of language ("docs/x" for
	// both docs/x.md and zh/docs/x.md), so search returns one language per
	// page. Empty for integrations entries.
	pageKey string
}

var (
	docIndexMu   sync.RWMutex
	docIndex     []docEntry
	docIndexOnce sync.Once
)

// searchN9eDocs scores entries against caller-supplied keywords and returns
// top N {title, permalink, description, contents}. Idea: the LLM hits this in
// place of the docs site's JS search page, so the response shape mimics what
// that page would show — title + URL + the page text.
//
// Scoring is intentionally dumb (substring + weighted sum). Fancy BM25/TF-IDF
// would barely help for a ~400-page corpus and would mean another dep.
func searchN9eDocs(ctx context.Context, _ *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	keywords := strings.TrimSpace(getArgString(args, "keywords"))
	if keywords == "" {
		return "", fmt.Errorf("keywords is required")
	}
	topN := getArgInt(args, "top_n", 3)
	if topN <= 0 {
		topN = 3
	}
	if topN > 10 {
		topN = 10
	}

	ensureDocIndex()
	docIndexMu.RLock()
	entries := docIndex
	docIndexMu.RUnlock()

	if len(entries) == 0 {
		// 不是暂时状态，重试也不会好：直接让调用方走代码通道或拒答
		return "", fmt.Errorf("n9e docs are not available in this build (built without -tags qa_code_embed and no integrations/ directory)")
	}

	// keywords 上面已 TrimSpace 并校验非空, strings.Fields 一定能产出 >=1 个 token,
	// 所以 tokenizeKeywords 这里不会返回空切片, 不再加额外判空。
	terms := tokenizeKeywords(keywords)

	type scored struct {
		idx   int
		score int
	}
	var hits []scored
	for i := range entries {
		// 过滤低分噪声：低于 SCORE_FLOOR 的命中通常是无关词偶然匹配
		// (如 "通知" 二字在某个跟用户问题完全无关的文档里出现一次)。
		// 让这些噪声进入 top_n 反而会污染 LLM 上下文。
		if s := scoreDocEntry(&entries[i], terms); s >= n9eDocScoreFloor {
			hits = append(hits, scored{i, s})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	// 同一页的中英两份常同时命中（PromQL、配置项名等两种语言里写法一样），
	// 只留得分高的那份，免得 top_n 被同一页占掉两个位置。
	seen := make(map[string]struct{}, len(hits))
	uniq := hits[:0]
	for _, h := range hits {
		if k := entries[h.idx].pageKey; k != "" {
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
		}
		uniq = append(uniq, h)
	}
	hits = uniq
	if len(hits) > topN {
		hits = hits[:topN]
	}

	maxScore := 0
	items := make([]map[string]interface{}, 0, len(hits))
	for _, h := range hits {
		if h.score > maxScore {
			maxScore = h.score
		}
		e := entries[h.idx]
		items = append(items, map[string]interface{}{
			"title":       e.Title,
			"permalink":   e.Permalink,
			"description": e.Description,
			"contents":    truncateRunes(e.Contents, n9eDocContentsMaxRunes),
			"score":       h.score,
			"source":      classifyEntrySource(&e),
		})
	}
	quality := classifyDocResultQuality(len(hits), maxScore)
	mustRefuse := quality == "empty"
	logger.Debugf("search_n9e_docs: keywords=%q terms=%v top_n=%d hits=%d returned=%d max_score=%d quality=%s",
		keywords, terms, topN, len(hits), len(items), maxScore, quality)

	payload, _ := json.Marshal(map[string]interface{}{
		"total":       len(items),
		"items":       items,
		"max_score":   maxScore,
		"quality":     quality,    // "empty" | "low" | "ok" | "high"
		"must_refuse": mustRefuse, // quality == "empty" 时为 true，调用方应触发拒答
	})
	return string(payload), nil
}

// classifyDocResultQuality 按 hit count 和 top1 分数把召回质量分四档：
//
//	high   max_score >= 20         强召回（多 term 都在 title/description 命中）
//	ok     10 <= max_score < 20    中等召回（title+description 双命中或 title 多次命中）
//	low    SCORE_FLOOR <= max_score < 10  弱召回（title 单命中、或仅 description/contents 命中）
//	empty  hits == 0 或所有 hit < SCORE_FLOOR    无有效召回，触发拒答
//
// 打分参考 scoreDocEntry: title +5、description +3、contents 每次 +1（封顶 3）。
// 单 term 仅 title 命中 = 5 分即 low 下限; title+desc+contents 全中 = 11 分进入 ok。
//
// SKILL.md 和限制版 GC handler 都按这四档决定行为：
//   - high/ok: 正常依据 contents 回答
//   - low:    允许回答但建议加"以下信息基于弱召回"警示
//   - empty:  禁止凭记忆补全产品特定标识符，必须按拒答模板回复
func classifyDocResultQuality(hitCount, maxScore int) string {
	if hitCount == 0 || maxScore < n9eDocScoreFloor {
		return "empty"
	}
	if maxScore < n9eDocLowConfFloor {
		return "low"
	}
	if maxScore < n9eDocHighConfFloor {
		return "ok"
	}
	return "high"
}

// tokenizeKeywords splits the keyword string on whitespace and dedups,
// lowercase. User confirmed: just whitespace, no fancy CJK segmentation.
func tokenizeKeywords(s string) []string {
	fields := strings.Fields(strings.ToLower(s))
	seen := make(map[string]struct{}, len(fields))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if _, dup := seen[f]; dup {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	return out
}

// scoreDocEntry: title +5, description +3, contents +1 per hit (capped at 3
// per term so a 30 KB doc that mentions the term 50 times doesn't crowd out
// a focused short doc).
//
// Plus: when the query carries "configuration intent" (mentions categraf /
// 配置 / toml / instances / input 等), [integration-config] entries get a
// flat +10 boost. Reason: README + 文档站 + 整篇 .toml 在 contents
// 命中次数上很容易压过短小但权威的 toml 样例; 配置语法翻车 (q001/q046/q064
// 的 [[inputs.xxx]]) 就是这种"被淹没"的产物. 用确定性加权把权威样本顶到前面.
func scoreDocEntry(e *docEntry, terms []string) int {
	title := strings.ToLower(e.Title)
	desc := strings.ToLower(e.Description)
	contents := strings.ToLower(e.Contents)
	score := 0
	for _, t := range terms {
		if strings.Contains(title, t) {
			score += 5
		}
		if strings.Contains(desc, t) {
			score += 3
		}
		if c := strings.Count(contents, t); c > 0 {
			if c > 3 {
				c = 3
			}
			score += c
		}
	}
	if isConfigQuery(terms) && strings.HasPrefix(e.Title, integrationConfigTitlePrefix) {
		score += 10
	}
	return score
}

// classifyEntrySource returns a short tag describing where a docEntry came
// from, so the LLM can prioritize integration-config samples for "how to
// configure" questions. Kept as a separate function (not on the struct) so
// scoring stays a pure function of (entry, terms).
func classifyEntrySource(e *docEntry) string {
	switch {
	case strings.HasPrefix(e.Title, integrationConfigTitlePrefix):
		return "integration-config"
	case strings.HasPrefix(e.Title, integrationDocTitlePrefix):
		return "integration-doc"
	default:
		return "n9e-docs"
	}
}

// isConfigQuery detects whether the user is asking about configuration syntax
// (in which case [integration-config] toml samples should rank highest).
// Keep the trigger list short — any false positive just nudges a true positive
// to be visible too, not harmful.
func isConfigQuery(terms []string) bool {
	for _, t := range terms {
		switch t {
		case "categraf", "配置", "config", "toml", "instances", "input",
			"采集", "插件", "writer", "writers", "heartbeat":
			return true
		}
	}
	return false
}

// truncateRunes returns s if its rune length is <= max, otherwise the first
// max runes plus a trailing ellipsis. Used to cap doc contents handed to the
// LLM so a single 30 KB outlier doc can't blow the context budget.
func truncateRunes(s string, max int) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// ensureDocIndex builds the index on first tool invocation — keeps the
// parse cost (~400 pages, a few hundred ms) off deployments that never use
// this skill. The pages are embedded in the binary, so there is no network
// fetch and nothing to refresh: the docs a build answers from are the docs
// that existed when it was released.
func ensureDocIndex() {
	docIndexOnce.Do(func() {
		var entries []docEntry
		docCount := 0
		if data := embedded.DocsTarball(); len(data) > 0 {
			docs, err := parseDocsTarball(data)
			if err != nil {
				logger.Warningf("n9e docs: parse embedded pages failed: %v", err)
			}
			entries = append(entries, docs...)
			docCount = len(docs)
		} else {
			logger.Warningf("n9e docs: no docs embedded in this build (built without -tags qa_code_embed); search_n9e_docs only covers integrations/")
		}

		// 合并 integrations/ 下的配置样例和 README。部署里没有 integrations/
		// 目录时就只有内嵌文档撑底。
		integrationCount := 0
		if extras, err := loadIntegrationsEntries(); err != nil {
			logger.Warningf("integrations: load failed, continuing with embedded docs only: %v", err)
		} else {
			entries = append(entries, extras...)
			integrationCount = len(extras)
		}

		docIndexMu.Lock()
		docIndex = entries
		docIndexMu.Unlock()
		logger.Infof("n9e doc index loaded: %d entries (docs %d, integrations %d)",
			len(entries), docCount, integrationCount)
	})
}

// setDocIndexForTest replaces the index and marks it loaded, so tests never
// fall through to the embedded pages.
func setDocIndexForTest(entries []docEntry) {
	docIndexOnce.Do(func() {})
	docIndexMu.Lock()
	docIndex = entries
	docIndexMu.Unlock()
}

// parseDocsTarball turns the embedded docs.tar.gz into one docEntry per page.
// The archive holds the site's rendered Markdown copies: docs/<id>.md (en)
// and zh/docs/<id>.md (zh), published next to each HTML page at the same path,
// so the page URL is simply the file path minus ".md".
func parseDocsTarball(data []byte) ([]docEntry, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var entries []docEntry
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return entries, nil
		}
		if err != nil {
			return entries, fmt.Errorf("read tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		if len(entries) >= n9eDocMaxPages {
			return entries, fmt.Errorf("more than %d pages, bump n9eDocMaxPages", n9eDocMaxPages)
		}
		total += hdr.Size
		if total > n9eDocMaxTotalBytes {
			return entries, fmt.Errorf("pages exceed %d bytes, bump n9eDocMaxTotalBytes", n9eDocMaxTotalBytes)
		}
		raw, err := io.ReadAll(tr)
		if err != nil {
			return entries, fmt.Errorf("read %s: %w", name, err)
		}
		if e, ok := parseDocPage(name, string(raw)); ok {
			entries = append(entries, e)
		}
	}
}

// heading anchors the site appends to every heading, e.g.
// `## Foo[\u200b](#foo "Direct link to Foo")` — pure noise for search and LLM.
var docHeadingAnchorRe = regexp.MustCompile(`\[\x{200B}\]\(#[^)]*\)`)

// in-page links point at the Markdown copies (https://n9e.github.io/docs/x.md);
// rewritten to the HTML page so links the LLM passes on open normally.
var docMarkdownLinkRe = regexp.MustCompile(`(https://n9e\.github\.io/[^\s()"#]*?)\.md\b`)

// parseDocPage reads one rendered page. Its layout is fixed by the site build:
//
//	# <title>
//
//	<description paragraph>
//
//	<body…>
//
// Generated category pages (docs/concepts.md and friends) carry a breadcrumb
// list above the title; stripDocBreadcrumb drops it first.
//
// name is the archive path ("docs/alerting/metric-rules.md" or
// "zh/docs/alerting/metric-rules.md").
func parseDocPage(name, raw string) (docEntry, bool) {
	route := strings.TrimSuffix(name, ".md")
	key := strings.TrimPrefix(route, "zh/")
	if !strings.HasPrefix(key, "docs") {
		return docEntry{}, false
	}

	text := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	text = docHeadingAnchorRe.ReplaceAllString(text, "")
	text = stripDocBreadcrumb(text)
	text = docMarkdownLinkRe.ReplaceAllString(text, "$1")

	var title, desc string
	if strings.HasPrefix(text, "# ") {
		line, rest, _ := strings.Cut(text, "\n")
		title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
		text = strings.TrimSpace(rest)
		// 标题下第一段是 frontmatter 里的 description；代码块 / 小标题不是
		if para, rest, _ := strings.Cut(text, "\n\n"); para != "" &&
			!strings.HasPrefix(para, "#") && !strings.HasPrefix(para, "```") && !strings.Contains(para, "\n") {
			desc = strings.TrimSpace(para)
			text = strings.TrimSpace(rest)
		}
	}
	if title == "" && text == "" {
		return docEntry{}, false
	}
	return docEntry{
		Title:       title,
		Permalink:   n9eDocSiteURL + "/" + route,
		Description: desc,
		Contents:    text,
		pageKey:     key,
	}, true
}

// stripDocBreadcrumb drops a leading breadcrumb ("* [](…/index.md)\n* Section")
// that the site puts above the "# title" of generated category pages. Only
// list items may precede the title; anything else leaves the text untouched.
func stripDocBreadcrumb(text string) string {
	if !strings.HasPrefix(text, "* ") {
		return text
	}
	rest := text
	for strings.HasPrefix(rest, "* ") {
		_, after, ok := strings.Cut(rest, "\n")
		if !ok {
			return text
		}
		rest = strings.TrimLeft(after, "\n")
	}
	if !strings.HasPrefix(rest, "# ") {
		return text
	}
	return rest
}
