package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	n9eDocIndexURL      = "https://flashcat.cloud/index.json"
	n9eDocSyncInterval  = 24 * time.Hour
	n9eDocIndexMaxBytes = 12 << 20 // 12 MiB — index is ~10 MB today, leave headroom

	// 失败重试从 1 分钟起翻倍退避，封顶 1 小时。之前是固定 1 分钟无限重试：
	// 内网/断网部署里 flashcat.cloud 永远拉不通，等于每分钟往日志里塞一条
	// WARNING，把真正要看的日志淹掉。
	n9eDocSyncRetryDelay    = 1 * time.Minute
	n9eDocSyncRetryMaxDelay = 1 * time.Hour

	// 连续失败到这个次数后，索引仍为空就认定是稳态不可用（而非启动预热），
	// 检索工具改口告诉 LLM「别再重试」。见 docIndexUnavailableErr。
	n9eDocSyncFailsTerminal = 2

	// 关闭远程同步的操作提示，同步失败日志里原样给运维看。写成自然语言而不是
	// 伪造成一行配置片段：toml 的表头和键值不能同行，"[Center.AIAgent]
	// DisableDocIndexSync = true" 照抄进去只会让配置解析失败。
	n9eDocSyncDisableHint = "set DisableDocIndexSync = true under [Center.AIAgent] in config.toml"

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
	// docSyncDisabled（配置关掉了远程同步）与 docSyncFails（连续失败次数）只用于
	// 在索引为空时区分「稳态不可用」和「启动预热中」，给 LLM 不同的措辞。
	docSyncDisabled bool
	docSyncFails    int

	docSyncOnce sync.Once
)

// 远程索引同步的进程级配置，由 center/router 在启动期（开始收请求之前）用
// [Center.AIAgent] 初始化，默认与历史行为一致：同步开启 + 官方地址。
//
// 刻意不走 ToolDeps：ProcessorAdapter 之类的路径构造 Agent 时根本不注入
// ToolDeps，谁先调用 search_n9e_docs 谁就会拿零值配置把 docSyncOnce 消费掉——
// 那样配置里的「关闭同步」永远没机会生效，出网也停不掉。
var (
	docSyncCfgMu       sync.RWMutex
	docSyncCfgDisabled bool
	docSyncCfgURL      = n9eDocIndexURL
)

// InitDocIndexSync 配置远程文档索引同步：disabled 为真则彻底不出网，
// indexURL 为空时用官方地址。须在开始接收请求前调用。
func InitDocIndexSync(disabled bool, indexURL string) {
	docSyncCfgMu.Lock()
	defer docSyncCfgMu.Unlock()
	docSyncCfgDisabled = disabled
	docSyncCfgURL = n9eDocIndexURL
	if indexURL != "" {
		docSyncCfgURL = indexURL
	}
}

func docSyncConfig() (disabled bool, indexURL string) {
	docSyncCfgMu.RLock()
	defer docSyncCfgMu.RUnlock()
	return docSyncCfgDisabled, docSyncCfgURL
}

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
	disabled, fails := docSyncDisabled, docSyncFails
	docIndexMu.RUnlock()

	if !loaded {
		return "", docIndexUnavailableErr(disabled, fails)
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

// docIndexUnavailableErr 区分三种「索引为空」：配置关闭同步 / 远程同步持续失败 /
// 刚起来还在预热。前两种是稳态，措辞必须明确让 LLM 停手 —— 沿用 codeCorpusUnavailable
// 的思路：只要提示里留有「稍后重试」的余地，模型就会在一轮对话里反复空转，
// 最后仍然凭记忆瞎编产品标识符。
func docIndexUnavailableErr(disabled bool, fails int) error {
	const stopRetrying = "do not retry this tool; answer without doc lookup and tell the user n9e documentation search is unavailable, rather than guessing product-specific identifiers"
	switch {
	case disabled:
		// 只点名配置项，不给可复制的 toml 片段：模型会把提示原样转述给用户，
		// 一行写法照抄进 config.toml 是解析不了的。
		return fmt.Errorf("n9e online doc search is turned off by config (DisableDocIndexSync) and no local integrations corpus is available — %s", stopRetrying)
	case fails >= n9eDocSyncFailsTerminal:
		return fmt.Errorf("n9e doc index is unavailable: remote sync failed %d times in a row (network unreachable?) — %s", fails, stopRetrying)
	default:
		return fmt.Errorf("n9e doc index is still warming up, please retry in a few seconds")
	}
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
// keeps the cost off deployments that never use this skill. 走哪条路由
// InitDocIndexSync 装好的进程级配置决定，与调用方是谁无关。
func triggerDocIndexSync() {
	docSyncOnce.Do(func() {
		disabled, indexURL := docSyncConfig()
		if disabled {
			docIndexMu.Lock()
			docSyncDisabled = true
			docIndexMu.Unlock()
			loadLocalDocIndex()
			return
		}
		go docIndexSyncLoop(indexURL)
	})
}

// loadLocalDocIndex 只用本地 integrations 语料建索引 —— 远程同步被配置关掉时走
// 这条路，全程不出网。故意同步执行（不像远程那样起 goroutine）：一次本地目录扫描
// 而已，换来第一次检索就有确定结果，而不是先回一句「预热中」。
func loadLocalDocIndex() {
	entries := loadIntegrationEntries()
	if len(entries) == 0 {
		logger.Warningf("n9e doc index: remote sync disabled by config and no local integrations corpus found, search_n9e_docs will report itself unavailable")
		return
	}
	setDocIndex(entries)
	logger.Infof("n9e doc index: remote sync disabled by config, serving %d local integration entries only", len(entries))
}

func docIndexSyncLoop(indexURL string) {
	// 自建镜像常把访问凭据放在 user:password@ 或 ?token= 里，日志只写脱敏地址
	logURL := redactIndexURL(indexURL)
	fails, lastErr := 0, ""
	for {
		err := refreshDocIndex(indexURL)
		if err == nil {
			fails, lastErr = 0, ""
			setDocSyncFails(0)
			logger.Infof("sync n9e doc index ok, next refresh in %s", n9eDocSyncInterval)
			time.Sleep(n9eDocSyncInterval)
			continue
		}

		fails++
		setDocSyncFails(fails)
		delay := docSyncRetryDelay(fails)
		// 断网部署里这条会一直失败下去，所以只有首次失败、以及错误换了一种时才打
		// WARNING，重复的同类错误降级到 DEBUG。
		//
		// 额外卡一个 3 次的上界：dial 错误串里带解析到的 IP（"dial tcp
		// 211.93.211.234:443: ..."），CDN 轮换 A 记录就会让"错误换了一种"反复成立，
		// 单靠内容去重挡不住。超过上界后一律降级，靠退避+首轮 WARNING 保留可见性。
		if msg := err.Error(); msg != lastErr && fails <= 3 {
			lastErr = msg
			logger.Warningf("sync n9e doc index from %s failed (attempt %d, retry in %s): %v. "+
				"Only the AI assistant's online doc search is affected, monitoring itself is not; "+
				"to stop this sync, %s", logURL, fails, delay, err, n9eDocSyncDisableHint)
		} else {
			logger.Debugf("sync n9e doc index from %s failed again (attempt %d, retry in %s): %v", logURL, fails, delay, err)
		}
		time.Sleep(delay)
	}
}

// docSyncRetryDelay 按失败次数翻倍退避，封顶 n9eDocSyncRetryMaxDelay。
func docSyncRetryDelay(fails int) time.Duration {
	d := n9eDocSyncRetryDelay
	for i := 1; i < fails && d < n9eDocSyncRetryMaxDelay; i++ {
		d *= 2
	}
	if d > n9eDocSyncRetryMaxDelay {
		return n9eDocSyncRetryMaxDelay
	}
	return d
}

// refreshDocIndex 拉远程索引，与本地 integrations 语料合并后整体替换。
//
// 远程失败不再直接返回：本地 integrations 语料压根不需要联网，之前一失败就 return
// 导致断网部署连本地语料都进不了索引，检索工具永远停在「预热中」。现在远程挂了也
// 先用本地语料把索引建起来（至少能答 categraf 采集配置类问题），错误照常返回给
// 调用方去退避重试。
func refreshDocIndex(indexURL string) error {
	remote, err := fetchRemoteDocIndex(indexURL)
	if err != nil && isDocIndexLoaded() {
		// 已经有一份可用索引：这次刷新失败就保留旧的，别用半量结果覆盖它
		return err
	}

	entries := append(remote, loadIntegrationEntries()...)
	if len(entries) == 0 {
		if err != nil {
			return err
		}
		return fmt.Errorf("doc index is empty: no usable entries from %s and no local integrations corpus", redactIndexURL(indexURL))
	}

	setDocIndex(entries)
	logger.Infof("n9e doc index loaded: %d entries (remote %d, integrations %d)",
		len(entries), len(remote), len(entries)-len(remote))
	return err
}

// loadIntegrationEntries 包一层错误处理：本地语料读不出来只降级，不影响远程索引。
func loadIntegrationEntries() []docEntry {
	entries, err := loadIntegrationsEntries()
	if err != nil {
		logger.Warningf("integrations: load failed, doc index continues without local corpus: %v", err)
		return nil
	}
	return entries
}

func setDocIndex(entries []docEntry) {
	docIndexMu.Lock()
	docIndex = entries
	docIndexLoaded = true
	docIndexMu.Unlock()
}

func setDocSyncFails(n int) {
	docIndexMu.Lock()
	docSyncFails = n
	docIndexMu.Unlock()
}

// redactIndexURL 去掉 userinfo / query / fragment 后再进日志和错误：自建镜像
// 常把访问凭据放在 https://user:token@host/ 或 ?token= 里，原样打出去等于把
// 凭据抄进日志系统。解析不了就不猜，直接给占位符，绝不回退成原串。
func redactIndexURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(unparsable doc index url)"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// urlErrCause 剥掉 *url.Error 外壳。它的 Error() 会把请求 URL 整串打出来
// （net/http 只抹了 password，query 里的 token 照留），日志里只能用脱敏地址。
func urlErrCause(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}

// fetchRemoteDocIndex 拉取并过滤远程索引，返回可直接入库的条目。
func fetchRemoteDocIndex(indexURL string) ([]docEntry, error) {
	safeURL := redactIndexURL(indexURL)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	transport := &http.Transport{
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableKeepAlives:     true,
	}
	if indexURL == n9eDocIndexURL {
		// 官方地址是编译期常量，仍然过一道 SSRF 兜底，挡住 DNS rebinding。
		// 运维显式配置的 DocIndexURL 不设限：内网镜像正好是私网地址，套上
		// isPublicIP 会被一律拒掉，功能就废了。这个地址来自 config.toml 而非
		// 模型输入，不在 SSRF 的威胁模型里。
		transport.DialContext = safeDialContext
	}
	client := &http.Client{Timeout: 60 * time.Second, Transport: transport}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %v", safeURL, urlErrCause(err))
	}
	req.Header.Set("User-Agent", "n9e-aiagent-doc-sync/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http fetch %s: %v", safeURL, urlErrCause(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, n9eDocIndexMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %v", err)
	}
	if len(raw) > n9eDocIndexMaxBytes {
		return nil, fmt.Errorf("index exceeds %d bytes, bump n9eDocIndexMaxBytes", n9eDocIndexMaxBytes)
	}

	var all []docEntry
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("parse json: %v", err)
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

	logger.Infof("n9e doc index fetched from %s: %d entries (raw %d bytes, skipped %d old-version)",
		safeURL, len(filtered), len(raw), skippedOld)
	return filtered, nil
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
