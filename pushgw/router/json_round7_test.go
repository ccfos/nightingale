package router

import (
	"encoding/json"
	"testing"

	"github.com/mailru/easyjson"
	"github.com/mailru/easyjson/jwriter"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

func TestHTTPMetricArr_JSONPreservesBatchOrder(t *testing.T) {
	input := HTTPMetricArr{
		{Metric: "cpu", Timestamp: 1000, ValueUnTyped: float64(1), Tags: map[string]string{"host": "a"}},
		{Metric: "mem", Timestamp: 2000, ValueUnTyped: float64(2)},
	}

	encoded, err := json.Marshal(input)

	require.NoError(t, err)
	require.JSONEq(t, `[
		{"metric":"cpu","timestamp":1000,"value":1,"tags":{"host":"a"}},
		{"metric":"mem","timestamp":2000,"value":2,"tags":null}
	]`, string(encoded))
	var decoded HTTPMetricArr
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, input, decoded)
	fast, err := easyjson.Marshal(input)
	require.NoError(t, err)
	require.JSONEq(t, string(encoded), string(fast))
}

func TestFalconMetricArr_JSONPreservesBatchOrder(t *testing.T) {
	input := FalconMetricArr{
		{Metric: "cpu", Endpoint: "a", Timestamp: 1000, ValueUnTyped: float64(1), Tags: "env=prod"},
		{Metric: "mem", Endpoint: "b", Timestamp: 2000, ValueUnTyped: float64(2)},
	}

	encoded, err := json.Marshal(input)

	require.NoError(t, err)
	require.JSONEq(t, `[
		{"metric":"cpu","endpoint":"a","timestamp":1000,"value":1,"tags":"env=prod"},
		{"metric":"mem","endpoint":"b","timestamp":2000,"value":2,"tags":""}
	]`, string(encoded))
	var decoded FalconMetricArr
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, input, decoded)
	fast, err := easyjson.Marshal(input)
	require.NoError(t, err)
	require.JSONEq(t, string(encoded), string(fast))
}

func TestMetricBatchJSON_DistinguishesNullFromEmptyArray(t *testing.T) {
	tests := []struct {
		name  string
		batch easyjson.Marshaler
		want  string
	}{
		{name: "nil OpenTSDB", batch: HTTPMetricArr(nil), want: "null"},
		{name: "empty OpenTSDB", batch: HTTPMetricArr{}, want: "[]"},
		{name: "nil Falcon", batch: FalconMetricArr(nil), want: "null"},
		{name: "empty Falcon", batch: FalconMetricArr{}, want: "[]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := easyjson.Marshal(tc.batch)

			require.NoError(t, err)
			require.Equal(t, tc.want, string(encoded))
			// The explicit writer flag normalizes nil batches for callers requiring an array.
			w := jwriter.Writer{Flags: jwriter.NilSliceAsEmpty}
			tc.batch.MarshalEasyJSON(&w)
			array, err := w.BuildBytes()
			require.NoError(t, err)
			require.Equal(t, "[]", string(array))
		})
	}
}

func TestHTTPMetricArr_UnmarshalReplacesPreviousBatch(t *testing.T) {
	batch := HTTPMetricArr{{Metric: "stale-a"}, {Metric: "stale-b"}}

	err := json.Unmarshal([]byte(`[{"metric":"cpu","tags":{},"future":{"nested":[1,2]}}]`), &batch)

	require.NoError(t, err)
	require.Equal(t, HTTPMetricArr{{Metric: "cpu", Tags: map[string]string{}}}, batch)
	require.NoError(t, json.Unmarshal([]byte("[]"), &batch))
	require.NotNil(t, batch)
	require.Empty(t, batch)
	require.NoError(t, json.Unmarshal([]byte("null"), &batch))
	require.Nil(t, batch)
}

func TestFalconMetricArr_UnmarshalReplacesPreviousBatch(t *testing.T) {
	batch := FalconMetricArr{{Metric: "stale-a"}, {Metric: "stale-b"}}

	err := json.Unmarshal([]byte(`[{"metric":"cpu","future":{"nested":[1,2]}}]`), &batch)

	require.NoError(t, err)
	require.Equal(t, FalconMetricArr{{Metric: "cpu"}}, batch)
	require.NoError(t, json.Unmarshal([]byte("[]"), &batch))
	require.NotNil(t, batch)
	require.Empty(t, batch)
	require.NoError(t, json.Unmarshal([]byte("null"), &batch))
	require.Nil(t, batch)
}

func TestDatadogMetric_UnmarshalReplacesPointsAndTags(t *testing.T) {
	metric := DatadogMetric{Points: []DatadogPoint{{1, 2}, {3, 4}}, Tags: []string{"old", "tags"}}

	err := json.Unmarshal([]byte(`{
		"metric":"cpu","points":[[1000,7]],"tags":["env:prod"],"future":{"nested":[1,2]}
	}`), &metric)

	require.NoError(t, err)
	require.Equal(t, DatadogMetric{Metric: "cpu", Points: []DatadogPoint{{1000, 7}}, Tags: []string{"env:prod"}}, metric)
	require.NoError(t, json.Unmarshal([]byte(`{"points":[],"tags":[]}`), &metric))
	require.NotNil(t, metric.Points)
	require.NotNil(t, metric.Tags)
	require.Empty(t, metric.Points)
	require.Empty(t, metric.Tags)
}

func TestDatadogMetric_JSONRepresentsMissingPointsAsNull(t *testing.T) {
	encoded, err := json.Marshal(DatadogMetric{Metric: "cpu"})

	require.NoError(t, err)
	require.JSONEq(t, `{"metric":"cpu","points":null,"host":""}`, string(encoded))
	var metric DatadogMetric
	require.NoError(t, json.Unmarshal([]byte(`{"metric":"cpu","points":[],"tags":[]}`), &metric))
	require.Equal(t, []DatadogPoint{}, metric.Points)
	require.Equal(t, []string{}, metric.Tags)
}

func TestDatadogTimeSeries_UnmarshalReplacesPreviousBatch(t *testing.T) {
	batch := TimeSeries{Series: []*DatadogMetric{{Metric: "stale-a"}, {Metric: "stale-b"}}}

	err := json.Unmarshal([]byte(`{"series":[null,{"metric":"cpu","points":[[1000,7]]}],"extra":{"x":1}}`), &batch)

	require.NoError(t, err)
	require.Equal(t, TimeSeries{Series: []*DatadogMetric{nil, {Metric: "cpu", Points: []DatadogPoint{{1000, 7}}}}}, batch)
	encoded, err := easyjson.Marshal(batch)
	require.NoError(t, err)
	require.JSONEq(t, `{"series":[null,{"metric":"cpu","host":"","points":[[1000,7]]}]}`, string(encoded))
	require.NoError(t, json.Unmarshal([]byte(`{"series":[]}`), &batch))
	require.NotNil(t, batch.Series)
	require.Empty(t, batch.Series)
}

func TestDatadogTimeSeries_JSONRepresentsNilSeriesAsNull(t *testing.T) {
	encoded, err := easyjson.Marshal(TimeSeries{})

	require.NoError(t, err)
	require.JSONEq(t, `{"series":null}`, string(encoded))
	var batch TimeSeries
	require.NoError(t, json.Unmarshal([]byte(`{"series":[]}`), &batch))
	require.Equal(t, []*DatadogMetric{}, batch.Series)
}

func TestDatadogPoint_JSONPreservesTimestampAndValue(t *testing.T) {
	var point DatadogPoint
	require.NoError(t, json.Unmarshal([]byte(`[1000,7]`), &point))

	encoded, err := json.Marshal(point)

	require.NoError(t, err)
	require.Equal(t, DatadogPoint{1000, 7}, point)
	require.JSONEq(t, `[1000,7]`, string(encoded))
}

func TestMetricJSON_RejectsWrongFieldTypes(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		output  interface{}
	}{
		{name: "OpenTSDB timestamp", payload: `{"timestamp":"yesterday"}`, output: &HTTPMetric{}},
		{name: "Falcon timestamp", payload: `{"timestamp":{}}`, output: &FalconMetric{}},
		{name: "Datadog points", payload: `{"points":"bad"}`, output: &DatadogMetric{}},
		{name: "Datadog point", payload: `["yesterday",7]`, output: &DatadogPoint{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := json.Unmarshal([]byte(tc.payload), tc.output)

			require.Error(t, err)
		})
	}
}

func TestExtractMetricFromTimeSeries_FindsNameAmongOtherLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels []prompb.Label
		want   string
	}{
		{name: "named", labels: []prompb.Label{{Name: "job", Value: "api"}, {Name: "__name__", Value: "cpu"}}, want: "cpu"},
		{name: "missing name", labels: []prompb.Label{{Name: "job", Value: "api"}}},
		{name: "empty"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractMetricFromTimeSeries(&prompb.TimeSeries{Labels: tc.labels})

			require.Equal(t, tc.want, got)
		})
	}
}
