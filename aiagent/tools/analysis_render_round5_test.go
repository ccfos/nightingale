package tools

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestRenderAnalysisDigest_RanksAndCapsSuspiciousCurves(t *testing.T) {
	curves := make([]curveEntry, 0, analyzeMaxSuspicious+2)
	// Reverse insertion order makes the lexical tie-break observable.
	for i := analyzeMaxSuspicious; i >= 0; i-- {
		curves = append(curves, curveEntry{
			Panel:    &analysisPanel{ID: fmt.Sprint(i), Name: fmt.Sprintf("Panel-%02d", i)},
			Findings: seriesFindings{Suspicious: true, Score: 10, Stats: seriesStats{Avg: 1, Last: 1}},
		})
	}
	curves = append(curves, curveEntry{
		Panel:    &analysisPanel{ID: "highest", Name: "Z-highest"},
		Findings: seriesFindings{Suspicious: true, Score: 100, Stats: seriesStats{Flat: true}},
	})

	out := renderAnalysisDigest(&models.Board{Id: 7, Name: "Operations"}, "1h", 1000, 4600, 86400, 3600,
		&resolvedVars{}, curves, nil, nil, nil, 0, 0, nil, 0, 0)

	require.Contains(t, out, "[Z-highest]")
	require.Contains(t, out, "[Panel-00]")
	require.Contains(t, out, "[Panel-01]")
	require.Less(t, strings.Index(out, "[Z-highest]"), strings.Index(out, "[Panel-00]"))
	require.Less(t, strings.Index(out, "[Panel-00]"), strings.Index(out, "[Panel-01]"))
	require.NotContains(t, out, fmt.Sprintf("[Panel-%02d]", analyzeMaxSuspicious))
	require.Contains(t, out, "另有 2 条可疑曲线未展示")
	require.Contains(t, out, "今日值恒定")
	require.NotContains(t, out, "平直线", "a suspicious constant series belongs in the warning section")
}

func TestRenderAnalysisDigest_DeduplicatesSortsAndCapsFailures(t *testing.T) {
	failures := []panelFailure{{Panel: "Panel-00", Reason: "unavailable"}}
	for i := 20; i >= 0; i-- {
		failures = append(failures, panelFailure{Panel: fmt.Sprintf("Panel-%02d", i), Reason: "unavailable"})
	}

	out := renderAnalysisDigest(&models.Board{Name: "Operations"}, "1h", 1000, 4600, 86400, 3600,
		&resolvedVars{}, nil, failures, nil, nil, 0, 0, nil, 0, 0)

	require.Equal(t, 1, strings.Count(out, `面板 "Panel-00" 查询失败`))
	require.Less(t, strings.Index(out, `面板 "Panel-00"`), strings.Index(out, `面板 "Panel-01"`))
	require.Contains(t, out, `面板 "Panel-19" 查询失败: unavailable`)
	require.NotContains(t, out, `面板 "Panel-20"`)
	require.Contains(t, out, "另有 1 条查询失败未展示")
}

func TestRenderAnalysisDigest_ExplainsIncompleteAndSkippedQueries(t *testing.T) {
	vanished := []string{"one", "two", "three", "four", "five", "six", "seven"}
	curves := make([]curveEntry, 12)
	for i := range curves {
		curves[i] = curveEntry{
			Panel:    &analysisPanel{Name: fmt.Sprintf("Flat-%02d", i)},
			Findings: seriesFindings{Stats: seriesStats{Flat: true, Last: 4}},
		}
	}

	out := renderAnalysisDigest(&models.Board{Name: "Operations"}, "1h", 1000, 4600, 86400, 3600,
		&resolvedVars{notes: []string{"host=web-1"}}, curves, nil,
		map[string]int{"text": 2}, map[string]int{"mysql": 3}, 4, 5, vanished, 7, 6)

	require.Contains(t, out, "变量: host=web-1")
	require.Contains(t, out, "平直线 12 条")
	require.Contains(t, out, "Flat-09")
	require.NotContains(t, out, "Flat-10")
	require.Contains(t, out, "跳过 2 个 text 面板")
	require.Contains(t, out, "跳过 3 个 mysql 数据源面板")
	require.Contains(t, out, "4 个查询无数据")
	require.Contains(t, out, "5 个查询的同比窗口取数失败")
	require.Contains(t, out, "one, two, three, four, five 等2条(共7条)")
	require.Contains(t, out, "6 个查询因分析总预算")
	require.Contains(t, out, "结果不完整")
}

func TestRenderSuspiciousEntry_ExplainsZeroBaselineAndTrendDirection(t *testing.T) {
	cases := []struct {
		name  string
		trend float64
		spike float64
		want  []string
	}{
		{
			name: "zero baseline", trend: relPctCap, spike: relPctCap,
			want: []string{"突变(自≈0)→5", "趋势↑(自≈0)", "同比avg自≈0↑/max自≈0↑"},
		},
		{name: "decline", trend: -80, spike: -75, want: []string{"突变-75%", "趋势↓80%"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			curve := curveEntry{
				Panel: &analysisPanel{Name: "Requests", Unit: "req/s"}, Legend: "api",
				Ts: []int64{1000, 1060}, Vals: []float64{1, 5},
				Findings: seriesFindings{
					Stats:  seriesStats{Avg: 3, Min: 1, Max: 5, MaxTs: 1060, Last: 5},
					Spikes: []pointMark{{Ts: 1060, Val: 5, Pct: tc.spike}}, TrendHit: true, TrendPct: tc.trend,
					YoY: &yoyCompare{Hit: true, AvgPct: relPctCap, MaxPct: relPctCap}, Periodic: true,
				},
			}

			out := renderSuspiciousEntry(curve, 86400)

			for _, text := range tc.want {
				require.Contains(t, out, text)
			}
			require.Contains(t, out, "单位:req/s")
			require.Contains(t, out, "疑似周期性(昨日同现)")
			require.NotContains(t, out, "99900%", "the zero-baseline cap is not a measured percentage")
		})
	}
}

func TestRenderNormalSection_ExplainsPeriodicSpikesAndTrends(t *testing.T) {
	panel := &analysisPanel{ID: "cpu", Name: "CPU", Group: "Hosts"}
	curves := []curveEntry{
		{
			Panel: panel, Legend: "periodic",
			Findings: seriesFindings{
				Stats: seriesStats{Avg: 1e6}, Periodic: true,
				Spikes: []pointMark{{Pct: 50}, {Pct: -200}}, YoY: &yoyCompare{AvgPct: 5},
			},
		},
		{Panel: panel, Legend: "rising", Findings: seriesFindings{Stats: seriesStats{Avg: 1e9}, TrendPct: 20}},
		{Panel: panel, Legend: "falling", Findings: seriesFindings{Stats: seriesStats{Avg: -2000}, TrendPct: -25}},
	}

	out := renderNormalSection(curves, 3600)

	require.Equal(t, "[Hosts/CPU] periodic: avg=1M(+5%) 周期性尖峰(-200%) | "+
		"rising: avg=1G 趋势↑20% | falling: avg=-2k 趋势↓25%\n", out)
}

func TestRenderAnalysisDigest_TruncatesAtValidTextBoundary(t *testing.T) {
	curves := []curveEntry{{
		Panel: &analysisPanel{Name: "CPU"}, Legend: strings.Repeat("服务", analyzeMaxOutputBytes),
		Findings: seriesFindings{Suspicious: true, Score: 100},
	}}

	out := renderAnalysisDigest(&models.Board{Name: "Operations"}, "1h", 1000, 4600, 86400, 3600,
		&resolvedVars{}, curves, nil, nil, nil, 0, 0, nil, 0, 0)

	require.True(t, utf8.ValidString(out))
	require.Contains(t, out, "输出超过 32KB 已截断")
	require.Less(t, len(out), analyzeMaxOutputBytes+200)
	require.Contains(t, out, "panel_ids 分批分析")
}
