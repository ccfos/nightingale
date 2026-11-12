package router

import (
	"math"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestQueryBatchV2PromValue_VectorDropsNonfiniteSamplesAndSortsLabels(t *testing.T) {
	vector := model.Vector{
		&model.Sample{Metric: model.Metric{"host": "b"}, Timestamp: 12000, Value: 4},
		&model.Sample{Metric: model.Metric{"host": "a"}, Timestamp: 11000, Value: 3},
		&model.Sample{Metric: model.Metric{"host": "nan"}, Value: model.SampleValue(math.NaN())},
		&model.Sample{Metric: model.Metric{"host": "inf"}, Value: model.SampleValue(math.Inf(1))},
	}

	got := queryBatchV2PromValue(vector, 999)

	require.Equal(t, resultTypeTimeSeries, got.ResultType)
	require.Nil(t, got.Scalar)
	require.Equal(t, []QueryBatchV2Series{
		{Labels: map[string]string{"host": "a"}, Samples: []QueryBatchV2Sample{{Timestamp: 11, Value: 3}}},
		{Labels: map[string]string{"host": "b"}, Samples: []QueryBatchV2Sample{{Timestamp: 12, Value: 4}}},
	}, got.Series)
}

func TestQueryBatchV2PromValue_MatrixPreservesFiniteSampleTimes(t *testing.T) {
	matrix := model.Matrix{
		&model.SampleStream{
			Metric: model.Metric{"__name__": "cpu", "host": "a"},
			Values: []model.SamplePair{
				{Timestamp: 1000, Value: 2},
				{Timestamp: 2000, Value: model.SampleValue(math.NaN())},
				{Timestamp: 3000, Value: 5},
			},
		},
	}

	got := queryBatchV2PromValue(matrix, 999)

	require.Nil(t, got.Scalar)
	require.Equal(t, []QueryBatchV2Series{{
		Labels:  map[string]string{"__name__": "cpu", "host": "a"},
		Samples: []QueryBatchV2Sample{{Timestamp: 1, Value: 2}, {Timestamp: 3, Value: 5}},
	}}, got.Series)
}

func TestQueryBatchV2PromValue_UnsupportedAndNonfiniteValuesProduceEmptySeries(t *testing.T) {
	cases := []struct {
		name  string
		value model.Value
	}{
		{name: "nil"},
		{name: "string", value: &model.String{Value: "not numeric"}},
		{name: "nan scalar", value: &model.Scalar{Value: model.SampleValue(math.NaN())}},
		{name: "infinite scalar", value: &model.Scalar{Value: model.SampleValue(math.Inf(-1))}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := queryBatchV2PromValue(tc.value, 100)

			require.Equal(t, resultTypeTimeSeries, got.ResultType)
			require.NotNil(t, got.Series)
			require.Empty(t, got.Series)
			require.Nil(t, got.Scalar)
		})
	}
}

func TestMarshalForLog_EncodesStructuredPayloads(t *testing.T) {
	payload := map[string]interface{}{"query": "up\n> 0", "limit": 5}

	got := marshalForLog(payload)

	require.JSONEq(t, `{"query":"up\n> 0","limit":5}`, got)
	require.NotContains(t, got, "\n")
}

func TestMarshalForLog_CapsOversizedPayload(t *testing.T) {
	payload := strings.Repeat("q", 3000)

	got := marshalForLog(payload)

	require.Equal(t, strings.Repeat("q", 2048)+"...(truncated)", got)
}
