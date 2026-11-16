package tools

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"math"
	"strings"
	"testing"
)

func TestR8eAnalysisVariableDefaults(t *testing.T) {
	for _, tc := range []struct {
		input interface{}
		want  []string
	}{{" a ", []string{"a"}}, {" ", nil}, {float64(3), []string{"3"}}, {[]interface{}{"a", "", float64(2), true}, []string{"a", "2"}}, {nil, nil}} {
		require.Equal(t, tc.want, defaultValueList(tc.input))
	}
	for _, raw := range []string{`{"x":" "}`, `{"x":[""]}`} {
		_, err := parseUserVars(raw)
		require.ErrorContains(t, err, "empty string")
	}
	configs := map[string]interface{}{"var": []interface{}{nil, map[string]interface{}{"name": "constant", "type": "custom", "definition": "b,a", "reg": "["}, map[string]interface{}{"name": "empty", "type": "custom", "definition": "b,a", "reg": "nomatch"}}}
	rv, err := resolveDashboardVars(context.Background(), &aiagent.ToolDeps{}, configs, nil, 1, 100, 200, 60)
	require.NoError(t, err)
	require.Equal(t, "a", rv.values["constant"])
	require.Equal(t, ".*", rv.values["empty"])
	require.Len(t, rv.notes, 2)
	require.Contains(t, strings.Join(rv.notes, " "), "RE2")
	var panels []analysisPanel
	collectAnalysisPanels([]interface{}{nil, map[string]interface{}{"name": "CPU", "targets": []interface{}{nil, map[string]interface{}{"hide": true, "expr": "hidden"}, map[string]interface{}{"__mode__": "__expr__", "expr": "A+B"}, map[string]interface{}{"expr": " "}, map[string]interface{}{"expr": "up"}}}}, "", "", &panels)
	require.Len(t, panels, 1)
	require.Equal(t, []analysisTarget{{Expr: "up"}}, panels[0].Targets)
	require.Equal(t, int64(8), panelDatasourceID(analysisPanel{DatasourceValue: "${ds}"}, &resolvedVars{dsIDs: map[string]int64{"ds": 8}}, 1))
	ts, vals := samplePairsToSlices([]model.SamplePair{{Timestamp: 1000, Value: model.SampleValue(math.NaN())}, {Timestamp: 2000, Value: model.SampleValue(math.Inf(1))}, {Timestamp: 3000, Value: 2}})
	require.Equal(t, []int64{3}, ts)
	require.Equal(t, []float64{2}, vals)
}

func TestR8eAnalysisFormattingBoundaries(t *testing.T) {
	for _, tc := range []struct {
		value float64
		want  string
	}{{1e9, "1G"}, {-2e6, "-2M"}, {0.0001, "0.0001"}} {
		require.Equal(t, tc.want, humanVal(tc.value))
	}
	require.Equal(t, "100", trimZeros("100"))
	require.Equal(t, strings.Repeat("x", 200)+"...", trimErr(errors.New(strings.Repeat("x", 201))))
	require.Zero(t, round2(math.Inf(1)))
	require.Zero(t, round2(math.NaN()))
	normal := make([]curveEntry, 0)
	for i := 0; i < 12; i++ {
		normal = append(normal, curveEntry{Panel: &analysisPanel{Name: "CPU"}, Findings: seriesFindings{}})
	}
	out := renderNormalSection(normal, 3600)
	require.Equal(t, 12, strings.Count(out, "[CPU]"))
	p := &analysisPanel{Name: "Grouped"}
	normal = nil
	for i := 0; i < 12; i++ {
		normal = append(normal, curveEntry{Panel: p})
	}
	out = renderNormalSection(normal, 3600)
	require.Equal(t, 1, strings.Count(out, "[Grouped]"))
	require.Contains(t, out, "...")
}

func TestR8eRuleNormalizationDefensiveInputs(t *testing.T) {
	for _, tc := range []struct{ cate, want string }{{"loki", "logging"}, {"host", "host"}, {"future", "metric"}} {
		require.Equal(t, tc.want, prodForCate(tc.cate))
	}
	for _, tc := range []struct{ cur, op, want string }{{"up > 1", "bad", "invalid operator"}, {"up", "", "metric expression"}} {
		_, err := rebuildBakedPromQL(tc.cur, "", tc.op, 0, false)
		require.ErrorContains(t, err, tc.want)
	}
	normalizeQueryIntervals(nil)
	queries := []interface{}{nil, map[string]interface{}{"interval_unit": "min"}, map[string]interface{}{"interval": "bad", "interval_unit": "min"}, map[string]interface{}{"interval": int64(2), "interval_unit": "other"}, map[string]interface{}{"interval": float64(3)}}
	normalizeQueryIntervals(map[string]interface{}{"queries": queries})
	require.Empty(t, queries[1])
	require.Equal(t, map[string]interface{}{"interval": "bad"}, queries[2])
	require.Equal(t, 120, queries[3].(map[string]interface{})["interval"])
	require.Equal(t, 180, queries[4].(map[string]interface{})["interval"])
	for _, v := range []interface{}{int(2), int64(2), json.Number("2")} {
		require.Equal(t, 2, jsonInt(v))
	}
	require.Zero(t, jsonInt("2"))
	severity, expr := importedRuleHeadline(&models.AlertRule{Severity: 3, PromQl: " legacy "})
	require.Equal(t, 3, severity)
	require.Equal(t, "legacy", expr)
	severity, expr = importedRuleHeadline(&models.AlertRule{RuleConfigJson: map[string]interface{}{"severity": float64(2), "prom_ql": "up"}})
	require.Equal(t, 2, severity)
	require.Equal(t, "up", expr)
	card := importedAlertRuleCard(&models.AlertRule{AnnotationsJSON: map[string]string{"summary": "help"}}, "Ops", 0, "")
	require.Equal(t, "help", card["note"])
}

func TestR8eQueryVariableExpansionFailures(t *testing.T) {
	rv := &resolvedVars{values: map[string]string{}, dsIDs: map[string]int64{}}
	vm := map[string]interface{}{"name": "host", "definition": "label_values(up,host)", "datasource": map[string]interface{}{"value": float64(3)}}
	require.Nil(t, expandQueryVariable(context.Background(), &aiagent.ToolDeps{}, rv, vm, 1))
	deps := &aiagent.ToolDeps{GetPromClient: func(id int64) prom.API { require.Equal(t, int64(3), id); return nil }}
	require.Nil(t, expandQueryVariable(context.Background(), deps, rv, vm, 1))
	deps.GetPromClient = func(int64) prom.API { return &fakePromAPI{labelVals: model.LabelValues{"b", "a"}} }
	vm["reg"] = "["
	require.Equal(t, []string{"a", "b"}, expandQueryVariable(context.Background(), deps, rv, vm, 1))
	require.Len(t, rv.notes, 1)
	vm["definition"] = "unsupported"
	require.Nil(t, expandQueryVariable(context.Background(), deps, rv, vm, 1))
}
