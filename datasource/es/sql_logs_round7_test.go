package es

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource/commons/eslike"
	"github.com/stretchr/testify/require"
)

func TestQueryLog_SQLBuildsRequestAndProjectsRowsByColumnName(t *testing.T) {
	requests := make(chan map[string]interface{}, 1)
	client := newHTTPTestElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_sql" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
   "columns":[{"name":"host","type":"keyword"},{"name":"count","type":"long"}],
   "rows":[["api",3],["worker"]]
  }`))
	})

	logs, total, err := client.QueryLog(context.Background(), map[string]interface{}{
		"sql": `SELECT host, count FROM "logs-*"`, "start": 10, "end": 20,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Equal(t, []interface{}{
		map[string]interface{}{"host": "api", "count": float64(3)},
		map[string]interface{}{"host": "worker"},
	}, logs)
	body := <-requests
	require.Equal(t, `SELECT host, count FROM "logs-*"`, body["query"])
	require.Equal(t, true, body["field_multi_value_leniency"])
}

func TestQueryLog_SQLReturnsEmptySliceForNoRows(t *testing.T) {
	client := newHTTPTestElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"columns":[{"name":"message","type":"text"}],"rows":[]}`))
	})

	logs, total, err := client.QueryLog(context.Background(), map[string]interface{}{"sql": "SELECT message FROM logs"})

	require.NoError(t, err)
	require.NotNil(t, logs)
	require.Empty(t, logs)
	require.Zero(t, total)
}

func TestQueryLog_SQLWrapsExecutionError(t *testing.T) {
	client := newHTTPTestElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"type":"parsing_exception","reason":"unknown column"},"status":400}`))
	})

	logs, total, err := client.QueryLog(context.Background(), map[string]interface{}{"sql": "SELECT missing FROM logs"})

	require.ErrorContains(t, err, "ES SQL query failed")
	require.Nil(t, logs)
	require.Zero(t, total)
}

func TestMakeLogQuery_PropagatesDecodeFailure(t *testing.T) {
	query, err := (&Elasticsearch{}).MakeLogQuery(
		context.Background(), map[string]interface{}{"start": []string{"bad"}}, nil, 1, 2,
	)

	require.Error(t, err)
	require.Nil(t, query)
}

func TestMakeTSQuery_OverridesWindowAndKeepsMetricDefinition(t *testing.T) {
	input := eslike.Query{Index: "logs-*", Start: 1, End: 2, MetricAggr: eslike.MetricAggr{Field: "latency", Func: "avg"}}

	query, err := (&Elasticsearch{}).MakeTSQuery(context.Background(), input, []string{"host=api"}, 10, 20)

	require.NoError(t, err)
	got := query.(*eslike.Query)
	require.Equal(t, int64(10), got.Start)
	require.Equal(t, int64(20), got.End)
	require.Equal(t, input.MetricAggr, got.MetricAggr)
	require.Contains(t, got.Filter, `host:"api"`)
}
