package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

type instantRound3API struct {
	*fakePromAPI
	query     string
	timestamp time.Time
	value     model.Value
	err       error
}

func (f *instantRound3API) Query(_ context.Context, query string, timestamp time.Time) (model.Value, prom.Warnings, error) {
	f.query, f.timestamp = query, timestamp
	return f.value, prom.Warnings{"partial response"}, f.err
}

type queryRound3Datasource struct {
	datasource.Datasource
	dataParam interface{}
	logParam  interface{}
	data      []models.DataResp
	logs      []interface{}
	total     int64
	err       error
}

func (f *queryRound3Datasource) QueryData(_ context.Context, query interface{}) ([]models.DataResp, error) {
	f.dataParam = query
	return f.data, f.err
}

func (f *queryRound3Datasource) QueryLog(_ context.Context, query interface{}) ([]interface{}, int64, error) {
	f.logParam = query
	return f.logs, f.total, f.err
}

func TestQueryPrometheusTool_PrefersExplicitDatasourceAndReturnsVector(t *testing.T) {
	fake := &instantRound3API{fakePromAPI: &fakePromAPI{}, value: model.Vector{&model.Sample{
		Metric: model.Metric{"instance": "web-1"}, Value: 42, Timestamp: 100000,
	}}}
	deps := &aiagent.ToolDeps{GetPromClient: func(id int64) prom.API { require.Equal(t, int64(7), id); return fake }}

	out, err := queryPrometheusTool(context.Background(), deps, map[string]interface{}{
		"datasource_id": "7", "query": "up",
	}, map[string]string{"datasource_id": "3"})

	require.NoError(t, err)
	require.Equal(t, "up", fake.query)
	require.False(t, fake.timestamp.IsZero())
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, "instant", result["query_type"])
	require.Equal(t, "vector", result["result_type"])
	require.Equal(t, []interface{}{"partial response"}, result["warnings"])
	require.JSONEq(t, `[{"metric":{"instance":"web-1"},"value":[100,"42"]}]`, mustRound3JSON(t, result["data"]))
}

func mustRound3JSON(t *testing.T, value interface{}) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}

func TestQueryPrometheusRange_PassesFixedWindowAndStep(t *testing.T) {
	fake := &fakePromAPI{rangeFn: func(query string, r prom.Range) model.Matrix {
		require.Equal(t, "rate(requests[5m])", query)
		require.Equal(t, time.Unix(1000, 0), r.Start)
		require.Equal(t, time.Unix(1600, 0), r.End)
		require.Equal(t, 30*time.Second, r.Step)
		return model.Matrix{mkStream(map[string]string{"service": "api"}, r.Start, 30, []float64{2, 3})}
	}}

	out, err := queryPrometheusRange(context.Background(), fake, "rate(requests[5m])", 1000, 1600,
		map[string]interface{}{"step": float64(30)})

	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, "range", result["query_type"])
	require.Equal(t, "matrix", result["result_type"])
	require.Equal(t, float64(30), result["step"])
	require.Equal(t, formatUnixTime(1000), result["start"])
	require.Equal(t, formatUnixTime(1600), result["end"])
	require.JSONEq(t, `[{"metric":{"service":"api"},"values":[[1000,"2"],[1030,"3"]]}]`,
		mustRound3JSON(t, result["data"]))
}

func TestQueryPrometheusInstant_PropagatesDatasourceError(t *testing.T) {
	fake := &instantRound3API{fakePromAPI: &fakePromAPI{}, err: errors.New("query rejected")}

	out, err := queryPrometheusInstant(context.Background(), fake, "bad query", 1000)

	require.Empty(t, out)
	require.ErrorContains(t, err, "prometheus query failed: query rejected")
	require.Equal(t, time.Unix(1000, 0), fake.timestamp)
}

func TestQueryPrometheusTool_RejectsMissingArgumentsOrClient(t *testing.T) {
	cases := []struct {
		name string
		args map[string]interface{}
		want string
	}{
		{name: "missing datasource", args: map[string]interface{}{"query": "up"}, want: "datasource_id is required"},
		{name: "missing query", args: map[string]interface{}{"datasource_id": "7"}, want: "query parameter is required"},
		{
			name: "missing client", args: map[string]interface{}{"datasource_id": "7", "query": "up"},
			want: "prometheus datasource not found: 7",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := &aiagent.ToolDeps{GetPromClient: func(int64) prom.API { return nil }}

			out, err := queryPrometheusTool(context.Background(), deps, tc.args, nil)

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestAutoStep_ScalesQueryResolutionAtBoundaries(t *testing.T) {
	cases := []struct {
		name     string
		duration int64
		want     int
	}{
		{name: "hour", duration: 3600, want: 15},
		{name: "over hour", duration: 3601, want: 60},
		{name: "six hours", duration: 21600, want: 60},
		{name: "over six hours", duration: 21601, want: 300},
		{name: "day", duration: 86400, want: 300},
		{name: "over day", duration: 86401, want: 1800},
		{name: "week", duration: 604800, want: 1800},
		{name: "over week", duration: 604801, want: 3600},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			step := autoStep(tc.duration)

			require.Equal(t, tc.want, step)
		})
	}
}

func TestQueryTimeseriesDataTool_ResolvesTypeFromDBAndReturnsSeries(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Datasource{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Datasource{Id: 7, PluginType: "mysql"}).Error)
	fake := &queryRound3Datasource{data: []models.DataResp{{
		Ref: "A", Metric: model.Metric{"service": "api"}, Values: [][]float64{{1000, 2}, {1060, 3}},
	}}}
	deps.GetSQLDatasource = func(typ string, id int64) (datasource.Datasource, bool) {
		require.Equal(t, "mysql", typ)
		require.Equal(t, int64(7), id)
		return fake, true
	}

	out, err := queryTimeseriesDataTool(context.Background(), deps, map[string]interface{}{
		"datasource_id": "7", "sql": "select ts, value, service from metrics", "value_key": "value",
		"label_key": "service", "time_key": "ts", "database": "observability",
	}, nil)

	require.NoError(t, err)
	param := fake.dataParam.(map[string]interface{})
	require.Equal(t, "select ts, value, service from metrics", param["sql"])
	require.Equal(t, "observability", param["database"])
	require.Equal(t, int64(3600), param["to"].(int64)-param["from"].(int64))
	require.Equal(t, map[string]interface{}{"valueKey": "value", "labelKey": "service", "timeKey": "ts"}, param["keys"])
	require.JSONEq(t, `{"total":1,"items":[{"ref":"A","metric":{"service":"api"},"values":[[1000,2],[1060,3]]}]}`, out)
}

func TestQueryLogDataTool_PassesFilterAndTruncatesToRequestedLimit(t *testing.T) {
	fake := &queryRound3Datasource{logs: []interface{}{"first", "second", "third"}, total: 10}
	deps := &aiagent.ToolDeps{GetSQLDatasource: func(typ string, id int64) (datasource.Datasource, bool) {
		require.Equal(t, "elasticsearch", typ)
		require.Equal(t, int64(7), id)
		return fake, true
	}}

	out, err := queryLogDataTool(context.Background(), deps, map[string]interface{}{
		"index": "logs-*", "filter": "level:error", "limit": float64(2), "time_range": "2h",
	}, map[string]string{"datasource_id": "7", "datasource_type": "elasticsearch"})

	require.NoError(t, err)
	param := fake.logParam.(map[string]interface{})
	require.Equal(t, "logs-*", param["index"])
	require.Equal(t, "level:error", param["filter"])
	require.Equal(t, "@timestamp", param["date_field"])
	require.Equal(t, 2, param["limit"])
	require.Equal(t, int64(7200), param["end"].(int64)-param["start"].(int64))
	require.JSONEq(t, `{"total":10,"count":2,"items":["first","second"]}`, out)
}

func TestBuildQueryDataParam_EncodesDatasourceSpecificKeys(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		args map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "TDengine", typ: "tdengine",
			args: map[string]interface{}{"sql": "select value from metrics", "value_key": "value", "label_key": "host"},
			want: map[string]interface{}{
				"query": "select value from metrics", "from": "1000", "to": "1600",
				"keys": map[string]interface{}{"metricKey": "value", "labelKey": "host", "timeFormat": ""},
			},
		},
		{
			name: "OpenSearch", typ: "opensearch",
			args: map[string]interface{}{"index": "logs", "date_field": "ts", "filter": "error"},
			want: map[string]interface{}{
				"index": "logs", "date_field": "ts", "filter": "error",
				"start": int64(1000), "end": int64(1600), "interval": int64(600),
			},
		},
		{
			name: "VictoriaLogs", typ: "victorialogs", args: map[string]interface{}{"query": "error", "step": "30s"},
			want: map[string]interface{}{"query": "error", "start": int64(1000), "end": int64(1600), "step": "30s"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			param, err := buildQueryDataParam(tc.typ, tc.args, 1000, 1600)

			require.NoError(t, err)
			require.Equal(t, tc.want, param)
		})
	}
}

func TestBuildQueryLogParam_EncodesDatasourceSpecificKeys(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		args map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "SQL", typ: "pgsql", args: map[string]interface{}{"sql": "select message from logs", "database": "events"},
			want: map[string]interface{}{
				"sql": "select message from logs", "database": "events",
				"from": int64(1000), "to": int64(1600), "limit": 10,
			},
		},
		{
			name: "TDengine", typ: "tdengine", args: map[string]interface{}{"sql": "select message from logs"},
			want: map[string]interface{}{"query": "select message from logs", "from": "1000", "to": "1600"},
		},
		{
			name: "VictoriaLogs", typ: "victorialogs", args: map[string]interface{}{"query": "error"},
			want: map[string]interface{}{"query": "error", "start": int64(1000), "end": int64(1600), "limit": 10},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			param, err := buildQueryLogParam(tc.typ, tc.args, 1000, 1600, 10)

			require.NoError(t, err)
			require.Equal(t, tc.want, param)
		})
	}
}
