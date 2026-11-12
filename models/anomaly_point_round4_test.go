package models

import (
	"math"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/unit"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestNewAnomalyPoint_CopiesLabelsAndSetsMetricName(t *testing.T) {
	labels := map[string]string{"host": "node-a", "__name__": "old"}

	point := NewAnomalyPoint("cpu", labels, 100, 1.5, 2)
	labels["host"] = "changed"

	require.Equal(t, "cpu", point.Key)
	require.Equal(t, model.Metric{"__name__": "cpu", "host": "node-a"}, point.Labels)
	require.Equal(t, int64(100), point.Timestamp)
	require.Equal(t, 1.5, point.Value)
	require.Equal(t, 2, point.Severity)
}

func TestAnomalyPointReadableValue_FormatsNumbersWithoutTrailingZeros(t *testing.T) {
	cases := []struct {
		name  string
		value float64
		want  string
	}{
		{name: "zero", value: 0, want: "0"},
		{name: "integer", value: 10, want: "10"},
		{name: "fraction", value: 1.25, want: "1.25"},
		{name: "precision", value: 1.234567, want: "1.23457"},
		{name: "negative", value: -2.5, want: "-2.5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			point := AnomalyPoint{Value: tc.value}

			got := point.ReadableValue()

			require.Equal(t, tc.want, got)
		})
	}
}

func TestAnomalyPointReadableValue_PrefersConfiguredUnit(t *testing.T) {
	point := AnomalyPoint{Value: 1024, ValuesUnit: map[string]unit.FormattedValue{"A": {Text: "1 KiB"}}}

	got := point.ReadableValue()

	require.Equal(t, "1 KiB", got)
}

func TestConvertAnomalyPoints_VectorPreservesLabelsAndSkipsNaN(t *testing.T) {
	metric := model.Metric{"__name__": "cpu", "host": "node-a"}
	value := model.Vector{
		&model.Sample{Metric: metric, Timestamp: model.Time(123000), Value: 2.5},
		&model.Sample{Metric: model.Metric{"host": "missing"}, Value: model.SampleValue(math.NaN())},
	}

	points := ConvertAnomalyPoints(value)

	require.Equal(t, []AnomalyPoint{{Key: metric.String(), Labels: metric, Timestamp: 123, Value: 2.5}}, points)
}

func TestConvertAnomalyPoints_MatrixUsesLastSampleAndSkipsNaN(t *testing.T) {
	metric := model.Metric{"host": "node-a"}
	value := model.Matrix{
		&model.SampleStream{Metric: metric, Values: []model.SamplePair{
			{Timestamp: model.Time(1000), Value: 1},
			{Timestamp: model.Time(2000), Value: 3},
		}},
		&model.SampleStream{Metric: model.Metric{"host": "missing"}, Values: []model.SamplePair{
			{Timestamp: model.Time(3000), Value: model.SampleValue(math.NaN())},
		}},
	}

	points := ConvertAnomalyPoints(value)

	require.Equal(t, []AnomalyPoint{{Key: metric.String(), Labels: metric, Timestamp: 2, Value: 3}}, points)
}

func TestConvertAnomalyPoints_ScalarUsesEmptyLabels(t *testing.T) {
	value := &model.Scalar{Timestamp: model.Time(42000), Value: 7.5}

	points := ConvertAnomalyPoints(value)

	require.Equal(t, []AnomalyPoint{{Key: "{}", Labels: model.Metric{}, Timestamp: 42, Value: 7.5}}, points)
}

func TestConvertAnomalyPoints_RejectsAbsentUnsupportedAndNaNScalarValues(t *testing.T) {
	cases := []struct {
		name  string
		value model.Value
	}{
		{name: "nil"},
		{name: "string", value: &model.String{Value: "text"}},
		{name: "NaN", value: &model.Scalar{Value: model.SampleValue(math.NaN())}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			points := ConvertAnomalyPoints(tc.value)

			require.Empty(t, points)
		})
	}
}
