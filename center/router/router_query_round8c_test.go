package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	pkgprom "github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/prometheus/common/model"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/stretchr/testify/require"
)

func TestR8cPrometheusQueryFailures(t *testing.T) {
	for _, tc := range []struct {
		raw, msg, code string
		emptyClients   bool
	}{
		{`{"expr":42}`, "decode prometheus query", "INVALID_QUERY", false},
		{`{}`, "expr is required", "INVALID_QUERY", false},
		{`{"expr":"up"}`, "step must be greater", "INVALID_QUERY", false},
		{`{"expr":"up","instant":true}`, "not found", "DATASOURCE_NOT_FOUND", false},
		{`{"expr":"up","instant":true}`, "not found", "DATASOURCE_NOT_FOUND", true},
	} {
		t.Run(tc.msg+fmt.Sprint(tc.emptyClients), func(t *testing.T) {
			e := queryBatchV2Executor{}
			if tc.emptyClients {
				e.promClients = &prom.PromClientMap{}
			}
			_, err := e.queryPrometheus(context.Background(), QueryBatchV2Request{}, QueryBatchV2Query{Datasource: &QueryBatchV2DatasourceRef{ID: 1}, Query: json.RawMessage(tc.raw)})
			require.ErrorContains(t, err, tc.msg)
			require.Equal(t, tc.code, queryBatchV2DatasourceErrorCode(err))
			require.NotNil(t, errors.Unwrap(err))
		})
	}
}
func TestR8cBatchDatasourceFailuresAndPanicIsolation(t *testing.T) {
	for _, mode := range []string{"logs error", "series error", "panic", "missing", "prom logs", "prom missing"} {
		t.Run(mode, func(t *testing.T) {
			fake := &queryRound6Datasource{data: func(context.Context, interface{}) ([]models.DataResp, error) {
				if mode == "panic" {
					panic("broken adapter")
				}
				return nil, errors.New("backend rejected")
			}, logs: func(context.Context, interface{}) ([]interface{}, int64, error) {
				return nil, 0, errors.New("backend rejected")
			}}
			e := queryBatchV2Executor{anonymousAccess: true, getDatasource: func(string, int64) (datasource.Datasource, bool) { return fake, mode != "missing" }}
			q := QueryBatchV2Query{RefID: "A", Kind: queryKindDatasource, Datasource: &QueryBatchV2DatasourceRef{Cate: "fake", ID: 1}, ResultType: resultTypeTimeSeries, Query: json.RawMessage(`{"expr":"up","instant":true}`)}
			if mode == "logs error" || mode == "prom logs" {
				q.ResultType = resultTypeLogs
			}
			if mode == "prom logs" || mode == "prom missing" {
				q.Datasource.Cate = "prometheus"
			}
			result := e.execute(queryRound6Context(), QueryBatchV2Request{Queries: []QueryBatchV2Query{q}}).Results[0]
			require.Equal(t, resultStatusError, result.Status)
			require.NotNil(t, result.Error)
			switch mode {
			case "panic":
				require.Contains(t, result.Error.Message, "query panic: broken adapter")
			case "missing", "prom missing":
				require.Equal(t, "DATASOURCE_NOT_FOUND", result.Error.Code)
			case "prom logs":
				require.Equal(t, "INVALID_QUERY", result.Error.Code)
			default:
				require.Contains(t, result.Error.Message, "backend rejected")
			}
		})
	}
}
func TestR8cQueryBatchHTTPValidation(t *testing.T) {
	rt := &Router{}
	q := QueryBatchV2Query{Kind: queryKindDatasource, RefID: "A", Datasource: &QueryBatchV2DatasourceRef{Cate: "x", ID: 1}, ResultType: resultTypeLogs, Query: json.RawMessage(`{}`)}
	for _, raw := range []json.RawMessage{json.RawMessage(`[]`), json.RawMessage(`"text"`)} {
		q.Query = raw
		w := r8cRequest(t, rt.queryBatchV2, QueryBatchV2Request{Queries: []QueryBatchV2Query{q}}, "/1")
		require.Equal(t, 400, w.Code)
		require.Contains(t, w.Body.String(), "JSON object")
	}
}

func TestR8cPrometheusExecutesInstantAndRangeQueries(t *testing.T) {
	for _, instant := range []bool{true, false} {
		t.Run(fmt.Sprint(instant), func(t *testing.T) {
			client := &batchRound6PromAPI{query: func(_ context.Context, expr string, at time.Time) (model.Value, pkgprom.Warnings, error) {
				require.Equal(t, "up", expr)
				require.Equal(t, int64(20), at.Unix())
				return &model.Scalar{Value: 3}, nil, nil
			}, queryRange: func(_ context.Context, expr string, r pkgprom.Range) (model.Value, pkgprom.Warnings, error) {
				require.Equal(t, "up", expr)
				require.Equal(t, int64(10), r.Start.Unix())
				require.Equal(t, int64(20), r.End.Unix())
				require.Equal(t, time.Second, r.Step)
				return model.Matrix{&model.SampleStream{Values: []model.SamplePair{{Timestamp: 10000, Value: 4}}}}, nil, nil
			}}
			e := queryBatchV2Executor{promClients: &prom.PromClientMap{ReaderClients: map[int64]pkgprom.API{1: client}}}
			raw, err := json.Marshal(queryBatchV2PromQuery{Expr: "up", Instant: instant, Step: 1})
			require.NoError(t, err)
			value, err := e.queryPrometheus(context.Background(), QueryBatchV2Request{From: 10, To: 20}, QueryBatchV2Query{Datasource: &QueryBatchV2DatasourceRef{ID: 1}, Query: raw})
			require.NoError(t, err)
			require.Len(t, value.Series, 1)
			if instant {
				require.Equal(t, float64(3), *value.Scalar)
			} else {
				require.Equal(t, float64(4), value.Series[0].Samples[0].Value)
			}
		})
	}
}
func TestR8cExpressionRejectsAmbiguousMetricsAndTooManyGroups(t *testing.T) {
	values := map[string]queryBatchV2Value{"A": {ResultType: resultTypeTimeSeries, Series: []QueryBatchV2Series{{Labels: map[string]string{"host": "a"}, Samples: []QueryBatchV2Sample{{Timestamp: 1, Value: 1}}}, {Labels: map[string]string{"host": "a"}, Samples: []QueryBatchV2Sample{{Timestamp: 1, Value: 9}}}}}}
	got, err := queryBatchV2EvaluateMath(context.Background(), "$A + 1", []string{"A"}, values, 1)
	require.NoError(t, err)
	require.Equal(t, float64(2), got.Series[0].Samples[0].Value)
	series := make([]QueryBatchV2Series, queryBatchV2MaxExpressionGroups+1)
	for i := range series {
		series[i].Labels = map[string]string{"host": fmt.Sprint(i)}
	}
	values["A"] = queryBatchV2Value{Series: series}
	_, err = queryBatchV2EvaluateMath(context.Background(), "$A + 1", []string{"A"}, values, 1)
	require.ErrorContains(t, err, "maximum group count")
}

func TestR8cBatchCancellationDoesNotCallBackend(t *testing.T) {
	c := queryRound6Context()
	ctx, cancel := context.WithCancel(c.Request.Context())
	cancel()
	c.Request = c.Request.WithContext(ctx)
	e := queryBatchV2Executor{anonymousAccess: true, getDatasource: func(string, int64) (datasource.Datasource, bool) { return nil, false }}
	queries := []QueryBatchV2Query{}
	for _, ref := range []string{"A", "B", "C"} {
		queries = append(queries, QueryBatchV2Query{Kind: queryKindDatasource, RefID: ref, Datasource: &QueryBatchV2DatasourceRef{Cate: "fake", ID: 1}, ResultType: resultTypeLogs, Query: json.RawMessage(`{}`)})
	}
	response := e.execute(c, QueryBatchV2Request{Queries: queries})
	require.Len(t, response.Results, 3)
	for _, result := range response.Results {
		require.Equal(t, "DATASOURCE_TIMEOUT", result.Error.Code)
		require.Contains(t, result.Error.Message, "context canceled")
	}
}
func TestR8cBatchInvalidPayloadIsAnIndividualFailure(t *testing.T) {
	e := queryBatchV2Executor{anonymousAccess: true}
	q := QueryBatchV2Query{Kind: queryKindDatasource, RefID: "A", Datasource: &QueryBatchV2DatasourceRef{Cate: "x", ID: 1}, Query: json.RawMessage(`{`)}
	result := e.execute(queryRound6Context(), QueryBatchV2Request{Queries: []QueryBatchV2Query{q}}).Results[0]
	require.Equal(t, "INVALID_QUERY", result.Error.Code)
	require.Contains(t, result.Error.Message, "unexpected end")
}
