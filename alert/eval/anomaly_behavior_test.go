package eval

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	promsdk "github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func newAnomalyTestWorker() *AlertRuleWorker {
	// Unregistered collectors keep each test independent of the global registry.
	stats := &astats.Stats{
		GaugeRuleEvalDuration: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "test_eval_duration"}, []string{"rule", "datasource"}),
		GaugeQuerySeriesCount: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "test_eval_series"}, []string{"rule", "datasource", "ref"}),
		CounterQueryDataTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_eval_queries"}, []string{"datasource", "rule"}),
		CounterQueryDataErrorTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_eval_query_errors"}, []string{"datasource"}),
		CounterRuleEvalErrorTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_eval_errors"}, []string{"datasource", "stage", "group", "rule"}),
	}
	return &AlertRuleWorker{
		DatasourceId: 7301, Rule: &models.AlertRule{Id: 7302, Cate: "anomaly-behavior-test", Prod: models.METRIC},
		Processor:       &process.Processor{Stats: stats, BusiGroupCache: &memsto.BusiGroupCacheType{}},
		LastSeriesStore: make(map[uint64]models.DataResp),
	}
}

func setAnomalyTestDatasource(t *testing.T, worker *AlertRuleWorker, ds datasource.Datasource) {
	t.Helper()
	cate, id := worker.Rule.Cate, worker.DatasourceId
	previous, existed := dscache.DsCache.Get(cate, id)
	dscache.DsCache.Put(cate, id, ds)
	t.Cleanup(func() {
		if existed {
			dscache.DsCache.Put(cate, id, previous)
		} else {
			dscache.DsCache.Delete(cate, id)
		}
	})
}

func setAnomalyRuleConfig(t *testing.T, worker *AlertRuleWorker, config models.RuleQuery) {
	t.Helper()
	encoded, err := json.Marshal(config)
	require.NoError(t, err)
	worker.Rule.RuleConfig = string(encoded)
}

func TestGetAnomalyPoint_ThresholdAndConditionalRecovery(t *testing.T) {
	tests := []struct {
		name        string
		value       float64
		judge       models.RecoverJudge
		wantFiring  int
		wantRecover int
	}{
		{name: "above threshold", value: 91, judge: models.Origin, wantFiring: 1},
		{name: "at threshold", value: 90, judge: models.Origin, wantRecover: 1},
		{name: "recovery hysteresis", value: 85, judge: models.RecoverOnCondition},
		{name: "recovery boundary", value: 80, judge: models.RecoverOnCondition},
		{name: "recovery fulfilled", value: 79, judge: models.RecoverOnCondition, wantRecover: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := newAnomalyTestWorker()
			labels := model.Metric{"__name__": "cpu", "host": "node-1"}
			ds := &fakeTriggerDatasource{series: map[string][]models.DataResp{"A": {fakeSeries("A", labels, tt.value)}}}
			setAnomalyTestDatasource(t, worker, ds)
			setAnomalyRuleConfig(t, worker, models.RuleQuery{
				Queries: []interface{}{map[string]interface{}{"ref": "A", "unit": "none"}},
				Triggers: []models.Trigger{{
					Exp: "$A > 90", Severity: 2,
					RecoverConfig: models.RecoverConfig{JudgeType: tt.judge, RecoverExp: "$A < 80"},
				}},
			})

			points, recovered, err := worker.GetAnomalyPoint(worker.Rule, worker.DatasourceId)

			require.NoError(t, err)
			require.Len(t, points, tt.wantFiring)
			require.Len(t, recovered, tt.wantRecover)
			for _, point := range append(points, recovered...) {
				require.Equal(t, "cpu", point.Key)
				require.Equal(t, labels, point.Labels)
				require.Equal(t, int64(1700000000), point.Timestamp)
				require.Equal(t, tt.value, point.Value)
				require.Equal(t, 2, point.Severity)
				require.Equal(t, tt.wantFiring == 1, point.Triggered)
				require.Equal(t, tt.judge, point.RecoverConfig.JudgeType)
				require.Contains(t, point.Query, "$A > 90")
			}
		})
	}
}

func TestGetAnomalyPoint_ExposesMetricAndLabelVariables(t *testing.T) {
	worker := newAnomalyTestWorker()
	labels := model.Metric{"__name__": "cpu", "env": "prod"}
	series := fakeSeries("A", labels, 95)
	series.Query = "SELECT cpu FROM metrics"
	setAnomalyTestDatasource(t, worker, &fakeTriggerDatasource{series: map[string][]models.DataResp{"A": {series}}})
	setAnomalyRuleConfig(t, worker, models.RuleQuery{
		Queries:  []interface{}{map[string]interface{}{"ref": "A", "unit": "none"}},
		Triggers: []models.Trigger{{Exp: `$A.cpu > 90 && $A.env == "prod"`, Severity: 1}},
	})

	points, recovered, err := worker.GetAnomalyPoint(worker.Rule, worker.DatasourceId)

	require.NoError(t, err)
	require.Empty(t, recovered)
	require.Len(t, points, 1)
	require.True(t, points[0].Triggered)
	require.Contains(t, points[0].Values, "$A.env:prod")
	require.Contains(t, points[0].Values, "$A.cpu:")
	require.Contains(t, points[0].Query, series.Query)
	require.Contains(t, points[0].ValuesUnit, "$A.cpu")
}

func TestGetAnomalyPoint_NoDataTracksMissingAndReturningSeries(t *testing.T) {
	worker := newAnomalyTestWorker()
	series := fakeSeries("A", model.Metric{"__name__": "cpu", "host": "node-1"}, 5)
	ds := &fakeTriggerDatasource{series: map[string][]models.DataResp{"A": {series}}}
	setAnomalyTestDatasource(t, worker, ds)
	setAnomalyRuleConfig(t, worker, models.RuleQuery{
		Queries: []interface{}{map[string]interface{}{"ref": "A"}}, ExpTriggerDisable: true,
		NodataTrigger: models.NodataTrigger{Enable: true, Severity: 3},
	})
	points, recovered, err := worker.GetAnomalyPoint(worker.Rule, worker.DatasourceId)
	require.NoError(t, err)
	require.Empty(t, points)
	require.Empty(t, recovered)
	require.Len(t, worker.LastSeriesStore, 1)
	ds.series["A"] = nil

	points, recovered, err = worker.GetAnomalyPoint(worker.Rule, worker.DatasourceId)

	require.NoError(t, err)
	require.Empty(t, recovered)
	require.Len(t, points, 1)
	require.Equal(t, "cpu", points[0].Key)
	require.Equal(t, series.Metric, points[0].Labels)
	require.Equal(t, models.TriggerTypeNodata, points[0].TriggerType)
	require.Equal(t, 3, points[0].Severity)
	require.True(t, points[0].Triggered)
	require.Contains(t, points[0].Values, "nodata since ")
	require.Contains(t, points[0].Query, `host="node-1"`)
	ds.series["A"] = []models.DataResp{series}

	points, _, err = worker.GetAnomalyPoint(worker.Rule, worker.DatasourceId)

	require.NoError(t, err)
	require.Empty(t, points)
}

func TestGetAnomalyPoint_EmptyFirstQueryDoesNotInventNoData(t *testing.T) {
	worker := newAnomalyTestWorker()
	setAnomalyTestDatasource(t, worker, &fakeTriggerDatasource{series: map[string][]models.DataResp{}})
	setAnomalyRuleConfig(t, worker, models.RuleQuery{
		Queries:       []interface{}{map[string]interface{}{"ref": "A"}},
		NodataTrigger: models.NodataTrigger{Enable: true, Severity: 3},
	})

	points, recovered, err := worker.GetAnomalyPoint(worker.Rule, worker.DatasourceId)

	require.NoError(t, err)
	require.Empty(t, points)
	require.Empty(t, recovered)
	require.Empty(t, worker.LastSeriesStore)
}

type failingAnomalyDatasource struct {
	fakeTriggerDatasource
}

func (f *failingAnomalyDatasource) QueryData(context.Context, interface{}) ([]models.DataResp, error) {
	return nil, errors.New("datasource unavailable")
}

func TestGetAnomalyPoint_QueryErrorPreservesNoDataHistory(t *testing.T) {
	worker := newAnomalyTestWorker()
	previous := fakeSeries("A", model.Metric{"host": "node-1"}, 5)
	worker.LastSeriesStore[1] = previous
	setAnomalyTestDatasource(t, worker, &failingAnomalyDatasource{})
	setAnomalyRuleConfig(t, worker, models.RuleQuery{
		Queries:       []interface{}{map[string]interface{}{"ref": "A"}},
		NodataTrigger: models.NodataTrigger{Enable: true, Severity: 3},
	})

	points, recovered, err := worker.GetAnomalyPoint(worker.Rule, worker.DatasourceId)

	require.ErrorContains(t, err, "datasource unavailable")
	require.Empty(t, points)
	require.Empty(t, recovered)
	require.Equal(t, map[uint64]models.DataResp{1: previous}, worker.LastSeriesStore)
}

type fixedAnomalyPromAPI struct {
	promsdk.API
	value    model.Value
	err      error
	warnings promsdk.Warnings
	queries  []string
}

func (f *fixedAnomalyPromAPI) Query(_ context.Context, query string, _ time.Time) (model.Value, promsdk.Warnings, error) {
	f.queries = append(f.queries, query)
	return f.value, f.warnings, f.err
}

func attachAnomalyPromAPI(worker *AlertRuleWorker, api *fixedAnomalyPromAPI) {
	worker.PromClients = &prom.PromClientMap{ReaderClients: map[int64]promsdk.API{worker.DatasourceId: api}}
}

func TestGetPromAnomalyPoint_ConvertsVectorAndPreservesQueryMetadata(t *testing.T) {
	worker := newAnomalyTestWorker()
	labels := model.Metric{"__name__": "memory", "host": "node-1"}
	api := &fixedAnomalyPromAPI{value: model.Vector{
		{Metric: labels, Timestamp: model.Time(1700000000000), Value: 2048},
		{Metric: model.Metric{"host": "nan"}, Value: model.SampleValue(math.NaN())},
	}}
	attachAnomalyPromAPI(worker, api)
	config := `{"inhibit":true,"queries":[{"prom_ql":"  memory > 1024  ","severity":2,"unit":"bytesIEC"}]}`

	points, err := worker.GetPromAnomalyPoint(config)

	require.NoError(t, err)
	require.Equal(t, []string{"memory > 1024"}, api.queries)
	require.True(t, worker.Inhibit)
	require.Len(t, points, 1)
	require.Equal(t, labels, points[0].Labels)
	require.Equal(t, int64(1700000000), points[0].Timestamp)
	require.Equal(t, 2048.0, points[0].Value)
	require.Equal(t, 2, points[0].Severity)
	require.Equal(t, "memory > 1024", points[0].Query)
	require.Equal(t, "KiB", points[0].ValuesUnit["v"].Unit)
	require.Equal(t, 2.0, points[0].ValuesUnit["v"].Value)
}

func TestGetPromAnomalyPoint_UsesLatestMatrixSample(t *testing.T) {
	worker := newAnomalyTestWorker()
	api := &fixedAnomalyPromAPI{value: model.Matrix{{
		Metric: model.Metric{"host": "node-1"},
		Values: []model.SamplePair{{Timestamp: 1700000000000, Value: 1}, {Timestamp: 1700000001000, Value: 2}},
	}}}
	attachAnomalyPromAPI(worker, api)

	points, err := worker.GetPromAnomalyPoint(`{"queries":[{"prom_ql":"up","severity":1}]}`)

	require.NoError(t, err)
	require.Len(t, points, 1)
	require.Equal(t, 2.0, points[0].Value)
	require.Equal(t, int64(1700000001), points[0].Timestamp)
}

func TestGetPromAnomalyPoint_PropagatesQueryError(t *testing.T) {
	worker := newAnomalyTestWorker()
	queryErr := errors.New("query unavailable")
	api := &fixedAnomalyPromAPI{err: queryErr}
	attachAnomalyPromAPI(worker, api)

	points, err := worker.GetPromAnomalyPoint(`{"queries":[{"prom_ql":"up","severity":1}]}`)

	require.ErrorIs(t, err, queryErr)
	require.Empty(t, points)
}

func TestGetPromAnomalyPoint_WarningsRetainValidSamples(t *testing.T) {
	worker := newAnomalyTestWorker()
	api := &fixedAnomalyPromAPI{
		value: model.Vector{{Value: 1}}, warnings: promsdk.Warnings{"partial response"},
	}
	attachAnomalyPromAPI(worker, api)

	points, err := worker.GetPromAnomalyPoint(`{"queries":[{"prom_ql":"up","severity":1}]}`)

	require.NoError(t, err)
	require.Len(t, points, 1)
	require.Equal(t, 1.0, points[0].Value)
}

func TestGetPromAnomalyPoint_RejectsInvalidRuleConfig(t *testing.T) {
	for _, config := range []string{`{`, `null`} {
		worker := newAnomalyTestWorker()

		points, err := worker.GetPromAnomalyPoint(config)

		require.Error(t, err)
		require.Empty(t, points)
	}
}

func TestExecuteQueryTemplate_RendersSQLWithVariablesAndFunctions(t *testing.T) {
	query := map[string]interface{}{"ref": "A", "sql": `select {{add .Offset 1}} where env='{{.Env}}'`}

	err := ExecuteQueryTemplate("mysql", query, map[string]interface{}{"Offset": 2, "Env": "prod"})

	require.NoError(t, err)
	require.Equal(t, "select 3 where env='prod'", query["sql"])
	require.Equal(t, "A", query["ref"])
}

func TestExecuteQueryTemplate_ErrorPreservesOriginalSQL(t *testing.T) {
	query := map[string]interface{}{"sql": `select {{.Missing}}`}

	err := ExecuteQueryTemplate("mysql", query, nil)

	require.ErrorContains(t, err, "query tmpl execute error")
	require.Equal(t, `select {{.Missing}}`, query["sql"])
}

func TestExecuteSqlTemplate_ReportsParseError(t *testing.T) {
	got, err := ExecuteSqlTemplate(`select {{unknown}}`, nil)

	require.ErrorContains(t, err, "query tmpl parse error")
	require.Empty(t, got)
}

func TestExecuteSqlTemplate_PreservesPlainSQL(t *testing.T) {
	got, err := ExecuteSqlTemplate("select 1", nil)

	require.NoError(t, err)
	require.Equal(t, "select 1", got)
}

func TestGetQueryRef_AcceptsMapsAndStructs(t *testing.T) {
	tests := []struct {
		name  string
		query interface{}
	}{
		{name: "map", query: map[string]interface{}{"ref": "A"}},
		{name: "struct", query: struct{ Ref string }{Ref: "A"}},
		{name: "pointer", query: &struct{ Ref string }{Ref: "A"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetQueryRef(tt.query)

			require.NoError(t, err)
			require.Equal(t, "A", got)
		})
	}
}

func TestGetQueryRef_RejectsMissingOrNonStringRef(t *testing.T) {
	for _, query := range []interface{}{
		map[string]interface{}{}, map[string]interface{}{"ref": 1},
		struct{}{}, struct{ Ref int }{Ref: 1}, "A", nil,
	} {
		got, err := GetQueryRef(query)

		require.Error(t, err)
		require.Empty(t, got)
	}
}
