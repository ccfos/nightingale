package tools

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/prom"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

// fakePromAPI implements just the two prom.API methods get_dashboard_data
// touches; the embedded interface panics on anything else, which is exactly
// what we want from a test double.
type fakePromAPI struct {
	prom.API
	mu        sync.Mutex
	queries   []recordedQuery
	rangeFn   func(query string, r prom.Range) model.Matrix
	labelVals model.LabelValues
}

type recordedQuery struct {
	Query      string
	Start, End time.Time
}

func (f *fakePromAPI) QueryRange(_ context.Context, query string, r prom.Range) (model.Value, prom.Warnings, error) {
	f.mu.Lock()
	f.queries = append(f.queries, recordedQuery{Query: query, Start: r.Start, End: r.End})
	f.mu.Unlock()
	if f.rangeFn == nil {
		return model.Matrix{}, nil, nil
	}
	return f.rangeFn(query, r), nil, nil
}

func (f *fakePromAPI) LabelValues(_ context.Context, _ string, _ []string) (model.LabelValues, prom.Warnings, error) {
	return f.labelVals, nil, nil
}

func (f *fakePromAPI) recorded() []recordedQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedQuery(nil), f.queries...)
}

// mkStream builds one series whose points start at r.Start with the given
// step, so the values land inside whatever window the handler asked for.
func mkStream(labels map[string]string, start time.Time, stepSec int64, vals []float64) *model.SampleStream {
	metric := model.Metric{}
	for k, v := range labels {
		metric[model.LabelName(k)] = model.LabelValue(v)
	}
	pairs := make([]model.SamplePair, len(vals))
	for i, v := range vals {
		pairs[i] = model.SamplePair{
			Timestamp: model.Time(start.Add(time.Duration(int64(i)*stepSec)*time.Second).Unix() * 1000),
			Value:     model.SampleValue(v),
		}
	}
	return &model.SampleStream{Metric: metric, Values: pairs}
}

const analyzeTestPayload = `{
	"var":[
		{"name":"prom","type":"datasource","definition":"prometheus","defaultValue":1},
		{"name":"ident","type":"query","definition":"label_values(cpu_usage_active, ident)","multi":true,"allOption":true,"datasource":{"cate":"prometheus","value":"${prom}"}}
	],
	"panels":[
		{"id":"row-1","type":"row","name":"概览"},
		{"id":"p1","type":"timeseries","name":"CPU使用率","datasourceCate":"prometheus","datasourceValue":"${prom}",
		 "targets":[{"refId":"A","expr":"cpu_usage_active{ident=~\"$ident\"}","legend":"{{ident}}"}]},
		{"id":"p2","type":"stat","name":"在线主机数","datasourceCate":"prometheus","datasourceValue":"${prom}",
		 "targets":[{"refId":"A","expr":"count(up)"}]},
		{"id":"p3","type":"table","name":"明细表","datasourceCate":"prometheus",
		 "targets":[{"refId":"A","expr":"whatever"}]}
	]
}`

// newAnalyzeFake wires the canonical scenario: web01 carries a fresh spike in
// the current window (clean yesterday), web02 is steady in both, count(up) is
// a flat constant.
func newAnalyzeFake() *fakePromAPI {
	wiggle := func(n int, base float64) []float64 {
		vals := make([]float64, n)
		for i := range vals {
			vals[i] = base
			if i%2 == 1 {
				vals[i] = base + base*0.001
			}
		}
		return vals
	}
	fake := &fakePromAPI{labelVals: model.LabelValues{"web01", "web02"}}
	fake.rangeFn = func(query string, r prom.Range) model.Matrix {
		prevWindow := r.End.Before(time.Now().Add(-30 * time.Minute))
		stepSec := int64(60)
		if strings.Contains(query, "cpu_usage_active") {
			web01 := wiggle(40, 20)
			if !prevWindow {
				web01[25] = 96 // today-only spike
			}
			return model.Matrix{
				mkStream(map[string]string{"__name__": "cpu_usage_active", "ident": "web01"}, r.Start, stepSec, web01),
				mkStream(map[string]string{"__name__": "cpu_usage_active", "ident": "web02"}, r.Start, stepSec, wiggle(40, 50)),
			}
		}
		// count(up): flat constant in both windows
		flat := make([]float64, 40)
		for i := range flat {
			flat[i] = 3
		}
		return model.Matrix{mkStream(map[string]string{}, r.Start, stepSec, flat)}
	}
	return fake
}

func newAnalyzeDeps(t *testing.T, fake *fakePromAPI) *aiagent.ToolDeps {
	t.Helper()
	deps := newDashboardTestDeps(t)
	deps.GetPromClient = func(dsId int64) prom.API {
		if dsId == 1 {
			return fake
		}
		return nil
	}
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Board{Id: 51, GroupId: 1, Name: "Linux 主机监控"}).Error)
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, analyzeTestPayload))
	return deps
}

// TestGetDashboardData_EndToEnd drives the full pipeline: variable expansion
// via label_values, row-group flattening, dual-window queries, screening, and
// the layered digest.
func TestGetDashboardData_EndToEnd(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	out, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h"},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	// Layered digest structure.
	require.Contains(t, out, "Linux 主机监控")
	require.Contains(t, out, "⚠ 可疑曲线", "the injected spike must surface")
	require.Contains(t, out, "[概览/CPU使用率] web01", "suspicious entry carries row group + legend")
	require.Contains(t, out, "突变", "spike detection mark must be rendered")
	require.Contains(t, out, "点:", "suspicious curves carry sample points")
	require.Contains(t, out, "✓ 正常曲线摘要")
	require.Contains(t, out, "web02", "the steady series lands in the normal section")
	require.Contains(t, out, "平直线 1 条", "count(up) is constant → flat bucket")
	require.Contains(t, out, "跳过 1 个 table 面板")
	require.NotContains(t, out, "whatever", "table panel must not be queried")

	// Variable expansion: label_values produced (web01|web02) into the expr.
	queries := fake.recorded()
	joined := ""
	for _, q := range queries {
		joined += q.Query + "\n"
	}
	require.Contains(t, joined, `cpu_usage_active{ident=~"(web01|web02)"}`, "label_values expansion must be substituted")
	require.NotContains(t, joined, "$ident", "no unresolved variable may reach the datasource")

	// Dual-window: every expr is queried twice (current + shifted).
	var cpuCur, cpuPrev int
	for _, q := range queries {
		if !strings.Contains(q.Query, "cpu_usage_active") {
			continue
		}
		if q.End.Before(time.Now().Add(-30 * time.Minute)) {
			cpuPrev++
		} else {
			cpuCur++
		}
	}
	require.Equal(t, 1, cpuCur, "one current-window query per target")
	require.Equal(t, 1, cpuPrev, "one shifted-window query per target")
}

// TestGetDashboardData_VarsOverride locks the priority order: an explicit
// vars argument beats label_values expansion.
func TestGetDashboardData_VarsOverride(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	_, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h", "vars": `{"ident":"web01"}`},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	for _, q := range fake.recorded() {
		require.NotContains(t, q.Query, "web02", "vars override must narrow the selector")
	}
	joined := ""
	for _, q := range fake.recorded() {
		joined += q.Query + "\n"
	}
	require.Contains(t, joined, `cpu_usage_active{ident=~"web01"}`)
}

// TestGetDashboardData_UnknownVarKeyRejected: a vars key that matches no
// dashboard variable (e.g. "indent" for "ident") must error instead of being
// silently dropped — otherwise the real variable falls back to full
// label_values expansion and the report covers the whole fleet while claiming
// it is scoped.
func TestGetDashboardData_UnknownVarKeyRejected(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	_, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h", "vars": `{"indent":"web01"}`},
		map[string]string{"user_id": "1"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "indent", "the offending key must be named")
	require.Contains(t, err.Error(), "ident", "available variable names must be listed")
	require.Empty(t, fake.recorded(), "no datasource query may run on a rejected vars argument")
}

// TestParseUserVars_RejectsUndecodableValues: a vars value the parser cannot
// turn into selector strings (bool/null/object/empty array) must error, never
// be silently dropped — a dropped key would slip past the unknown-key guard
// (which sees only the parsed map) and leave the variable on full .*
// expansion, the very failure mode that guard exists to prevent.
func TestParseUserVars_RejectsUndecodableValues(t *testing.T) {
	for name, raw := range map[string]string{
		"bool value":     `{"ident": true}`,
		"null value":     `{"ident": null}`,
		"object value":   `{"ident": {"v": "web01"}}`,
		"empty array":    `{"ident": []}`,
		"bool in array":  `{"ident": ["web01", true]}`,
		"empty string":   `{"ident": ""}`,
		"blank string":   `{"ident": "  "}`,
		"empty in array": `{"ident": ["web01", ""]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseUserVars(raw)
			require.Error(t, err)
			require.Contains(t, err.Error(), "ident", "the offending key must be named")
		})
	}

	got, err := parseUserVars(`{"ident": ["web01", 2]}`)
	require.NoError(t, err)
	require.Equal(t, []string{"web01", "2"}, got["ident"], "string/number arrays must still decode")
}

// TestParsePanelIDs_NumericArray: panel ids are stringified via stringVal, so a
// numeric JSON array [1,2] must decode to the same keys as ["1","2"]. A naive
// []string unmarshal would reject the numeric form and the comma-split fallback
// would mangle the raw JSON into garbage keys, filtering out every panel.
func TestParsePanelIDs_NumericArray(t *testing.T) {
	for name, raw := range map[string]string{
		"numeric array": `[1, 2]`,
		"string array":  `["1", "2"]`,
		"mixed array":   `[1, "2"]`,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := parsePanelIDs(raw)
			require.NoError(t, err)
			require.Equal(t, map[string]bool{"1": true, "2": true}, got)
		})
	}

	// a bare comma-separated list is still tolerated
	got, err := parsePanelIDs("p1, p2")
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"p1": true, "p2": true}, got)
}

// TestGetDashboardData_PanelIdsFilter limits the run to the listed panels.
func TestGetDashboardData_PanelIdsFilter(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	out, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h", "panel_ids": `["p2"]`},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	require.Contains(t, out, "在线主机数")
	for _, q := range fake.recorded() {
		require.NotContains(t, q.Query, "cpu_usage_active", "filtered-out panels must not be queried")
	}
}

// TestGetDashboardData_VanishedOnEmptyCurrentWindow: when the current window
// returns NO data at all but the comparison window had series, the digest must
// still carry the "昨日有、今日无" vanished signal — an entirely-empty target is
// the strongest instance-loss case, not just a neutral empty query.
func TestGetDashboardData_VanishedOnEmptyCurrentWindow(t *testing.T) {
	fake := newAnalyzeFake()
	base := fake.rangeFn
	fake.rangeFn = func(query string, r prom.Range) model.Matrix {
		if r.End.Before(time.Now().Add(-30 * time.Minute)) {
			return base(query, r) // prev window: series exist
		}
		return model.Matrix{} // current window: everything vanished
	}
	deps := newAnalyzeDeps(t, fake)

	out, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h"},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	require.Contains(t, out, "查询无数据", "empty current window still counts as empty queries")
	require.Contains(t, out, "昨日有、今日无的曲线", "vanished section must survive an all-empty current window")
	require.Contains(t, out, "web01")
	require.Contains(t, out, "web02")
}

// TestGetDashboardData_RotatingLabelsNotVanished is the regression for the
// fingerprint-match false positive: a panel whose series carry rotating labels
// (pod names, ephemeral instance ids) has ZERO cross-window overlap even when
// perfectly healthy, so reporting yesterday's series as "昨日有今日无" would
// fabricate a fleet-wide outage. Both windows are fully populated; nothing must
// land in the vanished section.
func TestGetDashboardData_RotatingLabelsNotVanished(t *testing.T) {
	wiggle := func(n int, base float64) []float64 {
		vals := make([]float64, n)
		for i := range vals {
			vals[i] = base
			if i%2 == 1 {
				vals[i] = base + base*0.001
			}
		}
		return vals
	}
	fake := &fakePromAPI{labelVals: model.LabelValues{"web01", "web02"}}
	fake.rangeFn = func(query string, r prom.Range) model.Matrix {
		prevWindow := r.End.Before(time.Now().Add(-30 * time.Minute))
		stepSec := int64(60)
		if strings.Contains(query, "cpu_usage_active") {
			// Pod identities rotate between yesterday and today: zero overlap,
			// but both windows are fully populated and healthy.
			a, b := "today-aaa", "today-bbb"
			if prevWindow {
				a, b = "yday-ccc", "yday-ddd"
			}
			return model.Matrix{
				mkStream(map[string]string{"__name__": "cpu_usage_active", "pod": a}, r.Start, stepSec, wiggle(40, 20)),
				mkStream(map[string]string{"__name__": "cpu_usage_active", "pod": b}, r.Start, stepSec, wiggle(40, 50)),
			}
		}
		flat := make([]float64, 40)
		for i := range flat {
			flat[i] = 3
		}
		return model.Matrix{mkStream(map[string]string{}, r.Start, stepSec, flat)}
	}
	deps := newAnalyzeDeps(t, fake)

	out, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h"},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	require.NotContains(t, out, "昨日有、今日无", "rotating-label series must not be reported as vanished")
	require.NotContains(t, out, "yday-ccc")
	require.NotContains(t, out, "yday-ddd")
}

// TestGetDashboardData_NoAnalyzablePanels errors loudly (with the cate
// breakdown) instead of returning an empty digest.
func TestGetDashboardData_NoAnalyzablePanels(t *testing.T) {
	deps := newDashboardTestDeps(t)
	deps.GetPromClient = func(int64) prom.API { return nil }
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Board{Id: 52, GroupId: 1, Name: "mysql-board"}).Error)
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 52,
		`{"panels":[{"id":"p1","type":"timeseries","name":"rows","datasourceCate":"mysql","targets":[{"refId":"A","expr":"select 1"}]}]}`))

	_, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(52)},
		map[string]string{"user_id": "1"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "mysql", "the error must name the unsupported cate")
}

// TestGetDashboardData_InvalidTimeRange rejects nonsense instead of silently
// defaulting.
func TestGetDashboardData_InvalidTimeRange(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	_, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "yesterday"},
		map[string]string{"user_id": "1"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "time_range")
}

// TestApplyVarReg mirrors the FE's filterOptionsByReg semantics: the analysis
// must expand a variable to the same value set the dashboard actually shows.
func TestApplyVarReg(t *testing.T) {
	vals := []string{"prod-a", "prod-b", "dev-c", ""}

	// Bare form is anchored ^…$ like the FE's stringToRegex.
	out, ok := applyVarReg("prod-.*", vals)
	require.True(t, ok)
	require.Equal(t, []string{"prod-a", "prod-b"}, out)

	// /.../ form is unanchored.
	out, ok = applyVarReg("/^prod-/", vals)
	require.True(t, ok)
	require.Equal(t, []string{"prod-a", "prod-b"}, out)

	// First capture group replaces the value (host extraction etc.).
	out, ok = applyVarReg("/^prod-(.*)$/", vals)
	require.True(t, ok)
	require.Equal(t, []string{"a", "b"}, out)

	// A group named "value" wins over positional groups.
	out, ok = applyVarReg("/^(prod)-(?<value>.*)$/", vals)
	require.True(t, ok)
	require.Equal(t, []string{"a", "b"}, out)

	// i flag.
	out, ok = applyVarReg("/^PROD-/i", vals)
	require.True(t, ok)
	require.Equal(t, []string{"prod-a", "prod-b"}, out)

	// Duplicate post-capture values are deduped (FE unionBy).
	out, ok = applyVarReg(`/^(prod|dev)-/`, []string{"prod-a", "prod-b", "dev-c"})
	require.True(t, ok)
	require.Equal(t, []string{"prod", "dev"}, out)

	// JS-only syntax (lookahead) doesn't compile under RE2 → unfiltered + flag.
	out, ok = applyVarReg("/^(?!template)/", vals)
	require.False(t, ok)
	require.Equal(t, vals, out)

	// Empty reg is a no-op.
	out, ok = applyVarReg("  ", vals)
	require.True(t, ok)
	require.Equal(t, vals, out)
}

// TestGetDashboardData_BuiltinVars: the FE's built-in time/step placeholders
// ($__rate_interval, $__interval, $__range, ... — getBuiltInVariables in
// replaceTemplateVariables.ts) appear in dozens of builtin integration
// dashboards. They must be substituted with the actual query step/window;
// unsubstituted they reach Prometheus verbatim and the panel always fails.
func TestGetDashboardData_BuiltinVars(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	payload := strings.Replace(analyzeTestPayload,
		`"expr":"cpu_usage_active{ident=~\"$ident\"}"`,
		`"expr":"rate(cpu_usage_active{ident=~\"$ident\"}[$__rate_interval])"`, 1)
	require.NotEqual(t, analyzeTestPayload, payload)
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, payload))

	_, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h"},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	joined := ""
	for _, q := range fake.recorded() {
		joined += q.Query + "\n"
	}
	require.NotContains(t, joined, "$__", "built-in vars must never reach the datasource")
	// 1h window → step = max(15, 3600/240) = 15s → rate window 4×step = 60s,
	// exactly like the FE (getDefaultStepByTime floored at minStep 15s,
	// `${interval*4}s`).
	require.Contains(t, joined, "[60s]")
}

// TestBuiltinVarValues locks the FE value formats (getBuiltInVariables):
// durations carry the s suffix, _ms/_s/epoch forms are bare numbers, __from is
// epoch millis.
func TestBuiltinVarValues(t *testing.T) {
	vals := builtinVarValues(1000, 4600, 60)
	require.Equal(t, "60s", vals["__interval"])
	require.Equal(t, "60000", vals["__interval_ms"])
	require.Equal(t, "240s", vals["__rate_interval"])
	require.Equal(t, "3600s", vals["__range"])
	require.Equal(t, "3600", vals["__range_s"])
	require.Equal(t, "3600000", vals["__range_ms"])
	require.Equal(t, "1000000", vals["__from"])
	require.Equal(t, "1000", vals["__from_date_seconds"])
	require.Equal(t, "4600000", vals["__to"])
	require.Equal(t, "1970-01-01T01:16:40.000Z", vals["__to_date_iso"])
}

// TestGetDashboardData_AllValueRespected: a multi-select variable resting on
// the 全选 state substitutes its custom allValue literal (FE ajustData.ts) —
// label_values expansion would scope the analysis differently from what the
// live dashboard renders.
func TestGetDashboardData_AllValueRespected(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	payload := strings.Replace(analyzeTestPayload,
		`"multi":true,"allOption":true,`,
		`"multi":true,"allOption":true,"allValue":".+",`, 1)
	require.NotEqual(t, analyzeTestPayload, payload)
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, payload))

	_, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h"},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	joined := ""
	for _, q := range fake.recorded() {
		joined += q.Query + "\n"
	}
	require.Contains(t, joined, `cpu_usage_active{ident=~".+"}`, "the 全选 state must substitute allValue literally")
	require.NotContains(t, joined, "web01|web02", "allValue must pre-empt label_values expansion")

	// An explicit vars argument still beats allValue.
	fake2 := newAnalyzeFake()
	deps.GetPromClient = func(dsId int64) prom.API {
		if dsId == 1 {
			return fake2
		}
		return nil
	}
	_, err = getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h", "vars": `{"ident":"web01"}`},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)
	joined = ""
	for _, q := range fake2.recorded() {
		joined += q.Query + "\n"
	}
	require.Contains(t, joined, `cpu_usage_active{ident=~"web01"}`, "explicit vars must override allValue")
}

// TestGetDashboardData_SingleSelectVarTakesHead: with no default, a non-multi
// query variable rests on its FIRST option (FE getValueByOptions.ts) —
// expanding the full label_values set would analyze the whole fleet while the
// live dashboard renders just one host.
func TestGetDashboardData_SingleSelectVarTakesHead(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	payload := strings.Replace(analyzeTestPayload, `"multi":true,"allOption":true,`, ``, 1)
	require.NotEqual(t, analyzeTestPayload, payload)
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, payload))

	_, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h"},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	joined := ""
	for _, q := range fake.recorded() {
		joined += q.Query + "\n"
	}
	require.Contains(t, joined, `cpu_usage_active{ident=~"web01"}`, "single-select must rest on the first option")
	require.NotContains(t, joined, "web02", "single-select must not expand to the full option set")
}

// TestGetDashboardData_DatasourceVarDeclaredLast: the FE resolves variables by
// dependency topo-sort (VariableManagerContext), so a query variable may
// legally reference a datasource variable declared after it. Single-pass
// declaration-order resolution would silently run label_values against the
// fallback datasource (here: none → ".*" full-fleet expansion).
func TestGetDashboardData_DatasourceVarDeclaredLast(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	const payload = `{
		"var":[
			{"name":"ident","type":"query","definition":"label_values(cpu_usage_active, ident)","multi":true,"allOption":true,"datasource":{"cate":"prometheus","value":"${prom}"}},
			{"name":"prom","type":"datasource","definition":"prometheus","defaultValue":1}
		],
		"panels":[
			{"id":"p1","type":"timeseries","name":"CPU使用率","datasourceCate":"prometheus","datasourceValue":"${prom}",
			 "targets":[{"refId":"A","expr":"cpu_usage_active{ident=~\"$ident\"}","legend":"{{ident}}"}]}
		]
	}`
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, payload))

	_, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h"},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	joined := ""
	for _, q := range fake.recorded() {
		joined += q.Query + "\n"
	}
	require.Contains(t, joined, `cpu_usage_active{ident=~"(web01|web02)"}`,
		"a datasource var declared after its referencing query var must still scope the expansion")
	require.NotContains(t, joined, `ident=~".*"`, "the resolver must not fall back to .* on a resolvable board")
}

// TestGetDashboardData_CustomConstantVars: FE parity for the non-query
// variable types — custom rests on the lexicographically-first option
// (Custom.tsx comma-split + sortBy + getValueByOptions head), constant
// substitutes its whole definition. Neither may fall to ".*": shipped boards
// use custom vars INSIDE range selectors (rate(x[$interval])) where ".*" is a
// PromQL parse error that fails every panel.
func TestGetDashboardData_CustomConstantVars(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	const payload = `{
		"var":[
			{"name":"prom","type":"datasource","definition":"prometheus","defaultValue":1},
			{"name":"interval","type":"custom","definition":"1s,5s,1m,5m,1h,6h,1d"},
			{"name":"job","type":"constant","definition":"node-exporter"}
		],
		"panels":[
			{"id":"p1","type":"timeseries","name":"QPS","datasourceCate":"prometheus","datasourceValue":"${prom}",
			 "targets":[{"refId":"A","expr":"rate(cpu_usage_active{job=\"$job\"}[$interval])"}]}
		]
	}`
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, payload))

	_, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h"},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	joined := ""
	for _, q := range fake.recorded() {
		joined += q.Query + "\n"
	}
	require.Contains(t, joined, `rate(cpu_usage_active{job="node-exporter"}[1d])`,
		"custom rests on the sorted head (FE sortBy → 1d), constant substitutes its definition")
	require.NotContains(t, joined, ".*", "no variable may fall back to .* on this board")
}

// TestGetDashboardData_PrevWindowBuiltinVars: the 同比 query must substitute
// the absolute builtin time vars with ITS OWN window — reusing the current
// window's $__to would embed today's timestamps into yesterday's query.
func TestGetDashboardData_PrevWindowBuiltinVars(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	payload := strings.Replace(analyzeTestPayload,
		`"expr":"cpu_usage_active{ident=~\"$ident\"}"`,
		`"expr":"cpu_usage_active{ident=~\"$ident\"} @ ${__to_date_seconds}"`, 1)
	require.NotEqual(t, analyzeTestPayload, payload)
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, payload))

	_, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h"},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	checked := 0
	for _, q := range fake.recorded() {
		if !strings.Contains(q.Query, "cpu_usage_active") {
			continue
		}
		idx := strings.LastIndex(q.Query, "@ ")
		require.Greater(t, idx, 0, "expr must carry the substituted @ timestamp: %s", q.Query)
		sec, err := strconv.ParseInt(strings.TrimSpace(q.Query[idx+2:]), 10, 64)
		require.NoError(t, err)
		require.Equal(t, q.End.Unix(), sec, "builtin __to must carry the query's OWN window end")
		checked++
	}
	require.Equal(t, 2, checked, "current + shifted window queries")
}

// TestGetDashboardData_ScalarAllDefaultIsLiteral: FE ajustData only treats the
// ARRAY ['all'] as the select-all sentinel — a SCALAR defaultValue "all" fails
// its _.isEqual(value, ['all']) check and substitutes the literal text "all".
// Expanding the full option set here would scope the analysis to the fleet
// while the live board queries ident=~"all".
func TestGetDashboardData_ScalarAllDefaultIsLiteral(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	payload := strings.Replace(analyzeTestPayload,
		`"multi":true,"allOption":true,`,
		`"defaultValue":"all",`, 1)
	require.NotEqual(t, analyzeTestPayload, payload)
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, payload))

	_, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h"},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	joined := ""
	for _, q := range fake.recorded() {
		joined += q.Query + "\n"
	}
	require.Contains(t, joined, `cpu_usage_active{ident=~"all"}`, "scalar 'all' substitutes literally, like the FE")
	require.NotContains(t, joined, "web01", "scalar 'all' must not expand the option set")

	// The ARRAY form ['all'] IS the select-all sentinel → joins the options.
	payload = strings.Replace(analyzeTestPayload,
		`"multi":true,"allOption":true,`,
		`"defaultValue":["all"],`, 1)
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, payload))
	fake2 := newAnalyzeFake()
	deps.GetPromClient = func(dsId int64) prom.API {
		if dsId == 1 {
			return fake2
		}
		return nil
	}
	_, err = getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h"},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)
	joined = ""
	for _, q := range fake2.recorded() {
		joined += q.Query + "\n"
	}
	require.Contains(t, joined, `cpu_usage_active{ident=~"(web01|web02)"}`, "array ['all'] rests on 全选 → full option join")
}

// TestGetDashboardData_CascadingVarDeclaredOutOfOrder: the FE topo-sorts ALL
// variable dependencies, so a query var may reference another query var
// declared after it; single-pass declaration-order resolution would leave the
// $ref literal in the label_values matcher → query fails → ".*" full-fleet
// fallback.
func TestGetDashboardData_CascadingVarDeclaredOutOfOrder(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	const payload = `{
		"var":[
			{"name":"ident","type":"query","definition":"label_values(cpu_usage_active{env=\"$env\"}, ident)","multi":true,"allOption":true,"datasource":{"cate":"prometheus","value":"${prom}"}},
			{"name":"env","type":"custom","definition":"prod"},
			{"name":"prom","type":"datasource","definition":"prometheus","defaultValue":1}
		],
		"panels":[
			{"id":"p1","type":"timeseries","name":"CPU使用率","datasourceCate":"prometheus","datasourceValue":"${prom}",
			 "targets":[{"refId":"A","expr":"cpu_usage_active{ident=~\"$ident\"}","legend":"{{ident}}"}]}
		]
	}`
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, payload))

	_, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h"},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	joined := ""
	for _, q := range fake.recorded() {
		joined += q.Query + "\n"
	}
	require.Contains(t, joined, `cpu_usage_active{ident=~"(web01|web02)"}`,
		"ident must resolve after its dependency env, despite declaration order")
	require.NotContains(t, joined, `ident=~".*"`, "out-of-order deps must not fall back to .*")
	require.NotContains(t, joined, "$env", "no unresolved ref may reach the datasource")
}

// TestGetDashboardData_PanelIdsMatchesRowId: passing a row's id selects every
// panel in that section — get_dashboard_detail surfaces rows with their ids,
// and "analyze this section" is the natural follow-up on a truncated digest.
func TestGetDashboardData_PanelIdsMatchesRowId(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	const payload = `{
		"var":[{"name":"prom","type":"datasource","definition":"prometheus","defaultValue":1}],
		"panels":[
			{"id":"row-1","type":"row","name":"概览"},
			{"id":"p1","type":"timeseries","name":"CPU使用率","datasourceCate":"prometheus","datasourceValue":"${prom}",
			 "targets":[{"refId":"A","expr":"cpu_usage_active","legend":"{{ident}}"}]},
			{"id":"row-2","type":"row","name":"主机"},
			{"id":"p2","type":"stat","name":"在线主机数","datasourceCate":"prometheus","datasourceValue":"${prom}",
			 "targets":[{"refId":"A","expr":"count(up)"}]}
		]
	}`
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, payload))

	out, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h", "panel_ids": `["row-1"]`},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)
	require.Contains(t, out, "CPU使用率", "panels under the row must be analyzed")

	for _, q := range fake.recorded() {
		require.NotContains(t, q.Query, "count(up)", "panels in other sections must be filtered out")
	}
}

// TestParseTimeRange_RejectsUnknownUnits: an unrecognized unit must yield
// (0,0) so callers report invalid time_range — the old default branch
// silently treated "30s" as 30 HOURS, analyzing a window 3600× the requested
// one while the report header still echoed "30s".
func TestParseTimeRange_RejectsUnknownUnits(t *testing.T) {
	for _, tr := range []string{"1y", "3mo", "100ms", "yesterday", "h", "0m", ""} {
		stime, etime := parseTimeRange(tr)
		require.Zero(t, stime, "time_range %q must be rejected", tr)
		require.Zero(t, etime, "time_range %q must be rejected", tr)
	}
	for tr, want := range map[string]int64{
		"90s": 90, "15m": 900, "1h": 3600, "24hour": 86400, "7d": 604800, "2w": 1209600,
	} {
		stime, etime := parseTimeRange(tr)
		require.NotZero(t, stime, "time_range %q must parse", tr)
		require.Equal(t, want, etime-stime, "time_range %q window", tr)
	}
}

// TestGetDashboardData_VarRegFilter: a variable whose reg hides part of the
// label_values result must expand to the filtered set only — the analysis
// covers what the dashboard shows, not the raw fleet.
func TestGetDashboardData_VarRegFilter(t *testing.T) {
	fake := newAnalyzeFake()
	deps := newAnalyzeDeps(t, fake)

	regPayload := strings.Replace(analyzeTestPayload,
		`"definition":"label_values(cpu_usage_active, ident)"`,
		`"definition":"label_values(cpu_usage_active, ident)","reg":"/^web01$/"`, 1)
	require.NotEqual(t, analyzeTestPayload, regPayload)
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, regPayload))

	_, err := getDashboardDataTool(context.Background(), deps,
		map[string]interface{}{"id": float64(51), "time_range": "1h"},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	joined := ""
	for _, q := range fake.recorded() {
		joined += q.Query + "\n"
	}
	require.Contains(t, joined, `cpu_usage_active{ident=~"web01"}`, "reg must narrow the expansion")
	require.NotContains(t, joined, "web02", "reg-excluded values must not be queried")
}
