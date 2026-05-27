package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/toolkits/pkg/logger"
)

func init() {
	register(defs.SearchN9eDocs, searchN9eDocs)
}

const (
	n9eDocIndexURL       = "https://flashcat.cloud/index.json"
	n9eDocSyncInterval   = 24 * time.Hour
	n9eDocSyncRetryDelay = 1 * time.Minute
	n9eDocIndexMaxBytes  = 12 << 20 // 12 MiB — index is ~10 MB today, leave headroom

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

// docEntry mirrors the subset of fields we use from /index.json.
type docEntry struct {
	Title       string `json:"title"`
	Permalink   string `json:"permalink"`
	Description string `json:"description"`
	Contents    string `json:"contents"`
}

var (
	docIndexMu     sync.RWMutex
	docIndex       []docEntry
	docIndexLoaded bool

	docSyncOnce sync.Once
)

// searchN9eDocs scores entries against caller-supplied keywords and returns
// top N {title, permalink, description, snippet}. Idea: the LLM hits this in
// place of the JS-rendered flashcat search page, so the response shape mimics
// what that page would show — title + URL + a contextual snippet.
//
// Scoring is intentionally dumb (substring + weighted sum). Fancy BM25/TF-IDF
// would barely help for a 960-doc corpus and would mean another dep.
func searchN9eDocs(ctx context.Context, _ *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	triggerDocIndexSync()

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

	docIndexMu.RLock()
	loaded := docIndexLoaded
	entries := docIndex
	docIndexMu.RUnlock()

	if !loaded {
		return "", fmt.Errorf("n9e doc index is still warming up, please retry in a few seconds")
	}

	terms := tokenizeKeywords(keywords)
	if len(terms) == 0 {
		return "", fmt.Errorf("keywords yielded no usable terms")
	}

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
	sort.Slice(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
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
//	ok     10 <= max_score < 20    中等召回（单 term 在 title/description 多处命中）
//	low    SCORE_FLOOR <= max_score < 10  弱召回（只有 contents 弱命中）
//	empty  hits == 0 或所有 hit < SCORE_FLOOR    无有效召回，触发拒答
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
// flat +10 boost. Reason: README + flashcat 文档站 + 整篇 .toml 在 contents
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

// triggerDocIndexSync spins up the sync loop on first tool invocation —
// keeps the cost off deployments that never use this skill.
func triggerDocIndexSync() {
	docSyncOnce.Do(func() {
		go docIndexSyncLoop()
	})
}

func docIndexSyncLoop() {
	for {
		if err := refreshDocIndex(); err != nil {
			logger.Warningf("sync n9e doc index failed: %v", err)
			if !isDocIndexLoaded() {
				time.Sleep(n9eDocSyncRetryDelay)
				continue
			}
		} else {
			logger.Infof("sync n9e doc index ok, next refresh in %s", n9eDocSyncInterval)
		}
		time.Sleep(n9eDocSyncInterval)
	}
}

func refreshDocIndex() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	transport := &http.Transport{
		DialContext:           safeDialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableKeepAlives:     true,
	}
	client := &http.Client{Timeout: 60 * time.Second, Transport: transport}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n9eDocIndexURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %v", err)
	}
	req.Header.Set("User-Agent", "n9e-aiagent-doc-sync/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http fetch: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, n9eDocIndexMaxBytes+1))
	if err != nil {
		return fmt.Errorf("read body: %v", err)
	}
	if len(raw) > n9eDocIndexMaxBytes {
		return fmt.Errorf("index exceeds %d bytes, bump n9eDocIndexMaxBytes", n9eDocIndexMaxBytes)
	}

	var all []docEntry
	if err := json.Unmarshal(raw, &all); err != nil {
		return fmt.Errorf("parse json: %v", err)
	}

	filtered := make([]docEntry, 0, len(all))
	skippedOld := 0
	for _, e := range all {
		// 只保留文档；landing / blog / changelog 等非文档对'平台使用类问答'是噪音
		if !strings.Contains(e.Permalink, "/docs/content/") {
			continue
		}
		if e.Title == "" && e.Contents == "" {
			continue
		}
		// 过滤旧版 nightingale 文档，只服务 V9 用户。
		// 旧版文档里的 API/UI/字段经常和 V9 不一致，LLM 看了会幻觉外推（实测：
		// 看到 V5 webapi 文档讲 JWT 认证后，给出"V9 Bearer Token"的瞎答）。
		// 与其靠 prompt 让 LLM 守纪律，不如索引层面直接去掉，根除幻觉源。
		if isOldNightingaleDoc(e.Permalink) {
			skippedOld++
			continue
		}
		filtered = append(filtered, e)
	}

	// 合并 integrations/ 下的配置样例和 README。失败不影响远程索引可用性 —
	// 走部署里没 integrations/ 目录的场景就只有远程文档撑底。
	integrationCount := 0
	if extras, ierr := loadIntegrationsEntries(); ierr == nil && len(extras) > 0 {
		filtered = append(filtered, extras...)
		integrationCount = len(extras)
	} else if ierr != nil {
		logger.Warningf("integrations: load failed, continuing with remote index only: %v", ierr)
	}

	docIndexMu.Lock()
	docIndex = filtered
	docIndexLoaded = true
	docIndexMu.Unlock()
	logger.Infof("n9e doc index loaded: %d entries (raw %d bytes, skipped %d old-version, integrations %d)",
		len(filtered), len(raw), skippedOld, integrationCount)
	return nil
}

// isOldNightingaleDoc 判断 permalink 是否指向旧版本 n9e 文档（V5/V6/V7/V8）。
//
// flashcat 文档站的路径约定：
//
//	/flashcat-monitor/nightingale-v6/...   → V6
//	/flashcat-monitor/nightingale-v7/...   → V7
//	/flashcat-monitor/nightingale-v8/...   → V8
//	/flashcat-monitor/nightingale-v9/...   → V9 ⭐ 保留
//	/flashcat-monitor/nightingale/...      → V5（注意：无 -vX 后缀就是 V5，
//	                                           历史遗留命名，introduction 开头会明示）
//	/flashcat-monitor/categraf/...         → 不分版本，保留
//	/flashcat-partner/...                  → 不分版本，保留
//	/flashcat/...                          → flashcat 企业版，保留
//
// 这里只杀 nightingale 旧版本（V5-V8），categraf 等辅助文档不动 —— 它们不带版本
// 漂移问题。如果将来 V10 来了，把 nightingale-v9 改成 nightingale-v10 即可，
// 或扩展成扫描 nightingale-vN 提取数字、动态判断"非当前主版本"。
func isOldNightingaleDoc(permalink string) bool {
	if strings.Contains(permalink, "/flashcat-monitor/nightingale-v6/") ||
		strings.Contains(permalink, "/flashcat-monitor/nightingale-v7/") ||
		strings.Contains(permalink, "/flashcat-monitor/nightingale-v8/") {
		return true
	}
	// V5 路径无版本号后缀：必须用前后双斜杠精确匹配，避免误杀 nightingale-v9。
	// strings.Contains(p, "/nightingale/") 既匹配 .../nightingale/api/...（V5）
	// 也不会匹配 .../nightingale-v9/...（中间是连字符）。
	if strings.Contains(permalink, "/flashcat-monitor/nightingale/") {
		return true
	}
	return false
}

func isDocIndexLoaded() bool {
	docIndexMu.RLock()
	defer docIndexMu.RUnlock()
	return docIndexLoaded
}
