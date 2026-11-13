package eval

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/evallog"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func round4EvalLog(t *testing.T) {
	t.Helper()
	// This package's tests do not initialize a center; restore the disabled singleton on cleanup.
	require.False(t, evallog.Enabled())
	directory, err := os.MkdirTemp("/tmp", "n9e-eval-round4-")
	require.NoError(t, err)
	t.Cleanup(func() {
		evallog.Shutdown()
		require.NoError(t, os.RemoveAll(directory))
	})
	require.NoError(t, evallog.Init(evallog.Config{
		Dir: directory, MaxSeriesPerQuery: 2, MaxPointsPerSeries: 2,
	}, evallog.Hooks{}))
}

func TestEvalLogSamplesFromValue_VectorCapsSeriesAndConvertsTimestamp(t *testing.T) {
	round4EvalLog(t)
	value := model.Vector{
		{Metric: model.Metric{"host": "a"}, Timestamp: 1700000000123, Value: 1.5},
		{Metric: model.Metric{"host": "b"}, Timestamp: 1700000002456, Value: 2.5},
		{Metric: model.Metric{"host": "c"}, Timestamp: 1700000003789, Value: 3.5},
	}

	samples, total := EvalLogSamplesFromValue(value)

	require.Equal(t, 3, total)
	require.Equal(t, []evallog.SeriesSample{
		{Labels: map[string]string{"host": "a"}, Points: [][2]float64{{1700000000, 1.5}}},
		{Labels: map[string]string{"host": "b"}, Points: [][2]float64{{1700000002, 2.5}}},
	}, samples)
	samples[0].Labels["host"] = "changed"
	require.Equal(t, model.LabelValue("a"), value[0].Metric["host"])
}

func TestEvalLogSamplesFromValue_MatrixKeepsLatestPoints(t *testing.T) {
	round4EvalLog(t)
	value := model.Matrix{{Metric: model.Metric{"host": "a"}, Values: []model.SamplePair{
		{Timestamp: 1000, Value: 1}, {Timestamp: 2000, Value: 2}, {Timestamp: 3000, Value: 3},
	}}}

	samples, total := EvalLogSamplesFromValue(value)

	require.Equal(t, 1, total)
	require.Equal(t, []evallog.SeriesSample{
		{Labels: map[string]string{"host": "a"}, Points: [][2]float64{{2, 2}, {3, 3}}},
	}, samples)
	samples[0].Points[0][1] = 99
	require.Equal(t, model.SampleValue(2), value[0].Values[1].Value)
}

func TestEvalLogSamplesFromDataResps_CapsSeriesAndSkipsMalformedPoints(t *testing.T) {
	round4EvalLog(t)
	input := []models.DataResp{
		{Metric: model.Metric{"host": "a"}, Values: [][]float64{{1, 1}, {2}, {3, 3}}},
		{Metric: model.Metric{"host": "b"}, Values: [][]float64{{4, 4}}},
		{Metric: model.Metric{"host": "c"}, Values: [][]float64{{5, 5}}},
	}

	samples, total := EvalLogSamplesFromDataResps(input)

	require.Equal(t, 3, total)
	require.Equal(t, []evallog.SeriesSample{
		{Labels: map[string]string{"host": "a"}, Points: [][2]float64{{3, 3}}},
		{Labels: map[string]string{"host": "b"}, Points: [][2]float64{{4, 4}}},
	}, samples)
	samples[0].Points[0][1] = 99
	require.Equal(t, 3.0, input[0].Values[2][1])
}

func TestEvalLogSamplesFromPoints_CapsSeriesAndPreservesValues(t *testing.T) {
	round4EvalLog(t)
	points := []models.AnomalyPoint{
		{Labels: model.Metric{"host": "a"}, Timestamp: 10, Value: 1},
		{Labels: model.Metric{"host": "b"}, Timestamp: 20, Value: 2},
		{Labels: model.Metric{"host": "c"}, Timestamp: 30, Value: 3},
	}

	samples, total := EvalLogSamplesFromPoints(points)

	require.Equal(t, 3, total)
	require.Equal(t, []evallog.SeriesSample{
		{Labels: map[string]string{"host": "a"}, Points: [][2]float64{{10, 1}}},
		{Labels: map[string]string{"host": "b"}, Points: [][2]float64{{20, 2}}},
	}, samples)
}

func TestEvalLogAnomalies_RecordsFiringAndRecoverySummaries(t *testing.T) {
	round4EvalLog(t)
	record := &evallog.EvalRecord{}
	firing := []models.AnomalyPoint{{Key: "cpu", Value: 95, Severity: 1, TriggerType: models.TriggerTypeNodata}}
	recovery := []models.AnomalyPoint{{Key: "memory", Value: 20, Severity: 2}}

	EvalLogAnomalies(record, firing, false)
	EvalLogAnomalies(record, recovery, true)

	require.Equal(t, 1, record.AnomalyTotal)
	require.Equal(t, 1, record.RecoverTotal)
	require.Equal(t, []evallog.AnomalyBrief{
		{Key: "cpu", Value: 95, Severity: 1, TriggerType: string(models.TriggerTypeNodata)},
		{Key: "memory", Value: 20, Severity: 2, Recover: true},
	}, record.Anomalies)
}

func TestEvalLogSamples_DisabledLoggingReturnsNoSamples(t *testing.T) {
	require.False(t, evallog.Enabled())

	samples, total := EvalLogSamplesFromValue(model.Vector{{Value: 1}})
	require.Empty(t, samples)
	require.Zero(t, total)
	samples, total = EvalLogSamplesFromDataResps([]models.DataResp{{Values: [][]float64{{1, 2}}}})
	require.Empty(t, samples)
	require.Zero(t, total)
	samples, total = EvalLogSamplesFromPoints([]models.AnomalyPoint{{Value: 1}})
	require.Empty(t, samples)
	require.Zero(t, total)
}

func TestTruncateForLog_PreservesUTF8AtByteBoundary(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "short", input: "query", want: "query"},
		{name: "at limit", input: strings.Repeat("a", 256), want: strings.Repeat("a", 256)},
		{name: "ASCII", input: strings.Repeat("a", 257), want: strings.Repeat("a", 256) + "...(truncated)"},
		{name: "multibyte", input: strings.Repeat("界", 86), want: strings.Repeat("界", 85) + "...(truncated)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := truncateForLog(tc.input)

			require.Equal(t, tc.want, result)
			require.True(t, utf8.ValidString(result))
		})
	}
}

func TestEvalLogQueryString_EncodesStructuredQueries(t *testing.T) {
	query := map[string]interface{}{"ref": "A", "limit": 5}

	result := EvalLogQueryString(query)

	require.JSONEq(t, `{"ref":"A","limit":5}`, result)
}

func TestEvalLogQueryString_FallsBackWhenJSONEncodingFails(t *testing.T) {
	query := complex(2, 3)

	result := EvalLogQueryString(query)

	require.Equal(t, "(2+3i)", result)
}
