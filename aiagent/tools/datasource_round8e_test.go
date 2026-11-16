package tools

import (
	"context"
	"errors"
	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type r8eRangeAPI struct {
	prom.API
	value model.Value
	err   error
}

func (f *r8eRangeAPI) QueryRange(context.Context, string, prom.Range) (model.Value, prom.Warnings, error) {
	return f.value, prom.Warnings{"partial"}, f.err
}

func TestR8eDatasourceBuilders(t *testing.T) {
	for _, kind := range []string{"mysql", "tdengine", "elasticsearch", "opensearch", "victorialogs", "unknown"} {
		for _, build := range []func(string, map[string]interface{}, int64, int64) (map[string]interface{}, error){buildQueryDataParam, func(k string, a map[string]interface{}, s, e int64) (map[string]interface{}, error) {
			return buildQueryLogParam(k, a, s, e, 15)
		}} {
			_, err := build(kind, nil, 100, 200)
			require.Error(t, err, kind)
		}
	}
	p, err := buildQueryDataParam("victorialogs", map[string]interface{}{"query": "*", "step": "30s"}, 100, 200)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"query": "*", "start": int64(100), "end": int64(200), "step": "30s"}, p)
	p, err = buildQueryLogParam("tdengine", map[string]interface{}{"sql": "select * from logs", "database": "metrics"}, 100, 200, 15)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"query": "select * from logs", "from": "100", "to": "200", "database": "metrics"}, p)
	for _, kind := range []string{"mysql", "tdengine"} {
		_, err := buildQueryDataParam(kind, map[string]interface{}{"sql": "delete from logs"}, 1, 2)
		require.ErrorContains(t, err, "forbidden")
		_, err = buildQueryLogParam(kind, map[string]interface{}{"sql": "drop table logs"}, 1, 2, 10)
		require.ErrorContains(t, err, "forbidden")
		_, err = buildQueryDataParam(kind, map[string]interface{}{"sql": "select 1"}, 1, 2)
		require.ErrorContains(t, err, "value_key")
	}
}

func TestR8eDatasourceHandlersFallbackAndFailures(t *testing.T) {
	fake := &queryRound3Datasource{err: errors.New("backend unavailable")}
	deps := &aiagent.ToolDeps{GetSQLDatasource: func(string, int64) (datasource.Datasource, bool) { return fake, true }}
	for _, fn := range []aiagent.BuiltinToolFunc{queryTimeseriesDataTool, queryLogDataTool} {
		out, err := fn(context.Background(), deps, map[string]interface{}{"datasource_id": "7", "datasource_type": "mysql", "sql": "select 1", "value_key": "v", "time_range": "garbage"}, nil)
		require.Empty(t, out)
		require.ErrorContains(t, err, "backend unavailable")
		out, err = fn(context.Background(), deps, map[string]interface{}{"datasource_id": "7", "datasource_type": "mysql"}, nil)
		require.Empty(t, out)
		require.ErrorContains(t, err, "sql parameter")
	}
	fake.logs = []interface{}{1, 2, 3}
	fake.total = 9
	fake.err = nil
	out, err := queryLogDataTool(context.Background(), deps, map[string]interface{}{"datasource_id": "7", "datasource_type": "mysql", "sql": "select 1", "limit": float64(2)}, nil)
	require.NoError(t, err)
	require.JSONEq(t, `{"total":9,"count":2,"items":[1,2]}`, out)
	for _, tc := range []struct {
		value model.Value
		err   error
		want  string
	}{{model.Vector{}, nil, "unexpected"}, {nil, errors.New("bad range"), "bad range"}} {
		_, err := promRangeMatrix(context.Background(), &r8eRangeAPI{value: tc.value, err: tc.err}, "up", 1, 2, 1)
		require.ErrorContains(t, err, tc.want)
	}
	api := &r8eRangeAPI{value: model.Matrix{}}
	out, err = queryPrometheusRange(context.Background(), api, "up", 100, 200, nil)
	require.NoError(t, err)
	require.Contains(t, out, `"warnings":["partial"]`)
	api.err = errors.New("bad range")
	_, err = queryPrometheusRange(context.Background(), api, "up", 100, 200, nil)
	require.ErrorContains(t, err, "prometheus range query failed")
	instant := &instantRound3API{fakePromAPI: &fakePromAPI{}, value: model.Vector{}}
	deps.GetPromClient = func(int64) prom.API { return instant }
	before := time.Now().Unix()
	out, err = queryPrometheusTool(context.Background(), deps, map[string]interface{}{"query": "up", "time_range": "garbage"}, map[string]string{"datasource_id": "7"})
	require.NoError(t, err)
	require.Contains(t, out, `"query_type":"instant"`)
	require.GreaterOrEqual(t, instant.timestamp.Unix(), before)
	require.LessOrEqual(t, instant.timestamp.Unix(), time.Now().Unix())
}
