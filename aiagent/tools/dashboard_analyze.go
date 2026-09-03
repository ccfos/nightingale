package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/prom"

	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/logger"
)

// ============================================================================
// get_dashboard_data — fetch every Prometheus curve of a dashboard over a time
// window, run the deterministic screening in dashboard_analyze_stats.go, and
// render a layered markdown digest for the agent (suspicious curves with
// sample points / one-line summaries for normal curves / counts for the rest).
//
// Spec: docs/specs/2026-06-07-analyze-dashboard-skill-design.md
// ============================================================================

func init() {
	register(defs.GetDashboardData, getDashboardDataTool)
}

// Sizing knobs (spec §3.3 / §3.6).
const (
	// The query step stays ≥60s: a finer step than the scrape interval makes
	// Prometheus repeat each sample, and the duplicated points corrupt the
	// MAD-of-diffs spike baseline (half the diffs become exactly 0). The FE
	// panel step (getDefaultStepByTime: window/240 floored at minStep 15s) is
	// tracked SEPARATELY and feeds only the $__interval/$__rate_interval
	// builtins, so rate() smoothing matches what the live dashboard renders —
	// at a blanket 60s rate window a 2-min spike clearly visible on a 1h
	// panel would be averaged ~2× lower than the user sees.
	analyzeMaxPointsPerQuery = 300       // query step = max(60, window/300)
	analyzeFEMaxPoints       = 240       // FE getDefaultStepByTime default maxDataPoints
	analyzeFEMinStepSec      = 15        // FE adjustStep minStep floor
	analyzeDisplayPoints     = 30        // sample points attached to each suspicious curve
	analyzeMaxSuspicious     = 30        // suspicious entries shown (ranked by score)
	analyzeMaxNormalLines    = 100       // one line per panel in the normal section
	analyzeMaxOutputBytes    = 32 * 1024 // hard cap on the rendered digest
	analyzeQueryTimeout      = 10 * time.Second
	analyzeQueryConcurrency  = 5
	// analyzeTotalTimeout caps the whole query phase (sequential variable
	// expansion + dual-window fan-out). Per-query timeouts alone let a slow
	// datasource stretch a big board to minutes (jobs/concurrency × 2 × 10s),
	// eating the agent run's entire budget; past this deadline the remaining
	// queries fail fast into failures and a partial digest still comes out.
	analyzeTotalTimeout   = 90 * time.Second
	analyzeMaxLabelValues = 200 // cap label_values expansion when resolving variables
)

// analyzableTypes are the panel types whose targets we query. Unlike
// fc-model-server we DO include stat: in a 1h~7d review window the trend of a
// "current value" panel (leader present, key count, ...) carries signal.
var analyzableTypes = map[string]bool{
	"timeseries": true, "barGauge": true, "barchart": true, "pie": true,
	"hexbin": true, "gauge": true, "heatmap": true, "stat": true,
}

type analysisTarget struct {
	Expr   string
	Legend string
}

type analysisPanel struct {
	ID              string
	Name            string
	Group           string // enclosing row name, "" when top-level
	GroupID         string // enclosing row id, so panel_ids can target a whole section
	Type            string
	Unit            string
	DatasourceCate  string
	DatasourceValue interface{}
	Targets         []analysisTarget
}

// collectAnalysisPanels flattens the panel tree (rows carry group names and
// may nest collapsed children) into the analysis-facing view.
func collectAnalysisPanels(panels []interface{}, group, groupID string, out *[]analysisPanel) {
	for _, p := range panels {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		typ := stringVal(pm, "type")
		if typ == "row" {
			// A row marker ends the previous section whether collapsed (children
			// nested under row.panels) or expanded (FE getRowPanels: a panel
			// belongs to the nearest preceding row) — so following top-level
			// panels must take THIS row's name, never inherit the prior one.
			group, groupID = stringVal(pm, "name"), stringVal(pm, "id")
			if nested, ok := pm["panels"].([]interface{}); ok && len(nested) > 0 {
				collectAnalysisPanels(nested, group, groupID, out)
			}
			continue
		}
		ap := analysisPanel{
			ID:              stringVal(pm, "id"),
			Name:            stringVal(pm, "name"),
			Group:           group,
			GroupID:         groupID,
			Type:            typ,
			Unit:            panelUnit(pm),
			DatasourceCate:  stringVal(pm, "datasourceCate"),
			DatasourceValue: pm["datasourceValue"],
		}
		if targets, ok := pm["targets"].([]interface{}); ok {
			for _, t := range targets {
				tm, ok := t.(map[string]interface{})
				if !ok {
					continue
				}
				if b, ok := tm["hide"].(bool); ok && b {
					continue
				}
				// __expr__ targets are server-evaluated expressions over other
				// refs — we cannot evaluate them here; counted by the caller.
				if stringVal(tm, "__mode__") == "__expr__" {
					continue
				}
				expr := stringVal(tm, "expr")
				if strings.TrimSpace(expr) == "" {
					continue
				}
				ap.Targets = append(ap.Targets, analysisTarget{Expr: expr, Legend: targetLegend(tm)})
			}
		}
		*out = append(*out, ap)
	}
}

// ============================================================================
// Variable resolution (spec §3.1 — simplified resolver)
// ============================================================================

type resolvedVars struct {
	values map[string]string // var name → substitution text ("v" or "(v1|v2)")
	dsIDs  map[string]int64  // datasource-type var name → datasource id
	notes  []string          // surfaced in the digest header ("ident 未取到值，已全匹配" ...)
}

// labelValuesRe parses label_values(<expr>, <label>) / label_values(<label>).
var labelValuesRe = regexp.MustCompile(`^\s*label_values\(\s*(?:(.+?)\s*,\s*)?([a-zA-Z_][a-zA-Z0-9_]*)\s*\)\s*$`)

// substRefRe matches $name, ${name} and [[name]] references.
var substRefRe = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}|\$([a-zA-Z_][a-zA-Z0-9_]*)|\[\[([a-zA-Z_][a-zA-Z0-9_]*)\]\]`)

func (rv *resolvedVars) substitute(s string) string {
	return substRefRe.ReplaceAllStringFunc(s, func(ref string) string {
		m := substRefRe.FindStringSubmatch(ref)
		name := m[1] + m[2] + m[3] // exactly one group is non-empty
		if v, ok := rv.values[name]; ok {
			return v
		}
		return ref
	})
}

// escapePromVarValue mirrors the FE's escapePromQLString (escapeString.ts,
// applied per selected value in ajustData.ts adjustValue): since 2024-07 it
// escapes ONLY parentheses, emitting `\\(` so the PromQL string literal
// yields a regex-escaped paren. Other regex metachars ('.', '+', ...) pass
// through unescaped on the live dashboard too — escaping them here would
// diverge from what the user's panels actually query.
func escapePromVarValue(s string) string {
	s = strings.ReplaceAll(s, "(", `\\(`)
	return strings.ReplaceAll(s, ")", `\\)`)
}

// joinVarValues renders a selected-value list as PromQL-substitutable text.
func joinVarValues(vals []string) string {
	if len(vals) == 1 {
		return escapePromVarValue(vals[0])
	}
	escaped := make([]string, len(vals))
	for i, v := range vals {
		escaped[i] = escapePromVarValue(v)
	}
	return "(" + strings.Join(escaped, "|") + ")"
}

// builtinVarValues mirrors the FE's getBuiltInVariables (Variables/utils/
// replaceTemplateVariables.ts): the global time/step placeholders panels
// reference without declaring ($__rate_interval & co. appear in dozens of
// builtin integration dashboards). Left unsubstituted they reach Prometheus
// verbatim and the query parse-errors, so every such panel would land in
// failures. step here is the FE-equivalent panel step (feStep), so
// $__interval/$__rate_interval substitute exactly what the live panel uses.
func builtinVarValues(stime, etime, step int64) map[string]string {
	window := etime - stime
	fromISO := time.Unix(stime, 0).UTC().Format("2006-01-02T15:04:05.000Z")
	toISO := time.Unix(etime, 0).UTC().Format("2006-01-02T15:04:05.000Z")
	return map[string]string{
		"__from":              strconv.FormatInt(stime*1000, 10), // epoch millis, like the FE
		"__from_date_seconds": strconv.FormatInt(stime, 10),
		"__from_date_iso":     fromISO,
		"__from_date":         fromISO,
		"__to":                strconv.FormatInt(etime*1000, 10),
		"__to_date_seconds":   strconv.FormatInt(etime, 10),
		"__to_date_iso":       toISO,
		"__to_date":           toISO,
		"__interval":          fmt.Sprintf("%ds", step),
		"__interval_ms":       strconv.FormatInt(step*1000, 10),
		"__rate_interval":     fmt.Sprintf("%ds", step*4),
		"__range":             fmt.Sprintf("%ds", window),
		"__range_s":           strconv.FormatInt(window, 10),
		"__range_ms":          strconv.FormatInt(window*1000, 10),
	}
}

// parseUserVars decodes the tool's vars argument: values may be a string, a
// number, or a non-empty array of strings/numbers. Anything else is an error,
// never a silent drop — a dropped key would slip past resolveDashboardVars'
// unknown-key guard (which sees only the parsed map) and leave the variable on
// full label_values expansion, so the digest would cover the whole fleet while
// the conversation claims it is scoped.
func parseUserVars(raw string) (map[string][]string, error) {
	out := map[string][]string{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("invalid vars JSON: %v", err)
	}
	for k, v := range m {
		switch t := v.(type) {
		case string:
			if strings.TrimSpace(t) == "" {
				return nil, fmt.Errorf("vars[%s] is an empty string: pass a value, or omit the key to use the dashboard default", k)
			}
			out[k] = []string{t}
		case float64:
			out[k] = []string{trimFloat(t)}
		case []interface{}:
			vals := make([]string, 0, len(t))
			for _, e := range t {
				switch et := e.(type) {
				case string:
					if strings.TrimSpace(et) == "" {
						return nil, fmt.Errorf("vars[%s] contains an empty string: array elements must be non-empty", k)
					}
					vals = append(vals, et)
				case float64:
					vals = append(vals, trimFloat(et))
				default:
					return nil, fmt.Errorf("vars[%s] contains an element of unsupported type %T: array elements must be strings or numbers", k, e)
				}
			}
			if len(vals) == 0 {
				return nil, fmt.Errorf("vars[%s] is an empty array: pass one or more values, or omit the key to use the dashboard default", k)
			}
			out[k] = vals
		default:
			return nil, fmt.Errorf("vars[%s] has unsupported type %T: pass a string, a number, or an array of strings/numbers", k, v)
		}
	}
	return out, nil
}

// varRestsOnAll reports whether the variable's resting selection is the FE's
// 全选 state. FE ajustData.ts recognises ONLY the array forms ['all'] /
// ['__all__'] as the select-all sentinel (and getValueByOptions produces
// ['all'] for a no-default multi+allOption var); a SCALAR "all" default fails
// its _.isEqual(value, ['all']) check and substitutes literally.
func varRestsOnAll(vm map[string]interface{}) bool {
	switch t := vm["defaultValue"].(type) {
	case string:
		if strings.TrimSpace(t) != "" {
			return false // scalar default — literal substitution, even "all"
		}
		// no default — fall through to the multi+allOption check
	case []interface{}:
		if len(t) > 0 {
			for _, e := range t {
				if s, _ := e.(string); s != "all" && s != "__all__" {
					return false
				}
			}
			return true
		}
	case float64:
		return false
	}
	multi, _ := vm["multi"].(bool)
	allOption, _ := vm["allOption"].(bool)
	return multi && allOption
}

// defaultValueList normalises a variable's defaultValue into the literal
// value list the FE would substitute (the all-sentinel ARRAY forms are
// handled by varRestsOnAll before this is consulted).
func defaultValueList(dv interface{}) []string {
	switch t := dv.(type) {
	case string:
		if s := strings.TrimSpace(t); s != "" {
			return []string{s}
		}
	case float64:
		return []string{trimFloat(t)}
	case []interface{}:
		var vals []string
		for _, e := range t {
			// Mirror the scalar branches above (and parseUserVars): a numeric
			// array element is a real selected value, not a drop — losing it
			// would fall the variable through to label_values expansion and
			// scope the analysis to the wrong series.
			switch ev := e.(type) {
			case string:
				if ev != "" {
					vals = append(vals, ev)
				}
			case float64:
				vals = append(vals, trimFloat(ev))
			}
		}
		return vals
	}
	return nil
}

// resolveDashboardVars resolves datasource variables first (the FE resolves
// variables by dependency topo-sort — VariableManagerContext — so a query
// variable may legally reference a datasource variable declared after it),
// then walks the rest of configs["var"] in declaration order (cascading
// query definitions reference earlier variables) and produces the
// substitution table, pre-seeded with the FE's built-in time/step
// placeholders. Priority per variable: tool vars arg → allValue (when resting
// on 全选) → defaultValue → label_values expansion → ".*" fallback (noted).
func resolveDashboardVars(ctx context.Context, deps *aiagent.ToolDeps, configs map[string]interface{},
	userVars map[string][]string, fallbackDsID, stime, etime, step int64) (*resolvedVars, error) {

	rv := &resolvedVars{values: builtinVarValues(stime, etime, step), dsIDs: map[string]int64{}}
	rawVars, _ := configs["var"].([]interface{})

	// Validate userVars keys BEFORE any expansion work. A key that matches no
	// dashboard variable is an error, not a silent no-op: a typo'd key
	// ({"indent": ...}) would leave the real variable on full label_values
	// expansion, so the digest silently covers the whole fleet while the
	// conversation claims it is scoped to one host.
	var names []string // declaration order
	nameSet := map[string]bool{}
	for _, v := range rawVars {
		if vm, ok := v.(map[string]interface{}); ok {
			if name := stringVal(vm, "name"); name != "" && !nameSet[name] {
				nameSet[name] = true
				names = append(names, name)
			}
		}
	}
	var unknown []string
	for k := range userVars {
		if !nameSet[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		avail := strings.Join(names, ", ")
		if avail == "" {
			avail = "<this dashboard has no variables>"
		}
		return nil, fmt.Errorf("vars key(s) %s do not match any dashboard variable (available: %s)",
			strings.Join(unknown, ", "), avail)
	}

	// Pass 1: datasource variables, regardless of where they are declared.
	// They never reference other variables (definition is a plugin type,
	// defaultValue a datasource id), so hoisting them is always safe — and a
	// query variable resolved before its datasource variable would silently
	// run label_values against the fallback datasource.
	for _, v := range rawVars {
		vm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		name := stringVal(vm, "name")
		if name == "" || stringVal(vm, "type") != "datasource" {
			continue
		}
		id := int64(0)
		if uv, ok := userVars[name]; ok && len(uv) > 0 {
			id, _ = strconv.ParseInt(uv[0], 10, 64)
		}
		if id == 0 {
			switch dv := vm["defaultValue"].(type) {
			case float64:
				id = int64(dv)
			case string:
				id, _ = strconv.ParseInt(dv, 10, 64)
			}
		}
		if id == 0 {
			id = fallbackDsID
		}
		rv.dsIDs[name] = id
		rv.values[name] = strconv.FormatInt(id, 10)
	}

	// Pass 2: everything else (query/textbox/custom/constant). The FE resolves
	// variables by dependency topo-sort (VariableManagerContext), so a
	// definition may legally reference a variable declared LATER — iterate to
	// a fixed point, resolving vars whose $refs are all ready; if a round
	// makes no progress (cycle / dangling ref), resolve the remainder in
	// declaration order rather than dropping them.
	var pending []map[string]interface{}
	for _, v := range rawVars {
		vm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if name := stringVal(vm, "name"); name != "" && stringVal(vm, "type") != "datasource" {
			pending = append(pending, vm)
		}
	}
	for len(pending) > 0 {
		var deferred []map[string]interface{}
		for _, vm := range pending {
			if varRefsResolved(vm, rv, nameSet) {
				resolveOneVar(ctx, deps, rv, vm, userVars, fallbackDsID)
			} else {
				deferred = append(deferred, vm)
			}
		}
		if len(deferred) == len(pending) { // no progress
			for _, vm := range deferred {
				resolveOneVar(ctx, deps, rv, vm, userVars, fallbackDsID)
			}
			break
		}
		pending = deferred
	}
	return rv, nil
}

// varRefsResolved reports whether every $ref in the variable's definition /
// reg / datasource.value that names ANOTHER dashboard variable is already in
// the substitution table — the resolution loop defers a variable until its
// dependencies are ready, mirroring the FE's dependency topo-sort.
func varRefsResolved(vm map[string]interface{}, rv *resolvedVars, declared map[string]bool) bool {
	refs := stringVal(vm, "definition") + " " + stringVal(vm, "reg")
	if ds, ok := vm["datasource"].(map[string]interface{}); ok {
		if s, ok := ds["value"].(string); ok {
			refs += " " + s
		}
	}
	self := stringVal(vm, "name")
	for _, m := range substRefRe.FindAllStringSubmatch(refs, -1) {
		name := m[1] + m[2] + m[3]
		if name == self || !declared[name] {
			continue // self/dangling refs can never become ready
		}
		if _, done := rv.values[name]; !done {
			return false
		}
	}
	return true
}

// resolveOneVar fills rv.values for one non-datasource variable. Priority:
// tool vars arg → constant definition → 全选 (allValue literal, else the full
// option set) → defaultValue literal(s) → option head → ".*" fallback.
func resolveOneVar(ctx context.Context, deps *aiagent.ToolDeps, rv *resolvedVars,
	vm map[string]interface{}, userVars map[string][]string, fallbackDsID int64) {

	name := stringVal(vm, "name")
	typ := stringVal(vm, "type")

	if uv, ok := userVars[name]; ok && len(uv) > 0 {
		rv.values[name] = joinVarValues(uv)
		return
	}
	// FE (ajustData.ts): a constant var substitutes its whole definition
	// string — defaultValue is ignored, and ".*" would be plain wrong.
	if typ == "constant" {
		rv.values[name] = stringVal(vm, "definition")
		return
	}
	// FE behavior (ajustData.ts): the 全选 resting state substitutes the
	// variable's custom allValue literal when one is set; without one the FE
	// joins the FULL option set (handled below by skipping the head
	// truncation). Any other defaultValue substitutes literally — note a
	// SCALAR "all" is literal on the FE too (varRestsOnAll).
	restsOnAll := varRestsOnAll(vm)
	if restsOnAll {
		if av := strings.TrimSpace(stringVal(vm, "allValue")); av != "" {
			rv.values[name] = av
			return
		}
	} else if dvs := defaultValueList(vm["defaultValue"]); len(dvs) > 0 {
		rv.values[name] = joinVarValues(dvs)
		return
	}
	if typ == "query" || typ == "" {
		if vals := expandQueryVariable(ctx, deps, rv, vm, fallbackDsID); len(vals) > 0 {
			// FE behavior (getValueByOptions.ts): with no default, only a
			// multi+allOption variable rests on the 全选 state; everything
			// else rests on the FIRST option. Joining the full set here
			// would scope the analysis to the whole fleet while the live
			// dashboard renders just the head value.
			if !restsOnAll {
				vals = vals[:1]
			} else if len(vals) > analyzeMaxLabelValues {
				// Never truncate silently: a 全选 over 350 hosts scoped to
				// the first 200 must say so, or the digest claims coverage
				// it doesn't have.
				rv.notes = append(rv.notes, fmt.Sprintf("变量 %s 有 %d 个选项，仅分析按字典序前 %d 个",
					name, len(vals), analyzeMaxLabelValues))
				vals = vals[:analyzeMaxLabelValues]
			}
			rv.values[name] = joinVarValues(vals)
			return
		}
	}
	// FE (Custom.tsx): custom options come from the comma-split definition,
	// trimmed and sorted by value, then follow the same head/全选 resting
	// rules as query vars. Falling to ".*" instead breaks panels outright —
	// shipped boards use custom vars INSIDE range selectors (rate(x[$interval])),
	// where ".*" is a PromQL parse error.
	if typ == "custom" {
		var opts []string
		for _, s := range strings.Split(stringVal(vm, "definition"), ",") {
			if s = strings.TrimSpace(s); s != "" {
				opts = append(opts, s)
			}
		}
		if reg := strings.TrimSpace(stringVal(vm, "reg")); reg != "" && len(opts) > 0 {
			filtered, ok := applyVarReg(rv.substitute(reg), opts)
			if !ok {
				rv.notes = append(rv.notes, fmt.Sprintf("变量 %s 的 reg %q 无法按 RE2 编译，选项未过滤", name, reg))
			} else {
				opts = filtered
			}
		}
		sort.Strings(opts) // FE _.sortBy(options, 'value')
		if len(opts) > 0 {
			if !restsOnAll {
				opts = opts[:1]
			}
			rv.values[name] = joinVarValues(opts)
			return
		}
	}
	// FE rests a no-default textbox on the empty string (getValueByOptions
	// textbox branch), never ".*".
	if typ == "textbox" {
		rv.values[name] = ""
		return
	}
	rv.values[name] = ".*"
	rv.notes = append(rv.notes, fmt.Sprintf("变量 %s 未取到值，已用 .* 全匹配", name))
}

// varRegSlashRe parses the FE's /pattern/flags reg form (stringToRegex).
var varRegSlashRe = regexp.MustCompile(`^/(.*)/([gimy]*)$`)

// applyVarReg mirrors the FE's filterOptionsByReg (VariableConfig/constant.tsx):
// a variable's reg keeps only the matching label values, and a capture group —
// one named "value", else the first positional one — replaces the value (e.g.
// extracting host from host:port). The bare (non-/.../) form is anchored ^…$
// exactly like the FE. Returns ok=false when the pattern doesn't compile under
// RE2 (JS lookaheads etc.); the caller keeps the unfiltered values and surfaces
// a note, matching the FE's lenient invalid-regex behavior.
func applyVarReg(reg string, vals []string) ([]string, bool) {
	reg = strings.TrimSpace(reg)
	if reg == "" {
		return vals, true
	}
	expr := reg
	if m := varRegSlashRe.FindStringSubmatch(reg); m != nil {
		expr = m[1]
		if strings.Contains(m[2], "i") {
			expr = "(?i)" + expr
		}
		if strings.Contains(m[2], "m") {
			expr = "(?m)" + expr
		}
	} else if strings.HasPrefix(reg, "/") {
		return vals, false // malformed /.../ form
	} else {
		expr = "^" + expr + "$"
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return vals, false
	}
	valueIdx := 0
	for i, n := range re.SubexpNames() {
		if n == "value" {
			valueIdx = i
		}
	}
	out := make([]string, 0, len(vals))
	seen := map[string]bool{}
	for _, v := range vals {
		if v == "" {
			continue
		}
		m := re.FindStringSubmatch(v)
		if m == nil {
			continue
		}
		val := v
		if valueIdx > 0 && m[valueIdx] != "" {
			val = m[valueIdx]
		} else if len(m) > 1 && m[1] != "" {
			val = m[1]
		}
		if !seen[val] {
			seen[val] = true
			out = append(out, val)
		}
	}
	return out, true
}

// expandQueryVariable resolves a query variable through the Prometheus
// label-values API. Returns nil on any failure (caller falls back to ".*").
func expandQueryVariable(ctx context.Context, deps *aiagent.ToolDeps, rv *resolvedVars,
	vm map[string]interface{}, fallbackDsID int64) []string {

	if deps.GetPromClient == nil {
		return nil
	}
	def := rv.substitute(stringVal(vm, "definition"))
	m := labelValuesRe.FindStringSubmatch(def)
	if m == nil {
		return nil
	}
	metricExpr, label := m[1], m[2]

	dsID := fallbackDsID
	if ds, ok := vm["datasource"].(map[string]interface{}); ok {
		switch dv := ds["value"].(type) {
		case float64:
			dsID = int64(dv)
		case string:
			// Resolve $name / ${name} / [[name]] datasource references — the same
			// three forms substitute() recognises; a [[name]] ref left unstripped
			// would miss rv.dsIDs and silently run label_values against the
			// fallback datasource.
			varName := strings.Trim(strings.TrimPrefix(strings.TrimSpace(dv), "$"), "{}[]")
			if id, ok := rv.dsIDs[varName]; ok && id != 0 {
				dsID = id
			}
		}
	}
	client := deps.GetPromClient(dsID)
	if client == nil {
		return nil
	}

	var matches []string
	if strings.TrimSpace(metricExpr) != "" {
		matches = []string{metricExpr}
	}
	qctx, cancel := context.WithTimeout(ctx, analyzeQueryTimeout)
	defer cancel()
	vals, _, err := client.LabelValues(qctx, label, matches)
	if err != nil {
		logger.Warningf("get_dashboard_data: label_values(%s) failed: %v", def, err)
		return nil
	}
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, string(v))
	}
	// The dashboard only shows the reg-filtered (and capture-extracted) option
	// set — scope the analysis to the same values the user actually sees.
	if reg := strings.TrimSpace(stringVal(vm, "reg")); reg != "" {
		filtered, ok := applyVarReg(rv.substitute(reg), out)
		if !ok {
			rv.notes = append(rv.notes, fmt.Sprintf("变量 %s 的 reg %q 无法按 RE2 编译，选项未过滤", stringVal(vm, "name"), reg))
		} else {
			out = filtered
		}
	}
	// FE sorts the option set lexicographically (_.sortBy(...,'value') in
	// Variable/Query.tsx) AFTER reg filtering and before resting on head —
	// without this, a reg capture group that reorders values (host:port → host)
	// would make the single-select head differ from the dropdown's. The 全选
	// join is capped (with a note) by the caller; the head case needs no cap.
	sort.Strings(out)
	return out
}

// firstPromDatasourceID picks the dashboard-independent fallback datasource:
// the first enabled Prometheus datasource visible to the user.
func firstPromDatasourceID(deps *aiagent.ToolDeps, user *models.User) int64 {
	dsList, err := models.GetDatasourcesGetsBy(deps.DBCtx, "prometheus", "", "", "")
	if err != nil || len(dsList) == 0 {
		return 0
	}
	if deps.FilterDatasources != nil {
		dsList = deps.FilterDatasources(dsList, user)
	}
	for _, ds := range dsList {
		if ds.Status == "enabled" {
			return ds.Id
		}
	}
	if len(dsList) > 0 {
		return dsList[0].Id
	}
	return 0
}

// panelDatasourceID resolves which datasource a panel queries.
func panelDatasourceID(p analysisPanel, rv *resolvedVars, fallback int64) int64 {
	switch dv := p.DatasourceValue.(type) {
	case float64:
		if dv > 0 {
			return int64(dv)
		}
	case string:
		// $name / ${name} / [[name]] — the three ref forms substitute() handles.
		varName := strings.Trim(strings.TrimPrefix(strings.TrimSpace(dv), "$"), "{}[]")
		if id, ok := rv.dsIDs[varName]; ok && id != 0 {
			return id
		}
		if id, err := strconv.ParseInt(dv, 10, 64); err == nil && id > 0 {
			return id
		}
	}
	return fallback
}

// ============================================================================
// Handler
// ============================================================================

// curveEntry is one analyzed series ready for rendering.
type curveEntry struct {
	Panel    *analysisPanel
	Legend   string
	Findings seriesFindings
	Ts       []int64
	Vals     []float64
}

type panelFailure struct {
	Panel  string
	Reason string
}

func getDashboardDataTool(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermDashboards); err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}
	board, err := models.BoardGetByID(deps.DBCtx, id)
	if err != nil {
		return "", fmt.Errorf("failed to get dashboard: %v", err)
	}
	if board == nil {
		return "", fmt.Errorf("dashboard not found: id=%d", id)
	}
	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(deps, user)
		if err != nil {
			return "", err
		}
		if !int64SliceContains(bgids, board.GroupId) {
			return "", fmt.Errorf("forbidden: no access to this dashboard")
		}
	}

	payload, err := models.BoardPayloadGet(deps.DBCtx, id)
	if err != nil {
		return "", fmt.Errorf("failed to get dashboard config: %v", err)
	}
	if strings.TrimSpace(payload) == "" {
		return "", fmt.Errorf("dashboard id=%d has no config payload", id)
	}
	var configs map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &configs); err != nil {
		return "", fmt.Errorf("invalid dashboard config payload: %v", err)
	}

	timeRange := getArgString(args, "time_range")
	if timeRange == "" {
		timeRange = "1h"
	}
	stime, etime := parseTimeRange(timeRange)
	if stime == 0 {
		return "", fmt.Errorf("invalid time_range %q: use formats like 15m / 1h / 24h / 7d", timeRange)
	}
	window := etime - stime
	step := int64(math.Max(60, float64(window)/analyzeMaxPointsPerQuery))
	// feStep mirrors the FE panel step and feeds only the builtin time vars —
	// per-point values then equal what the user's panels show, sampled at step.
	feStep := int64(math.Max(analyzeFEMinStepSec, float64(window)/analyzeFEMaxPoints))
	shift := window
	if shift < 86400 {
		shift = 86400 // ≤24h windows compare against yesterday's same slot
	}

	userVars, err := parseUserVars(getArgString(args, "vars"))
	if err != nil {
		return "", err
	}
	panelFilter, err := parsePanelIDs(getArgString(args, "panel_ids"))
	if err != nil {
		return "", err
	}

	// Collect panels and bucket the non-analyzable ones for the digest.
	var all []analysisPanel
	if rawPanels, ok := configs["panels"].([]interface{}); ok {
		collectAnalysisPanels(rawPanels, "", "", &all)
	}
	var panels []analysisPanel
	skippedTypes := map[string]int{}
	skippedCates := map[string]int{}
	for _, p := range all {
		// A panel_ids entry may name a ROW id — get_dashboard_detail surfaces
		// rows with their ids, and "analyze this section" is a natural ask;
		// matching only leaf panel ids would silently turn that into "no
		// analyzable panels".
		if len(panelFilter) > 0 && !panelFilter[p.ID] && !(p.GroupID != "" && panelFilter[p.GroupID]) {
			continue
		}
		if !analyzableTypes[p.Type] {
			skippedTypes[p.Type]++
			continue
		}
		// Empty cate is the v5-era default and means Prometheus.
		if p.DatasourceCate != "" && p.DatasourceCate != "prometheus" {
			skippedCates[p.DatasourceCate]++
			continue
		}
		if len(p.Targets) == 0 {
			continue
		}
		panels = append(panels, p)
	}
	if len(panels) == 0 {
		return "", fmt.Errorf("no analyzable Prometheus panels in dashboard %q (skipped types: %v, other datasource cates: %v, panel_ids filter: %d)",
			board.Name, mapKeys(skippedTypes), mapKeys(skippedCates), len(panelFilter))
	}
	if deps.GetPromClient == nil {
		return "", fmt.Errorf("prometheus query capability is not wired")
	}

	fallbackDsID := firstPromDatasourceID(deps, user)

	phaseCtx, cancelPhase := context.WithTimeout(ctx, analyzeTotalTimeout)
	defer cancelPhase()

	rv, err := resolveDashboardVars(phaseCtx, deps, configs, userVars, fallbackDsID, stime, etime, feStep)
	if err != nil {
		return "", err
	}
	// The 同比 window gets its own substitution table: the absolute builtin
	// time vars ($__from/$__to family) must carry THAT window's timestamps —
	// reusing the current-window expr would embed today's range into
	// yesterday's query. Dashboard variables and the window-relative builtins
	// ($__interval/$__range/...) are identical for both windows.
	rvPrev := &resolvedVars{values: make(map[string]string, len(rv.values))}
	for k, v := range rv.values {
		rvPrev.values[k] = v
	}
	for k, v := range builtinVarValues(stime-shift, etime-shift, feStep) {
		rvPrev.values[k] = v
	}

	// ---- fan out the per-target queries (current + shifted window) ----
	type job struct {
		panel  *analysisPanel
		target analysisTarget
	}
	var jobs []job
	for i := range panels {
		for _, t := range panels[i].Targets {
			jobs = append(jobs, job{panel: &panels[i], target: t})
		}
	}

	var (
		mu            sync.Mutex
		curves        []curveEntry
		failures      []panelFailure
		emptyCnt      int
		vanished      []string
		vanishedTotal int
		noPrevCnt     int
		timedOutCnt   int // queries killed by the TOTAL phase budget, not by their datasource
	)
	sem := make(chan struct{}, analyzeQueryConcurrency)
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()

			dsID := panelDatasourceID(*j.panel, rv, fallbackDsID)
			client := deps.GetPromClient(dsID)
			if client == nil {
				mu.Lock()
				failures = append(failures, panelFailure{j.panel.Name, fmt.Sprintf("数据源 %d 不可用", dsID)})
				mu.Unlock()
				return
			}
			expr := rv.substitute(j.target.Expr)

			cur, err := promRangeMatrix(phaseCtx, client, expr, stime, etime, step)
			if err != nil {
				mu.Lock()
				// Distinguish "the 90s analysis budget ran out" from a real
				// datasource failure: blaming healthy-but-unattempted panels
				// with 查询失败 would send the user chasing a phantom outage.
				if phaseCtx.Err() != nil {
					timedOutCnt++
				} else {
					failures = append(failures, panelFailure{j.panel.Name, trimErr(err)})
				}
				mu.Unlock()
				return
			}
			prev, prevErr := promRangeMatrix(phaseCtx, client, rvPrev.substitute(j.target.Expr), stime-shift, etime-shift, step)
			if prevErr != nil {
				prev = nil // comparison is best-effort; detections still run
			}

			prevByFp := map[model.Fingerprint]*model.SampleStream{}
			for _, ss := range prev {
				prevByFp[ss.Metric.Fingerprint()] = ss
			}
			curFps := map[model.Fingerprint]bool{}

			mu.Lock()
			defer mu.Unlock()
			if prevErr != nil {
				noPrevCnt++
			}
			if len(cur) == 0 {
				// No early return: fall through so the vanished loop below still
				// runs — prev-window series with an entirely-empty current window
				// ("yesterday yes, today no" for the whole target) is the strongest
				// instance-loss signal, not just a neutral empty query.
				emptyCnt++
			}
			for _, ss := range cur {
				fp := ss.Metric.Fingerprint()
				curFps[fp] = true
				ts, vals := samplePairsToSlices(ss.Values)
				var prevTs []int64
				var prevVals []float64
				if pss, ok := prevByFp[fp]; ok {
					prevTs, prevVals = samplePairsToSlices(pss.Values)
				}
				f := analyzeSeries(ts, vals, prevTs, prevVals, shift, step)
				curves = append(curves, curveEntry{
					Panel:    j.panel,
					Legend:   renderLegend(j.target.Legend, ss.Metric),
					Findings: f,
					Ts:       ts,
					Vals:     vals,
				})
			}
			// "Vanished" (昨日有今日无) is only a trustworthy instance-loss
			// signal when series identity is stable across the two windows.
			// Matching is by full-label Fingerprint, so a panel whose series
			// carry rotating labels (pod names, ephemeral instance ids, build
			// hashes) shows ZERO overlap between yesterday and today even when
			// nothing is wrong — flagging every one of yesterday's series as
			// vanished would fabricate a fleet-wide outage. Trust the signal
			// only when at least one series persisted (a genuine partial loss
			// within a stable set) or the current window is entirely empty (the
			// whole target stopped returning data).
			overlap := 0
			for fp := range prevByFp {
				if curFps[fp] {
					overlap++
				}
			}
			reportVanished := overlap > 0 || len(cur) == 0
			for fp, pss := range prevByFp {
				if !curFps[fp] {
					if !reportVanished {
						continue
					}
					vanishedTotal++
					// Generous sample bound (display caps at 5); the exact
					// total is carried separately so truncation is never silent.
					if len(vanished) < 200 {
						vanished = append(vanished, renderLegend(j.target.Legend, pss.Metric))
					}
				}
			}
		}(j)
	}
	wg.Wait()
	// Map iteration and goroutine completion order are both randomized — sort
	// so identical data always reports the same (sub)set of vanished series.
	sort.Strings(vanished)

	digest := renderAnalysisDigest(board, timeRange, stime, etime, shift, window, rv, curves,
		failures, skippedTypes, skippedCates, emptyCnt, noPrevCnt, vanished, vanishedTotal, timedOutCnt)
	logger.Infof("get_dashboard_data: user=%s, id=%d, range=%s, panels=%d, curves=%d, bytes=%d",
		user.Username, id, timeRange, len(panels), len(curves), len(digest))
	return digest, nil
}

func parsePanelIDs(raw string) (map[string]bool, error) {
	out := map[string]bool{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	// Accept both string and numeric array elements: panel ids are stringified
	// via stringVal, so a numeric id 1 in the board must match a passed 1 as
	// "1". A []string unmarshal would reject [1,2] and fall through to the
	// comma split, which then mangles the raw JSON into garbage keys.
	var arr []interface{}
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		for _, e := range arr {
			switch et := e.(type) {
			case string:
				if et = strings.TrimSpace(et); et != "" {
					out[et] = true
				}
			case float64:
				out[trimFloat(et)] = true
			default:
				return nil, fmt.Errorf("panel_ids contains an element of unsupported type %T: array elements must be strings or numbers", e)
			}
		}
		return out, nil
	}
	// tolerate a bare comma-separated list
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out[s] = true
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("invalid panel_ids: pass a JSON array like [\"panel-1\"]")
	}
	return out, nil
}

func promRangeMatrix(ctx context.Context, client prom.API, expr string, stime, etime, step int64) (model.Matrix, error) {
	qctx, cancel := context.WithTimeout(ctx, analyzeQueryTimeout)
	defer cancel()
	value, _, err := client.QueryRange(qctx, expr, prom.Range{
		Start: time.Unix(stime, 0),
		End:   time.Unix(etime, 0),
		Step:  time.Duration(step) * time.Second,
	})
	if err != nil {
		return nil, err
	}
	matrix, ok := value.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("unexpected result type %s", value.Type())
	}
	return matrix, nil
}

func samplePairsToSlices(pairs []model.SamplePair) ([]int64, []float64) {
	ts := make([]int64, 0, len(pairs))
	vals := make([]float64, 0, len(pairs))
	for _, p := range pairs {
		v := float64(p.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		ts = append(ts, int64(p.Timestamp)/1000)
		vals = append(vals, v)
	}
	return ts, vals
}

// renderLegend mirrors the FE: {{label}} templates resolve from the series
// labels; without a template fall back to __name__ plus a few labels.
var legendTmplRe = regexp.MustCompile(`\{\{\s*(.+?)\s*\}\}`)

func renderLegend(legend string, metric model.Metric) string {
	if legend != "" {
		return legendTmplRe.ReplaceAllStringFunc(legend, func(s string) string {
			key := strings.TrimSpace(strings.Trim(s, "{}"))
			if v, ok := metric[model.LabelName(key)]; ok {
				return string(v)
			}
			return s
		})
	}
	name := string(metric[model.LabelName("__name__")])
	keys := make([]string, 0, len(metric))
	for k := range metric {
		if k != "__name__" {
			keys = append(keys, string(k))
		}
	}
	sort.Strings(keys)
	if len(keys) > 3 {
		keys = keys[:3]
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, metric[model.LabelName(k)]))
	}
	if len(parts) == 0 {
		return name
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}

// ============================================================================
// Digest rendering
// ============================================================================

func renderAnalysisDigest(board *models.Board, timeRange string, stime, etime, shift, window int64,
	rv *resolvedVars, curves []curveEntry, failures []panelFailure,
	skippedTypes, skippedCates map[string]int, emptyCnt, noPrevCnt int, vanished []string, vanishedTotal, timedOutCnt int) string {

	var suspicious, normal, flat []curveEntry
	for _, c := range curves {
		switch {
		// Suspicious outranks Flat: a series pegged constant today while
		// yesterday ran at a different level (qps stuck at 0) is flagged by
		// the YoY check and must not hide in the harmless flat bucket.
		case c.Findings.Suspicious:
			suspicious = append(suspicious, c)
		case c.Findings.Stats.Flat:
			flat = append(flat, c)
		default:
			normal = append(normal, c)
		}
	}
	sort.Slice(suspicious, func(i, j int) bool {
		if suspicious[i].Findings.Score != suspicious[j].Findings.Score {
			return suspicious[i].Findings.Score > suspicious[j].Findings.Score
		}
		// Ties are common (equal hit counts and magnitudes) and goroutine
		// completion order is random — break them deterministically so
		// identical data renders the identical digest, including WHICH
		// entries survive the analyzeMaxSuspicious cap.
		li := panelLabel(suspicious[i].Panel, suspicious[i].Legend)
		lj := panelLabel(suspicious[j].Panel, suspicious[j].Legend)
		return li < lj
	})

	var sb strings.Builder
	// The header endpoints always carry the date: for a 24h window the
	// bare-clock form would print the SAME hh:mm twice ("15:00 ~ 15:00").
	hdrFmt := "01-02 15:04"
	fmt.Fprintf(&sb, "## 仪表盘: %s (id=%d) | 窗口: %s (%s ~ %s) | %d条曲线: %d可疑 / %d正常 / %d平直\n",
		board.Name, board.Id, timeRange, time.Unix(stime, 0).Format(hdrFmt), time.Unix(etime, 0).Format(hdrFmt),
		len(curves), len(suspicious), len(normal), len(flat))
	fmt.Fprintf(&sb, "同比基准: 前移 %s 的同长窗口 | 检测: MAD离群/突变/趋势/同比(服务端确定性算法)\n", durStr(shift))
	if len(rv.notes) > 0 {
		fmt.Fprintf(&sb, "变量: %s\n", strings.Join(rv.notes, "; "))
	}

	// ---- suspicious ----
	if len(suspicious) == 0 {
		sb.WriteString("\n### 未发现可疑曲线\n所有曲线的离群/突变/趋势/同比检测均未触发。\n")
	} else {
		shown := suspicious
		if len(shown) > analyzeMaxSuspicious {
			shown = shown[:analyzeMaxSuspicious]
		}
		fmt.Fprintf(&sb, "\n### ⚠ 可疑曲线 (%d)\n", len(suspicious))
		for _, c := range shown {
			sb.WriteString(renderSuspiciousEntry(c, window))
		}
		if len(suspicious) > analyzeMaxSuspicious {
			fmt.Fprintf(&sb, "(另有 %d 条可疑曲线未展示，可用 panel_ids 聚焦相关面板)\n", len(suspicious)-analyzeMaxSuspicious)
		}
	}

	// ---- normal (grouped one line per panel) ----
	if len(normal) > 0 {
		fmt.Fprintf(&sb, "\n### ✓ 正常曲线摘要 (%d)\n", len(normal))
		sb.WriteString(renderNormalSection(normal, window))
	}

	// ---- skipped / failures ----
	sb.WriteString("\n### 略过\n")
	if len(flat) > 0 {
		names := make([]string, 0, 10)
		for i, c := range flat {
			if i >= 10 {
				break
			}
			names = append(names, fmt.Sprintf("%s=%s", panelLabel(c.Panel, c.Legend), humanVal(c.Findings.Stats.Last)))
		}
		extra := ""
		if len(flat) > 10 {
			extra = " ..."
		}
		fmt.Fprintf(&sb, "- 平直线 %d 条(值恒定): %s%s\n", len(flat), strings.Join(names, ", "), extra)
	}
	for _, typ := range mapKeys(skippedTypes) {
		fmt.Fprintf(&sb, "- 跳过 %d 个 %s 面板(类型不分析)\n", skippedTypes[typ], typ)
	}
	for _, cate := range mapKeys(skippedCates) {
		fmt.Fprintf(&sb, "- 跳过 %d 个 %s 数据源面板(仅支持 prometheus)\n", skippedCates[cate], cate)
	}
	if emptyCnt > 0 {
		fmt.Fprintf(&sb, "- %d 个查询无数据\n", emptyCnt)
	}
	if noPrevCnt > 0 {
		fmt.Fprintf(&sb, "- %d 个查询的同比窗口取数失败(同比检测跳过，其余照常)\n", noPrevCnt)
	}
	if len(vanished) > 0 {
		shown := vanished
		extra := ""
		if vanishedTotal > 5 {
			shown = shown[:5]
			extra = fmt.Sprintf(" 等%d条(共%d条)", vanishedTotal-5, vanishedTotal)
		}
		fmt.Fprintf(&sb, "- 昨日有、今日无的曲线: %s%s\n", strings.Join(shown, ", "), extra)
	}
	if timedOutCnt > 0 {
		fmt.Fprintf(&sb, "- %d 个查询因分析总预算(%ds)超时未完成——不是数据源故障，结果不完整，可用 panel_ids 分批分析\n",
			timedOutCnt, int(analyzeTotalTimeout/time.Second))
	}
	if len(failures) > 0 {
		// One bad datasource on a 10-target panel yields 10 identical entries
		// (jobs are per-target) in random completion order — dedup, sort, cap.
		seen := map[panelFailure]bool{}
		deduped := make([]panelFailure, 0, len(failures))
		for _, f := range failures {
			if !seen[f] {
				seen[f] = true
				deduped = append(deduped, f)
			}
		}
		sort.Slice(deduped, func(i, j int) bool {
			if deduped[i].Panel != deduped[j].Panel {
				return deduped[i].Panel < deduped[j].Panel
			}
			return deduped[i].Reason < deduped[j].Reason
		})
		for i, f := range deduped {
			if i >= 20 {
				fmt.Fprintf(&sb, "- (另有 %d 条查询失败未展示)\n", len(deduped)-20)
				break
			}
			fmt.Fprintf(&sb, "- 面板 %q 查询失败: %s\n", f.Panel, f.Reason)
		}
	}

	out := sb.String()
	if len(out) > analyzeMaxOutputBytes {
		cut := strings.LastIndexByte(out[:analyzeMaxOutputBytes], '\n')
		if cut <= 0 {
			// No newline in the first 32KB (one pathological line): back up to
			// a rune boundary — the digest is mostly multi-byte Chinese, and a
			// mid-rune cut would hand the LLM invalid UTF-8.
			cut = analyzeMaxOutputBytes
			for cut > 0 && !utf8.RuneStart(out[cut]) {
				cut--
			}
		}
		out = out[:cut] + "\n\n[输出超过 32KB 已截断：请用 panel_ids 分批分析]"
	}
	return out
}

func panelLabel(p *analysisPanel, legend string) string {
	name := p.Name
	if p.Group != "" {
		name = p.Group + "/" + name
	}
	if legend != "" && legend != name {
		return fmt.Sprintf("[%s] %s", name, legend)
	}
	return "[" + name + "]"
}

func renderSuspiciousEntry(c curveEntry, window int64) string {
	f := c.Findings
	var marks []string
	if len(f.Outliers) > 0 {
		top := topMark(f.Outliers)
		marks = append(marks, fmt.Sprintf("离群%s@%s", humanVal(top.Val), clockFmt(top.Ts, window)))
	}
	if len(f.Spikes) > 0 {
		top := topMark(f.Spikes)
		if pctCapped(top.Pct) {
			// The percent is the zero-baseline cap artifact — the level says it.
			marks = append(marks, fmt.Sprintf("突变(自≈0)→%s@%s", humanVal(top.Val), clockFmt(top.Ts, window)))
		} else {
			marks = append(marks, fmt.Sprintf("突变%s@%s", pctStr(top.Pct), clockFmt(top.Ts, window)))
		}
	}
	if f.TrendHit {
		if pctCapped(f.TrendPct) {
			marks = append(marks, "趋势"+trendArrow(f.TrendPct)+"(自≈0)")
		} else {
			marks = append(marks, "趋势"+trendArrow(f.TrendPct)+pctAbs(f.TrendPct))
		}
	}
	if f.YoY != nil && f.YoY.Hit {
		marks = append(marks, fmt.Sprintf("同比avg%s/max%s", pctStrZeroBase(f.YoY.AvgPct), pctStrZeroBase(f.YoY.MaxPct)))
	}
	if f.Stats.Flat {
		marks = append(marks, "今日值恒定")
	}
	if f.Periodic {
		marks = append(marks, "疑似周期性(昨日同现)")
	}
	if f.YoY == nil {
		marks = append(marks, "无昨日数据")
	}

	avgPart := "avg=" + humanVal(f.Stats.Avg)
	if f.YoY != nil {
		avgPart += fmt.Sprintf("(昨日%s,%s)", humanVal(f.YoY.PrevAvg), pctStrZeroBase(f.YoY.AvgPct))
	}
	unit := ""
	if c.Panel.Unit != "" {
		unit = " 单位:" + c.Panel.Unit
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s: %s min=%s max=%s@%s last=%s%s %s ⚠\n",
		panelLabel(c.Panel, c.Legend), avgPart,
		humanVal(f.Stats.Min),
		humanVal(f.Stats.Max), clockFmt(f.Stats.MaxTs, window),
		humanVal(f.Stats.Last), unit, strings.Join(marks, " "))

	allMarks := append(append([]pointMark(nil), f.Outliers...), f.Spikes...)
	ts, vals := downsampleForDisplay(c.Ts, c.Vals, allMarks, analyzeDisplayPoints)
	pts := make([]string, 0, len(ts))
	for i := range ts {
		pts = append(pts, fmt.Sprintf("%s=%s", clockFmt(ts[i], window), humanVal(vals[i])))
	}
	fmt.Fprintf(&sb, "  点: %s\n", strings.Join(pts, " "))
	return sb.String()
}

// renderNormalSection prints one line per panel: all its normal curves joined.
func renderNormalSection(normal []curveEntry, window int64) string {
	type panelGroup struct {
		label string
		parts []string
	}
	order := []string{}
	groups := map[string]*panelGroup{}
	for _, c := range normal {
		key := c.Panel.ID
		if key == "" {
			// Hand-imported boards may lack panel ids; pointer identity keeps
			// distinct id-less panels from collapsing into one "" group.
			key = fmt.Sprintf("%p", c.Panel)
		}
		g, ok := groups[key]
		if !ok {
			name := c.Panel.Name
			if c.Panel.Group != "" {
				name = c.Panel.Group + "/" + name
			}
			g = &panelGroup{label: name}
			groups[key] = g
			order = append(order, key)
		}
		if len(g.parts) >= 8 {
			if len(g.parts) == 8 {
				g.parts = append(g.parts, "...")
			}
			continue
		}
		f := c.Findings
		desc := "avg=" + humanVal(f.Stats.Avg)
		if f.YoY != nil {
			desc += "(" + pctStrZeroBase(f.YoY.AvgPct) + ")"
		}
		flags := ""
		if f.Periodic {
			// Carry the spike magnitude so the agent can sanity-check the
			// downgrade instead of taking "periodic" on faith.
			flags = " 周期性尖峰"
			if m := append(append([]pointMark(nil), f.Outliers...), f.Spikes...); len(m) > 0 {
				flags += "(" + pctStrZeroBase(topMark(m).Pct) + ")"
			}
		} else if math.Abs(f.TrendPct) >= 10 {
			flags = " 趋势" + trendArrow(f.TrendPct) + pctAbs(f.TrendPct)
		} else {
			flags = " 平稳"
		}
		legend := c.Legend
		if legend == "" {
			legend = "-"
		}
		g.parts = append(g.parts, legend+": "+desc+flags)
	}

	var sb strings.Builder
	for i, key := range order {
		if i >= analyzeMaxNormalLines {
			fmt.Fprintf(&sb, "(其余 %d 个面板的正常曲线已省略)\n", len(order)-analyzeMaxNormalLines)
			break
		}
		g := groups[key]
		fmt.Fprintf(&sb, "[%s] %s\n", g.label, strings.Join(g.parts, " | "))
	}
	return sb.String()
}

// ---- small formatting helpers ----

func topMark(marks []pointMark) pointMark {
	top := marks[0]
	for _, m := range marks[1:] {
		if math.Abs(m.Pct) > math.Abs(top.Pct) {
			top = m
		}
	}
	return top
}

func clockFmt(ts, window int64) string {
	t := time.Unix(ts, 0)
	// Windows of a day or longer carry the date: a 24h window's endpoints
	// share the same HH:MM, so "max@15:04" would be ambiguous between the
	// start and the end of the window.
	if window < 86400 {
		return t.Format("15:04")
	}
	return t.Format("01-02 15:04")
}

func durStr(sec int64) string {
	switch {
	case sec%86400 == 0:
		return fmt.Sprintf("%dd", sec/86400)
	case sec%3600 == 0:
		return fmt.Sprintf("%dh", sec/3600)
	default:
		return fmt.Sprintf("%dm", sec/60)
	}
}

func trendArrow(pct float64) string {
	if pct >= 0 {
		return "↑"
	}
	return "↓"
}

func pctStr(p float64) string {
	return fmt.Sprintf("%+.0f%%", p)
}

// pctStrZeroBase renders a percent, except at relPct's zero-baseline cap where
// "+99900%" would present an artifact as fact.
func pctStrZeroBase(p float64) string {
	if pctCapped(p) {
		return "自≈0" + trendArrow(p)
	}
	return pctStr(p)
}

func pctAbs(p float64) string {
	return fmt.Sprintf("%.0f%%", math.Abs(p))
}

func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// humanVal renders a sample value compactly: k/M/G suffixes above 1000, two
// significant decimals below, trailing zeros trimmed.
func humanVal(v float64) string {
	abs := math.Abs(v)
	switch {
	case abs >= 1e9:
		return trimZeros(fmt.Sprintf("%.2f", v/1e9)) + "G"
	case abs >= 1e6:
		return trimZeros(fmt.Sprintf("%.2f", v/1e6)) + "M"
	case abs >= 1e3:
		return trimZeros(fmt.Sprintf("%.2f", v/1e3)) + "k"
	case abs >= 0.01 || v == 0:
		return trimZeros(fmt.Sprintf("%.2f", v))
	default:
		return fmt.Sprintf("%.4g", v)
	}
}

func trimZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

func trimErr(err error) string {
	s := err.Error()
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

func mapKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
