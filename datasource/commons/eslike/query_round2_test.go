package eslike

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/olivere/elastic/v7"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func round2Query() *Query {
	return &Query{
		Ref: "A", Index: "logs-*,archive-*", Start: 120, End: 250, Interval: 60,
		MetricAggr: MetricAggr{Field: "latency", Func: "avg"},
	}
}

func round2SearchResult(aggregation string) *elastic.SearchResult {
	return &elastic.SearchResult{Aggregations: elastic.Aggregations{"ts": json.RawMessage(aggregation)}}
}

func round2Source(t *testing.T, source interface{}) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(source)
	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &result))
	return result
}

func TestQueryData_BuildsTimeRangeWithDelayAndOffset(t *testing.T) {
	q := round2Query()
	q.Filter = "service:api"
	q.Offset = 5
	var body map[string]interface{}
	search := func(_ context.Context, indices []string, source interface{}, timeout, maxShard int) (*elastic.SearchResult, error) {
		require.Equal(t, []string{"logs-*", "archive-*"}, indices)
		require.Equal(t, 3, timeout)
		require.Equal(t, 5, maxShard)
		body = round2Source(t, source)
		return round2SearchResult(`{"buckets":[]}`), nil
	}
	c := context.WithValue(context.Background(), "delay", int64(10))

	items, err := QueryData(c, q, 3000, "8.1.0", search)

	require.NoError(t, err)
	require.Empty(t, items)
	require.JSONEq(t, `{
  "bool":{"must":{"query_string":{"query":"service:api"}},"filter":{"range":{"@timestamp":{
   "gte":105000,"lt":235000,"format":"epoch_millis"
  }}}}
 }`, mustRound2JSON(t, body["query"]))
}

func mustRound2JSON(t *testing.T, value interface{}) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func TestQueryData_SelectsVersionCompatibleHistogramInterval(t *testing.T) {
	for _, tc := range []struct {
		name        string
		version     string
		intervalKey string
		offset      string
	}{
		{name: "ES6", version: "6.8.0", intervalKey: "interval"},
		{name: "ES71", version: "7.1.0", intervalKey: "interval", offset: "-50s"},
		{name: "ES72", version: "7.2.0", intervalKey: "fixed_interval", offset: "-50s"},
		{name: "ES8", version: "8.0.0", intervalKey: "fixed_interval", offset: "-50s"},
		{name: "OpenSearch", version: "2.17.0", intervalKey: "interval"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := round2Query()
			q.Start = 130
			var histogram map[string]interface{}
			search := func(_ context.Context, _ []string, source interface{}, _, _ int) (*elastic.SearchResult, error) {
				body := round2Source(t, source)
				aggs := body["aggregations"].(map[string]interface{})
				histogram = aggs["ts"].(map[string]interface{})["date_histogram"].(map[string]interface{})
				return round2SearchResult(`{"buckets":[]}`), nil
			}

			_, err := QueryData(context.Background(), q, 1000, tc.version, search)

			require.NoError(t, err)
			require.Equal(t, "60s", histogram[tc.intervalKey])
			require.Equal(t, "@timestamp", histogram["field"])
			if tc.offset == "" {
				require.NotContains(t, histogram, "offset")
			} else {
				require.Equal(t, tc.offset, histogram["offset"])
			}
		})
	}
}

func TestQueryData_BuildsMetricAggregations(t *testing.T) {
	for _, tc := range []struct {
		function    string
		aggregation string
		percentile  float64
	}{
		{function: "avg", aggregation: "avg"},
		{function: "max", aggregation: "max"},
		{function: "min", aggregation: "min"},
		{function: "sum", aggregation: "sum"},
		{function: "p90", aggregation: "percentiles", percentile: 90},
		{function: "p95", aggregation: "percentiles", percentile: 95},
		{function: "p99", aggregation: "percentiles", percentile: 99},
		{function: "median", aggregation: "percentiles", percentile: 50},
	} {
		t.Run(tc.function, func(t *testing.T) {
			q := round2Query()
			q.MetricAggr.Func = tc.function
			var metric map[string]interface{}
			search := func(_ context.Context, _ []string, source interface{}, _, _ int) (*elastic.SearchResult, error) {
				body := round2Source(t, source)
				ts := body["aggregations"].(map[string]interface{})["ts"].(map[string]interface{})
				metric = ts["aggregations"].(map[string]interface{})["latency"].(map[string]interface{})
				return round2SearchResult(`{"buckets":[]}`), nil
			}

			_, err := QueryData(context.Background(), q, 1000, "7.10+", search)

			require.NoError(t, err)
			agg := metric[tc.aggregation].(map[string]interface{})
			require.Equal(t, "latency", agg["field"])
			if tc.percentile != 0 {
				require.Equal(t, []interface{}{tc.percentile}, agg["percents"])
			}
		})
	}
}

func TestQueryData_ParsesGroupedSeriesAndDropsIncompleteBucket(t *testing.T) {
	q := round2Query()
	q.GroupBy = []GroupBy{{Cate: Terms, Field: "host"}}
	search := func(_ context.Context, _ []string, source interface{}, _, _ int) (*elastic.SearchResult, error) {
		body := round2Source(t, source)
		ts := body["aggregations"].(map[string]interface{})["ts"].(map[string]interface{})
		host := ts["aggregations"].(map[string]interface{})["host"].(map[string]interface{})
		require.JSONEq(t, `{"field":"host","size":300,"min_doc_count":1,"order":[{"_key":"desc"}]}`,
			mustRound2JSON(t, host["terms"]))
		return round2SearchResult(`{"buckets":[
   {"key":120000,"key_as_string":"120000","host":{"buckets":[{"key":"a--b=c","latency":{"value":2.5}}]}},
   {"key":180000,"key_as_string":"180000","host":{"buckets":[{"key":"a--b=c","latency":{"value":3.5}}]}},
   {"key":240000,"key_as_string":"240000","host":{"buckets":[{"key":"a--b=c","latency":{"value":1}}]}}
  ]}`), nil
	}

	items, err := QueryData(context.Background(), q, 1000, "8.0.0", search)

	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "A", items[0].Ref)
	require.Equal(t, model.Metric{"__name__": "A_latency_avg", "host": "a--b=c"}, items[0].Metric)
	// The bucket at 240 is incomplete because the query ends at 250.
	require.Equal(t, [][]float64{{120, 2.5}, {180, 3.5}}, items[0].Values)
	require.Contains(t, items[0].Query, "logs-*,archive-*")
}

func TestQueryData_CountUsesDocumentCounts(t *testing.T) {
	q := round2Query()
	q.MetricAggr.Func = "count"
	search := func(_ context.Context, _ []string, source interface{}, _, _ int) (*elastic.SearchResult, error) {
		body := round2Source(t, source)
		ts := body["aggregations"].(map[string]interface{})["ts"].(map[string]interface{})
		require.NotContains(t, ts, "aggregations")
		return round2SearchResult(`{"buckets":[{"key":120000,"key_as_string":"1970-01-01T00:02:00Z","doc_count":9}]}`), nil
	}

	items, err := QueryData(context.Background(), q, 1000, "8.0.0", search)

	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, [][]float64{{120, 9}}, items[0].Values)
}

func TestQueryData_ResolvesIndexPattern(t *testing.T) {
	old := GetEsIndexPatternCacheType()
	t.Cleanup(func() { SetEsIndexPatternCacheType(old) })
	cache := &memsto.EsIndexPatternCacheType{}
	cache.Set(map[int64]*models.EsIndexPattern{7: {Id: 7, Name: "app-*", TimeField: "created"}})
	SetEsIndexPatternCacheType(cache)
	q := round2Query()
	q.IndexType, q.IndexPatternId = "index_pattern", 7
	search := func(_ context.Context, indices []string, source interface{}, _, _ int) (*elastic.SearchResult, error) {
		require.Equal(t, []string{"app-*"}, indices)
		body := round2Source(t, source)
		ts := body["aggregations"].(map[string]interface{})["ts"].(map[string]interface{})
		require.Equal(t, "created", ts["date_histogram"].(map[string]interface{})["field"])
		return round2SearchResult(`{"buckets":[]}`), nil
	}

	_, err := QueryData(context.Background(), q, 1000, "8.0.0", search)

	require.NoError(t, err)
}

func TestQueryData_PropagatesSearchError(t *testing.T) {
	failure := errors.New("search unavailable")
	search := func(context.Context, []string, interface{}, int, int) (*elastic.SearchResult, error) {
		return nil, failure
	}

	items, err := QueryData(context.Background(), round2Query(), 1000, "8.0.0", search)

	require.ErrorIs(t, err, failure)
	require.Nil(t, items)
}

func TestQueryData_RejectsUnsupportedMetricFunction(t *testing.T) {
	q := round2Query()
	q.MetricAggr.Func = "unsupported"

	items, err := QueryData(context.Background(), q, 1000, "8.0.0", nil)

	require.ErrorContains(t, err, "func unsupported not support")
	require.Nil(t, items)
}

func TestQueryData_RejectsMalformedAggregationResponse(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response string
	}{
		{name: "invalid JSON", response: "{"},
		{name: "missing buckets", response: `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			search := func(context.Context, []string, interface{}, int, int) (*elastic.SearchResult, error) {
				return round2SearchResult(tc.response), nil
			}

			items, err := QueryData(context.Background(), round2Query(), 1000, "8.0.0", search)

			require.Error(t, err)
			require.Nil(t, items)
		})
	}
}

func TestTransferData_PreservesLabelSeparatorsAndEqualsInValues(t *testing.T) {
	points := [][]float64{{100, 2}}
	labels := "host=a--b" + LabelSeparator + "path=a=b"

	got := TransferData("requests_count", "B", map[string][][]float64{labels: points})

	require.Len(t, got, 1)
	require.Equal(t, model.Metric{"__name__": "B_requests_count", "host": "a--b", "path": "a=b"}, got[0].Metric)
	require.Equal(t, points, got[0].Values)
}

func TestMakeQuery_AppendsQuotedEventTagsAndOverridesRange(t *testing.T) {
	for _, tc := range []struct {
		name      string
		makeQuery func(context.Context, interface{}, []string, int64, int64) (interface{}, error)
	}{
		{name: "logs", makeQuery: MakeLogQuery},
		{name: "series", makeQuery: MakeTSQuery},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]interface{}{"filter": "status:error", "start": 1, "end": 2}

			value, err := tc.makeQuery(context.Background(), input, []string{`host=a b`, `path=x=y`}, 100, 200)

			require.NoError(t, err)
			got := value.(*Query)
			require.Equal(t, `status:error AND host:"a b" AND path:"x=y"`, got.Filter)
			require.Equal(t, int64(100), got.Start)
			require.Equal(t, int64(200), got.End)
			require.Equal(t, "status:error", input["filter"])
		})
	}
}
