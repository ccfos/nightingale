package record

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"
	promapi "github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

type recordQueryFake struct {
	promapi.API
	value    model.Value
	warnings promapi.Warnings
	err      error
	queries  []string
}

func (f *recordQueryFake) Query(_ context.Context, query string, _ time.Time) (model.Value, promapi.Warnings, error) {
	f.queries = append(f.queries, query)
	return f.value, f.warnings, f.err
}

type recordWriteFake struct {
	requests []*http.Request
	batches  []prompb.WriteRequest
	status   int
}

func (f *recordWriteFake) URL(path string, _ map[string]string) *url.URL {
	return &url.URL{Scheme: "http", Host: "unused.invalid", Path: path}
}

func (f *recordWriteFake) Do(_ context.Context, req *http.Request) (*http.Response, []byte, error) {
	f.requests = append(f.requests, req)
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, nil, err
	}
	decoded, err := snappy.Decode(nil, data)
	if err != nil {
		return nil, nil, err
	}
	var batch prompb.WriteRequest
	if err := batch.Unmarshal(decoded); err != nil {
		return nil, nil, err
	}
	f.batches = append(f.batches, batch)
	return &http.Response{StatusCode: f.status}, []byte("write response"), nil
}

func recordEvalFixture(t *testing.T) (*RecordRuleContext, *recordQueryFake, *recordWriteFake) {
	t.Helper()
	query := &recordQueryFake{value: model.Vector{}}
	sink := &recordWriteFake{status: http.StatusNoContent}
	clients := &prom.PromClientMap{}
	clients.Reset()
	clients.Set(7, query, promapi.WriterType{
		Client: sink, Opts: promapi.ClientOptions{Url: "http://unused.invalid/write"},
	})
	stats := &astats.Stats{
		CounterRecordEval: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_record_evals"}, []string{"ds", "rule"},
		),
		CounterRecordEvalErrorTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_record_errors"}, []string{"ds", "query_ds", "stage", "rule"},
		),
		GaugeRecordEvalDuration: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "test_record_duration"}, []string{"rule", "ds"},
		),
		GaugeRecordSeriesCount: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "test_record_series"}, []string{"rule", "ds"},
		),
	}
	rrc := NewRecordRuleContext(&models.RecordingRule{Id: 42, Name: "job:requests", PromQl: "  requests_total  ",
		PromEvalInterval: 60}, 7, clients, nil, stats)
	t.Cleanup(rrc.Stop)
	return rrc, query, sink
}

func TestRecordRuleEval_WritesConvertedQueryResult(t *testing.T) {
	rrc, query, sink := recordEvalFixture(t)
	query.value = model.Vector{&model.Sample{Metric: model.Metric{"job": "api"}, Value: 12, Timestamp: 1700000000000}}

	rrc.Eval()

	require.Equal(t, []string{"requests_total"}, query.queries)
	require.Len(t, sink.requests, 1)
	require.Equal(t, "/write", sink.requests[0].URL.Path)
	require.Equal(t, "snappy", sink.requests[0].Header.Get("Content-Encoding"))
	require.Len(t, sink.batches, 1)
	require.Equal(t, []prompb.TimeSeries{{
		Labels:  []prompb.Label{{Name: "__name__", Value: "job:requests"}, {Name: "job", Value: "api"}},
		Samples: []prompb.Sample{{Value: 12, Timestamp: 1700000000000}},
	}}, sink.batches[0].Timeseries)
	require.Equal(t, float64(1), testutil.ToFloat64(rrc.stats.GaugeRecordSeriesCount.WithLabelValues("42", "7")))
	require.Equal(t, float64(1), testutil.ToFloat64(rrc.stats.CounterRecordEval.WithLabelValues("7", "42")))
}

func TestRecordRuleEval_EmptyResultSkipsWrite(t *testing.T) {
	rrc, query, sink := recordEvalFixture(t)

	rrc.Eval()

	require.Len(t, query.queries, 1)
	require.Empty(t, sink.requests)
	require.Zero(t, testutil.ToFloat64(rrc.stats.GaugeRecordSeriesCount.WithLabelValues("42", "7")))
}

// Negative series counts distinguish unavailable clients (-2) from query failures (-1).
func TestRecordRuleEval_ReportsFailureStage(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*RecordRuleContext, *recordQueryFake, *recordWriteFake)
		stage   string
		series  float64
		queries int
		writes  int
	}{
		{name: "blank query", prepare: func(r *RecordRuleContext, _ *recordQueryFake, _ *recordWriteFake) {
			r.rule.PromQl = " \n "
		}, stage: StageCheckQuery},
		{name: "missing reader", prepare: func(r *RecordRuleContext, _ *recordQueryFake, _ *recordWriteFake) {
			r.promClients.Del(7)
		}, stage: StageGetClient, series: -2},
		{name: "query failure", prepare: func(_ *RecordRuleContext, q *recordQueryFake, _ *recordWriteFake) {
			q.err = errors.New("query failed")
		}, stage: StageQueryData, series: -1, queries: 1},
		{name: "query warning", prepare: func(_ *RecordRuleContext, q *recordQueryFake, _ *recordWriteFake) {
			q.warnings = promapi.Warnings{"partial response"}
		}, stage: StageQueryData, series: -1, queries: 1},
		{name: "write failure", prepare: func(_ *RecordRuleContext, q *recordQueryFake, w *recordWriteFake) {
			q.value = model.Vector{&model.Sample{Value: 3, Timestamp: 1000}}
			w.status = http.StatusServiceUnavailable
		}, stage: StageWriteData, series: 1, queries: 1, writes: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rrc, query, sink := recordEvalFixture(t)
			tt.prepare(rrc, query, sink)

			rrc.Eval()

			require.Len(t, query.queries, tt.queries)
			require.Len(t, sink.requests, tt.writes)
			counter := rrc.stats.CounterRecordEvalErrorTotal.WithLabelValues("7", "7", tt.stage, "42")
			require.Equal(t, float64(1), testutil.ToFloat64(counter))
			require.Equal(t, tt.series, testutil.ToFloat64(rrc.stats.GaugeRecordSeriesCount.WithLabelValues("42", "7")))
		})
	}
}

func TestRecordRuleContext_DefaultIntervalRegistersSchedule(t *testing.T) {
	rrc, _, _ := recordEvalFixture(t)

	require.Equal(t, "@every 60s", rrc.rule.CronPattern)
	require.Len(t, rrc.scheduler.Entries(), 1)
	require.Equal(t, "record-7-42", rrc.Key())
}

func TestRecordRuleContext_InvalidCronDoesNotRegisterSchedule(t *testing.T) {
	rrc := NewRecordRuleContext(&models.RecordingRule{CronPattern: "invalid cron"}, 7, nil, nil, nil)
	t.Cleanup(rrc.Stop)

	require.Empty(t, rrc.scheduler.Entries())
}

func TestRecordRuleContext_HashTracksEvaluationConfiguration(t *testing.T) {
	original := models.RecordingRule{Id: 1, Name: "sum:requests", PromQl: "sum(requests)", CronPattern: "@every 30s"}
	tests := []struct {
		name   string
		change func(*RecordRuleContext)
	}{
		{name: "id", change: func(r *RecordRuleContext) { r.rule.Id++ }},
		{name: "name", change: func(r *RecordRuleContext) { r.rule.Name = "other" }},
		{name: "query", change: func(r *RecordRuleContext) { r.rule.PromQl = "up" }},
		{name: "schedule", change: func(r *RecordRuleContext) { r.rule.CronPattern = "@every 60s" }},
		{name: "tags", change: func(r *RecordRuleContext) { r.rule.AppendTags = "env=prod" }},
		{name: "datasource", change: func(r *RecordRuleContext) { r.datasourceId++ }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := original
			rrc := &RecordRuleContext{rule: &rule, datasourceId: 7}
			before := rrc.Hash()

			tt.change(rrc)

			require.NotEqual(t, before, rrc.Hash())
		})
	}
}

func TestConvertToTimeSeries_VectorFiltersNaNAndReplacesMetricName(t *testing.T) {
	rule := &models.RecordingRule{Name: "recorded", AppendTagsJSON: []string{"env=prod", "bad-label=ignored", "expr=a=b"}}
	input := model.Vector{
		&model.Sample{Metric: model.Metric{"__name__": "original", "job": "api", "bad-label": "ignored"},
			Value: 2.5, Timestamp: 1700000000000},
		&model.Sample{Value: model.SampleValue(math.NaN()), Timestamp: 1700000000000},
	}

	got := ConvertToTimeSeries(input, rule)

	require.Len(t, got, 1)
	require.Equal(t, []prompb.Sample{{Value: 2.5, Timestamp: 1700000000000}}, got[0].Samples)
	require.ElementsMatch(t, []prompb.Label{
		{Name: "__name__", Value: "recorded"}, {Name: "job", Value: "api"},
		{Name: "env", Value: "prod"}, {Name: "expr", Value: "a=b"},
	}, got[0].Labels)
}

func TestConvertToTimeSeries_MatrixFiltersInvalidSamples(t *testing.T) {
	input := model.Matrix{
		&model.SampleStream{Metric: model.Metric{"job": "api"}, Values: []model.SamplePair{
			{Timestamp: 1000, Value: 1}, {Timestamp: 2000, Value: model.SampleValue(math.NaN())}, {Timestamp: 3000, Value: 3},
		}},
		&model.SampleStream{Values: []model.SamplePair{{Timestamp: 3000, Value: model.SampleValue(math.NaN())}}},
	}

	got := ConvertToTimeSeries(input, &models.RecordingRule{Name: "recorded"})

	require.Len(t, got, 1)
	require.Equal(t, []prompb.Sample{{Timestamp: 1000, Value: 1}, {Timestamp: 3000, Value: 3}}, got[0].Samples)
	require.ElementsMatch(t, []prompb.Label{
		{Name: "__name__", Value: "recorded"}, {Name: "job", Value: "api"},
	}, got[0].Labels)
}

func TestConvertToTimeSeries_DropsNaNScalarAndUnsupportedString(t *testing.T) {
	tests := []struct {
		name  string
		value model.Value
	}{
		{name: "NaN", value: &model.Scalar{Value: model.SampleValue(math.NaN())}},
		{name: "string", value: &model.String{Value: "unsupported"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertToTimeSeries(tt.value, &models.RecordingRule{})

			require.Empty(t, got)
		})
	}
}
