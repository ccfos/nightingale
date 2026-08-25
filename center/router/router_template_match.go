package router

// 数据源模板匹配（哨兵指标法）：
// 每个内置大盘模板从它自己的 panel 表达式里挑 1–3 个「哨兵指标」，告警模板按
// (组件, cate) 组提取；请求时把全部哨兵拼成一条 instant query 打到目标数据源，
// 哪些哨兵有近期样本（默认 5min lookback），对应的模板组即判定「数据已存在」。
// 设计约束（详见产品方案 A4.2–A4.7）：
//   1. 请求数不随组件数增长 —— 全部哨兵合并进一条 __name__ 正则查询；
//   2. 匹配语义是「最近有数据」，不是「历史上出现过」；
//   3. 仅 Prometheus 系数据源适用。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/center/integration"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	promqlparser "github.com/prometheus/prometheus/promql/parser"
	"github.com/toolkits/pkg/logger"
)

type tplPayloadBrief struct {
	UUID int64  `json:"uuid"`
	Name string `json:"name"`
}

type tplDashboard struct {
	UUID      int64    `json:"uuid"`
	Name      string   `json:"name"`
	Sentinels []string `json:"sentinels"`
}

type tplAlertGroup struct {
	Cate      string            `json:"cate"` // 采集器变体标识，如 redis_by_categraf
	Sentinels []string          `json:"sentinels"`
	Rules     []tplPayloadBrief `json:"rules"`
}

type tplComponentEntry struct {
	ComponentID uint64          `json:"component_id"`
	Component   string          `json:"component"`
	Dashboards  []tplDashboard  `json:"dashboards"`
	AlertGroups []tplAlertGroup `json:"alert_groups"`
}

type tplMatchIndex struct {
	builtAt   time.Time
	entries   []*tplComponentEntry
	sentinels []string // 去重 + regexp.QuoteMeta 后的全部哨兵，用于拼一条查询
}

// ---------- 指标名提取 ----------

var (
	// 大盘模板里的 Grafana 风格时间变量，替换后再走 promql parser
	tplVarRangeRe = regexp.MustCompile(`\[\s*\$\{?[a-zA-Z_][a-zA-Z0-9_]*\}?\s*\]`)
	// fallback 词法提取前先剔除字符串字面量，避免把 label 值当成指标名
	tplStringLitRe = regexp.MustCompile(`"[^"]*"|'[^']*'`)
	tplTokenRe     = regexp.MustCompile(`[a-zA-Z_:][a-zA-Z0-9_:]*`)
)

// promql 函数与关键字，fallback 词法提取时排除
var promqlKeywords = map[string]struct{}{}

func init() {
	for _, kw := range []string{
		"abs", "absent", "absent_over_time", "avg", "avg_over_time", "bool", "bottomk", "by",
		"ceil", "changes", "clamp", "clamp_max", "clamp_min", "count", "count_over_time",
		"count_values", "day_of_month", "day_of_week", "day_of_year", "days_in_month",
		"delta", "deriv", "exp", "floor", "group", "group_left", "group_right",
		"histogram_quantile", "holt_winters", "hour", "idelta", "ignoring", "increase",
		"irate", "label_join", "label_replace", "last_over_time", "ln", "log10", "log2",
		"max", "max_over_time", "min", "min_over_time", "minute", "month", "offset", "on",
		"predict_linear", "present_over_time", "quantile", "quantile_over_time", "rate",
		"resets", "round", "scalar", "sgn", "sort", "sort_desc", "sqrt", "stddev",
		"stddev_over_time", "stdvar", "stdvar_over_time", "sum", "sum_over_time", "time",
		"timestamp", "topk", "unless", "vector", "without", "year", "and", "or", "atan2",
		"start", "end", "group_by",
	} {
		promqlKeywords[kw] = struct{}{}
	}
}

// extractMetricNames 从一条表达式中提取指标名。
// 优先走 promql parser（先替换模板时间变量）；解析失败（多为 $var 导致）退化为
// 词法启发式提取。提取噪音由哨兵挑选的黑名单/跨组件过滤 + 真实数据验证兜底。
func extractMetricNames(q string) []string {
	s := strings.ReplaceAll(q, "$__rate_interval", "5m")
	s = strings.ReplaceAll(s, "$__interval", "5m")
	s = strings.ReplaceAll(s, "$__range", "1h")
	s = tplVarRangeRe.ReplaceAllString(s, "[5m]")

	if expr, err := promqlparser.ParseExpr(s); err == nil {
		var names []string
		promqlparser.Inspect(expr, func(node promqlparser.Node, _ []promqlparser.Node) error {
			if vs, ok := node.(*promqlparser.VectorSelector); ok {
				if vs.Name != "" {
					names = append(names, vs.Name)
				} else {
					for _, m := range vs.LabelMatchers {
						if m.Name == labels.MetricName && m.Type == labels.MatchEqual {
							names = append(names, m.Value)
						}
					}
				}
			}
			return nil
		})
		return names
	}

	stripped := tplStringLitRe.ReplaceAllString(s, `""`)
	var names []string
	for _, tok := range tplTokenRe.FindAllString(stripped, -1) {
		if !strings.Contains(tok, "_") {
			continue
		}
		if _, bad := promqlKeywords[tok]; bad {
			continue
		}
		names = append(names, tok)
	}
	return names
}

// collectExprStrings 递归遍历模板 JSON，收集 expr / prom_ql / promql 字段。
// 兼容大盘（panel targets 的 expr）与告警（rule_config / 顶层的 prom_ql）两种结构。
func collectExprStrings(v interface{}, out *[]string) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, vv := range t {
			if s, ok := vv.(string); ok {
				if (k == "expr" || k == "prom_ql" || k == "promql") && strings.TrimSpace(s) != "" {
					*out = append(*out, s)
				}
				continue
			}
			collectExprStrings(vv, out)
		}
	case []interface{}:
		for _, item := range t {
			collectExprStrings(item, out)
		}
	}
}

// ---------- 哨兵挑选 ----------

// 跨组件通用指标：谁都有，不能证明某个组件存在
var sentinelExactBlacklist = map[string]struct{}{
	"up":                            {},
	"process_cpu_seconds_total":     {},
	"process_resident_memory_bytes": {},
	"process_open_fds":              {},
	"process_start_time_seconds":    {},
	"go_goroutines":                 {},
	"go_threads":                    {},
}

var sentinelPrefixBlacklist = []string{"go_", "process_", "promhttp_", "scrape_", "net_conntrack_"}

func inSentinelBlacklist(m string) bool {
	if _, ok := sentinelExactBlacklist[m]; ok {
		return true
	}
	for _, p := range sentinelPrefixBlacklist {
		if strings.HasPrefix(m, p) {
			return true
		}
	}
	return false
}

// pickSentinels 从一组指标里挑 ≤max 个哨兵：
// 黑名单排除 → 跨组件通用指标排除 → uptime/up 类加权 + 出现次数排序。
// 严格筛选后为空时放宽跨组件限制取 1 个（弱证据，聊胜于无）。
func pickSentinels(metrics map[string]int, isShared func(string) bool, max int) []string {
	type cand struct {
		name  string
		score int
	}
	build := func(relaxShared bool) []cand {
		var out []cand
		for m, cnt := range metrics {
			if inSentinelBlacklist(m) {
				continue
			}
			if !relaxShared && isShared(m) {
				continue
			}
			score := cnt
			if strings.Contains(m, "uptime") || strings.HasSuffix(m, "_up") || strings.Contains(m, "boot_time") {
				score += 100
			}
			out = append(out, cand{m, score})
		}
		return out
	}

	cands := build(false)
	limit := max
	if len(cands) == 0 {
		cands = build(true)
		limit = 1
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		return cands[i].name < cands[j].name
	})
	if len(cands) > limit {
		cands = cands[:limit]
	}
	names := make([]string, 0, len(cands))
	for _, c := range cands {
		names = append(names, c.name)
	}
	return names
}

// ---------- 索引构建（懒构建 + TTL 缓存） ----------

// 按语言分桶：模板名要跟着请求方的 X-Language 走，否则英文用户看到的是中文盘名/规则名。
// key 必须是 ResolveBucketLang 收敛后的语言：归一化后的语言码直接来自请求头，
// 只有收敛到真实存在的词条桶，这里的 key 空间才随内置语言数有界。
var (
	tplMatchIdxMu sync.Mutex
	tplMatchIdx   = map[string]*tplMatchIndex{}
)

const tplMatchIndexTTL = 10 * time.Minute

func (rt *Router) getTplMatchIndex(lang string) (*tplMatchIndex, error) {
	tplMatchIdxMu.Lock()
	defer tplMatchIdxMu.Unlock()
	cached := tplMatchIdx[lang]
	if cached != nil && time.Since(cached.builtAt) < tplMatchIndexTTL {
		return cached, nil
	}
	idx, err := rt.buildTplMatchIndex(lang)
	if err != nil {
		if cached != nil {
			// 构建失败时容忍旧索引
			logger.Warningf("rebuild template-match index failed, keep stale: %v", err)
			return cached, nil
		}
		return nil, err
	}
	tplMatchIdx[lang] = idx
	return idx, nil
}

func (rt *Router) buildTplMatchIndex(lang string) (*tplMatchIndex, error) {
	t0 := time.Now()

	comps, err := models.BuiltinComponentGets(rt.Ctx, "", -1)
	if err != nil {
		return nil, err
	}

	type dashRaw struct {
		uuid    int64
		name    string
		metrics map[string]int
	}
	type groupRaw struct {
		cate    string
		rules   []tplPayloadBrief
		metrics map[string]int
	}
	type compRaw struct {
		id     uint64
		ident  string
		dashes []*dashRaw
		groups map[string]*groupRaw
	}

	var raws []*compRaw
	// 指标 → 出现的组件集合，用于排除跨组件通用指标
	metricComponents := map[string]map[uint64]struct{}{}
	addMetric := func(compId uint64, bag map[string]int, m string) {
		bag[m]++
		set := metricComponents[m]
		if set == nil {
			set = map[uint64]struct{}{}
			metricComponents[m] = set
		}
		set[compId] = struct{}{}
	}
	extractInto := func(compId uint64, content string, bag map[string]int) {
		var doc interface{}
		if err := json.Unmarshal([]byte(content), &doc); err != nil {
			return
		}
		var exprs []string
		collectExprStrings(doc, &exprs)
		for _, e := range exprs {
			for _, m := range extractMetricNames(e) {
				addMetric(compId, bag, m)
			}
		}
	}

	for _, comp := range comps {
		cr := &compRaw{id: comp.ID, ident: comp.Ident, groups: map[string]*groupRaw{}}

		// 内置模板全量在文件端内存（BuiltinPayloadInFile）；用户自建模板不参与推荐
		dashboards, _ := integration.BuiltinPayloadInFile.GetBuiltinPayload("dashboard", "", "", comp.ID, lang)
		for _, p := range dashboards {
			bag := map[string]int{}
			extractInto(comp.ID, p.Content, bag)
			if len(bag) == 0 {
				continue
			}
			cr.dashes = append(cr.dashes, &dashRaw{uuid: p.UUID, name: p.Name, metrics: bag})
		}

		alerts, _ := integration.BuiltinPayloadInFile.GetBuiltinPayload("alert", "", "", comp.ID, lang)
		for _, p := range alerts {
			g := cr.groups[p.Cate]
			if g == nil {
				g = &groupRaw{cate: p.Cate, metrics: map[string]int{}}
				cr.groups[p.Cate] = g
			}
			g.rules = append(g.rules, tplPayloadBrief{UUID: p.UUID, Name: p.Name})
			extractInto(comp.ID, p.Content, g.metrics)
		}

		if len(cr.dashes)+len(cr.groups) > 0 {
			raws = append(raws, cr)
		}
	}

	isShared := func(m string) bool { return len(metricComponents[m]) > 2 }

	idx := &tplMatchIndex{builtAt: time.Now()}
	sentinelSet := map[string]struct{}{}
	for _, cr := range raws {
		entry := &tplComponentEntry{ComponentID: cr.id, Component: cr.ident}
		for _, d := range cr.dashes {
			ss := pickSentinels(d.metrics, isShared, 3)
			if len(ss) == 0 {
				continue
			}
			entry.Dashboards = append(entry.Dashboards, tplDashboard{UUID: d.uuid, Name: d.name, Sentinels: ss})
			for _, s := range ss {
				sentinelSet[s] = struct{}{}
			}
		}
		for _, g := range cr.groups {
			ss := pickSentinels(g.metrics, isShared, 3)
			if len(ss) == 0 {
				continue
			}
			sort.Slice(g.rules, func(i, j int) bool { return g.rules[i].Name < g.rules[j].Name })
			entry.AlertGroups = append(entry.AlertGroups, tplAlertGroup{Cate: g.cate, Sentinels: ss, Rules: g.rules})
			for _, s := range ss {
				sentinelSet[s] = struct{}{}
			}
		}
		if len(entry.Dashboards)+len(entry.AlertGroups) == 0 {
			continue
		}
		sort.Slice(entry.Dashboards, func(i, j int) bool { return entry.Dashboards[i].Name < entry.Dashboards[j].Name })
		sort.Slice(entry.AlertGroups, func(i, j int) bool { return entry.AlertGroups[i].Cate < entry.AlertGroups[j].Cate })
		idx.entries = append(idx.entries, entry)
	}
	sort.Slice(idx.entries, func(i, j int) bool { return idx.entries[i].Component < idx.entries[j].Component })

	for s := range sentinelSet {
		idx.sentinels = append(idx.sentinels, regexp.QuoteMeta(s))
	}
	sort.Strings(idx.sentinels)

	logger.Infof("template-match index built: lang=%s, %d components, %d sentinels, cost %dms",
		lang, len(idx.entries), len(idx.sentinels), time.Since(t0).Milliseconds())
	return idx, nil
}

// match 按命中的哨兵过滤出可推荐的模板组；组级命中即整组可用（产品方案 A4.3）
func (idx *tplMatchIndex) match(hit map[string]struct{}) []*tplComponentEntry {
	anyHit := func(sentinels []string) bool {
		for _, s := range sentinels {
			if _, ok := hit[s]; ok {
				return true
			}
		}
		return false
	}

	out := []*tplComponentEntry{}
	for _, e := range idx.entries {
		me := &tplComponentEntry{ComponentID: e.ComponentID, Component: e.Component}
		for _, d := range e.Dashboards {
			if anyHit(d.Sentinels) {
				me.Dashboards = append(me.Dashboards, d)
			}
		}
		for _, g := range e.AlertGroups {
			if anyHit(g.Sentinels) {
				me.AlertGroups = append(me.AlertGroups, g)
			}
		}
		if len(me.Dashboards)+len(me.AlertGroups) > 0 {
			out = append(out, me)
		}
	}
	return out
}

// ---------- HTTP handler ----------

func (rt *Router) datasourceTemplateMatch(c *gin.Context) {
	var req struct {
		Id int64 `json:"id"`
	}
	ginx.BindJSON(c, &req)

	ds := rt.DatasourceCache.GetById(req.Id)
	if ds == nil {
		ginx.Bomb(http.StatusNotFound, "datasource not found")
	}
	if ds.PluginType != models.PROMETHEUS {
		ginx.Bomb(http.StatusBadRequest, "only prometheus-like datasource is supported")
	}
	if rt.PromClients.IsNil(req.Id) {
		ginx.Bomb(http.StatusBadRequest, "prometheus client not ready, try again later")
	}

	lang := integration.NormalizeLang(c.GetHeader("X-Language"))
	if integration.BuiltinPayloadInFile != nil {
		lang = integration.BuiltinPayloadInFile.ResolveBucketLang(lang)
	}
	idx, err := rt.getTplMatchIndex(lang)
	ginx.Dangerous(err)

	if len(idx.sentinels) == 0 {
		ginx.NewRender(c).Data(gin.H{"matched": []*tplComponentEntry{}}, nil)
		return
	}

	// 哨兵合并成 __name__ 正则一次查完；但整库哨兵去重后仍有数百个，
	// 拼成单条 PromQL 会超过 TSDB 的查询长度上限（VictoriaMetrics 默认
	// -search.maxQueryLen=16384，实测 86 个组件拼出 ~20KB 直接 422）。
	// 因此按长度切成常数条批次：请求数只与哨兵总长度有关，仍不随组件数线性增长。
	qctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	cli := rt.PromClients.GetCli(req.Id)

	hit := map[string]struct{}{}
	for _, batch := range chunkSentinels(idx.sentinels, tplMatchMaxSelectorLen) {
		promql := fmt.Sprintf("count by (__name__) ({__name__=~%q})", strings.Join(batch, "|"))
		val, _, qErr := cli.Query(qctx, promql, time.Now())
		if qErr != nil {
			ginx.Dangerous(qErr)
			return
		}
		if vec, ok := val.(model.Vector); ok {
			for _, s := range vec {
				hit[string(s.Metric[model.MetricNameLabel])] = struct{}{}
			}
		}
	}

	ginx.NewRender(c).Data(gin.H{"matched": idx.match(hit)}, nil)
}

// 单批 selector 的最大字符数。留足余量给 `count by (__name__) ({__name__=~"..."})`
// 外壳与转义，避免贴着 TSDB 上限（VM 默认 16384）。
const tplMatchMaxSelectorLen = 8000

// chunkSentinels 按拼接后长度切批，保证每批 join 后不超过 maxLen。
// 单个哨兵本身超长时独占一批（不丢弃，交给 TSDB 判定）。
func chunkSentinels(sentinels []string, maxLen int) [][]string {
	var batches [][]string
	var cur []string
	curLen := 0
	for _, s := range sentinels {
		add := len(s)
		if len(cur) > 0 {
			add++ // "|"
		}
		if len(cur) > 0 && curLen+add > maxLen {
			batches = append(batches, cur)
			cur, curLen = nil, 0
			add = len(s)
		}
		cur = append(cur, s)
		curLen += add
	}
	if len(cur) > 0 {
		batches = append(batches, cur)
	}
	return batches
}
