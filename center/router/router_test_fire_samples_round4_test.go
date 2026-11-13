package router

import (
	"math"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestPromValueToDataResps_PreservesLabelsAndDropsNonFiniteSamples(t *testing.T) {
	metric := model.Metric{model.MetricNameLabel: "cpu", "host": "host-a"}
	value := model.Vector{
		{Metric: metric, Timestamp: model.TimeFromUnix(123), Value: 2.5},
		{Metric: model.Metric{"host": "nan"}, Value: model.SampleValue(math.NaN())},
		{Metric: model.Metric{"host": "positive-infinity"}, Value: model.SampleValue(math.Inf(1))},
		{Metric: model.Metric{"host": "negative-infinity"}, Value: model.SampleValue(math.Inf(-1))},
		{Metric: model.Metric{"host": "zero"}, Timestamp: model.TimeFromUnix(124), Value: 0},
	}

	got := promValueToDataResps(value, "A", "cpu")

	require.Equal(t, []models.DataResp{
		{Ref: "A", Query: "cpu", Metric: metric, Values: [][]float64{{123, 2.5}}},
		{Ref: "A", Query: "cpu", Metric: model.Metric{"host": "zero"}, Values: [][]float64{{124, 0}}},
	}, got)
}

func TestPromValueToDataResps_IgnoresNonVectorResults(t *testing.T) {
	cases := []struct {
		name  string
		value model.Value
	}{
		{name: "nil"},
		{name: "scalar", value: &model.Scalar{Value: 3}},
		{name: "matrix", value: model.Matrix{{Metric: model.Metric{"host": "a"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := promValueToDataResps(tc.value, "A", "cpu")

			require.NotNil(t, got)
			require.Empty(t, got)
		})
	}
}

func TestPromSeriesCountAndFirst_SelectsFirstVectorSample(t *testing.T) {
	value := model.Vector{
		{Metric: model.Metric{model.MetricNameLabel: "cpu", "host": "host-a"}, Value: 2.5},
		{Metric: model.Metric{"host": "host-b"}, Value: 3.5},
	}

	count, sample := promSeriesCountAndFirst(value)

	require.Equal(t, 2, count)
	require.NotNil(t, sample)
	require.Equal(t, map[string]string{"__name__": "cpu", "host": "host-a"}, sample.labels)
	require.Equal(t, 2.5, sample.value)
}

func TestPromSeriesCountAndFirst_HandlesScalarMatrixAndEmptyResults(t *testing.T) {
	cases := []struct {
		name   string
		value  model.Value
		count  int
		sample *testFireAutoSample
	}{
		{name: "nil"},
		{name: "empty vector", value: model.Vector{}},
		{name: "typed nil scalar", value: (*model.Scalar)(nil)},
		{
			name: "scalar", value: &model.Scalar{Value: 7.5}, count: 1,
			sample: &testFireAutoSample{labels: map[string]string{}, value: 7.5},
		},
		{
			name: "matrix", count: 2,
			value: model.Matrix{{Metric: model.Metric{"host": "a"}}, {Metric: model.Metric{"host": "b"}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			count, sample := promSeriesCountAndFirst(tc.value)

			require.Equal(t, tc.count, count)
			require.Equal(t, tc.sample, sample)
		})
	}
}
