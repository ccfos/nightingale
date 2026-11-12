package eval

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/evallog"
	"github.com/glebarez/sqlite"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"github.com/toolkits/pkg/container/list"
	"gorm.io/gorm"
)

func eventWorker(t *testing.T, cate string) *AlertRuleWorker {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.AlertCurEvent{}))
	old := queue.EventQueue
	queue.EventQueue = list.NewSafeListLimited(100)
	t.Cleanup(func() { queue.EventQueue = old })
	base := newAnomalyTestWorker()
	base.Processor.Stats.CounterRuleEval = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_eval_cycles"}, nil)
	rule := &models.AlertRule{Id: 7302, Cate: cate, Prod: models.METRIC, Name: "CPU high", Annotations: "{}"}
	rules := &memsto.AlertRuleCacheType{}
	rules.Set(map[int64]*models.AlertRule{rule.Id: rule}, 1, 1)
	c := ctx.NewContext(context.Background(), db, true)
	processor := process.NewProcessor("local", rule, 7301, rules,
		&memsto.TargetCacheType{}, &memsto.TargetsOfAlertRuleCacheType{}, &memsto.BusiGroupCacheType{},
		&memsto.AlertMuteCacheType{}, &memsto.DatasourceCacheType{}, c, base.Processor.Stats)
	worker := NewAlertRuleWorker(rule, 7301, processor, nil, c)
	worker.Prepare()
	return worker
}

func TestEval_ConvertsDatasourceThresholdAndRecoveryIntoEvents(t *testing.T) {
	worker := eventWorker(t, "worker-round5")
	series := fakeSeries("A", model.Metric{"__name__": "cpu", "host": "node-1"}, 95)
	ds := &fakeTriggerDatasource{series: map[string][]models.DataResp{"A": {series}}}
	setAnomalyTestDatasource(t, worker, ds)
	setAnomalyRuleConfig(t, worker, models.RuleQuery{
		Queries: []interface{}{map[string]interface{}{"ref": "A"}},
		Triggers: []models.Trigger{{
			Exp: "$A > 90", Severity: 2,
			RecoverConfig: models.RecoverConfig{JudgeType: models.RecoverOnCondition, RecoverExp: "$A < 80"},
		}},
	})

	worker.Eval()

	items := queue.EventQueue.PopBackBy(10)
	require.Len(t, items, 1)
	firing := items[0].(*models.AlertCurEvent)
	require.False(t, firing.IsRecovered)
	require.Equal(t, worker.Rule.Id, firing.RuleId)
	require.Equal(t, worker.DatasourceId, firing.DatasourceId)
	require.Equal(t, 2, firing.Severity)
	require.Equal(t, "95.00", firing.TriggerValue)
	require.Equal(t, "node-1", firing.TagsMap["host"])
	require.Nil(t, worker.EvalRec)
	require.Nil(t, worker.Processor.EvalRec)
	series.Values[0][1] = 75

	worker.Eval()

	items = queue.EventQueue.PopBackBy(10)
	require.Len(t, items, 1)
	recovered := items[0].(*models.AlertCurEvent)
	require.True(t, recovered.IsRecovered)
	require.Equal(t, firing.Hash, recovered.Hash)
	require.Equal(t, "75", recovered.TriggerValue)
	require.Equal(t, "95.00", firing.TriggerValue)
	require.Equal(t, 2.0, testutil.ToFloat64(worker.Processor.Stats.CounterRuleEval.WithLabelValues()))
	require.Nil(t, worker.EvalRec)
	require.Nil(t, worker.Processor.EvalRec)
}

func TestEval_PrometheusAndLokiProduceEventsFromVector(t *testing.T) {
	for _, cate := range []string{models.PROMETHEUS, models.LOKI} {
		t.Run(cate, func(t *testing.T) {
			worker := eventWorker(t, cate)
			worker.Rule.RuleConfig = `{"queries":[{"prom_ql":"cpu > 90","severity":1}]}`
			api := &fixedAnomalyPromAPI{value: model.Vector{{
				Metric: model.Metric{"host": "node-1"}, Timestamp: 1700000000000, Value: 95,
			}}}
			attachAnomalyPromAPI(worker, api)

			worker.Eval()

			require.Equal(t, []string{"cpu > 90"}, api.queries)
			items := queue.EventQueue.PopBackBy(10)
			require.Len(t, items, 1)
			event := items[0].(*models.AlertCurEvent)
			require.Equal(t, cate, event.Cate)
			require.Equal(t, "95.00", event.TriggerValue)
			require.Equal(t, "cpu > 90", event.PromQl)
			require.Equal(t, int64(1700000000), event.TriggerTime)
			require.Equal(t, 1, event.Severity)
		})
	}
}

func TestEval_QueryFailureProducesNoEventsAndClearsRecord(t *testing.T) {
	worker := eventWorker(t, "worker-round5")
	setAnomalyTestDatasource(t, worker, &failingAnomalyDatasource{})
	setAnomalyRuleConfig(t, worker, models.RuleQuery{Queries: []interface{}{map[string]interface{}{"ref": "A"}}})
	worker.HostAndDeviceIdentCache.Store("old", []string{"stale"})

	worker.Eval()

	require.Zero(t, queue.EventQueue.Len())
	_, cached := worker.HostAndDeviceIdentCache.Load("old")
	require.False(t, cached)
	require.Nil(t, worker.EvalRec)
	require.Nil(t, worker.Processor.EvalRec)
	counter := worker.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues("7301", GET_CLIENT, "", "7302")
	require.Equal(t, 1.0, testutil.ToFloat64(counter))
}

func TestGetAnomalyPoint_InvalidConfigurationReturnsError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config string
		want   string
	}{
		{name: "blank", config: "  ", want: "ruleConfig is blank"},
		{name: "malformed", config: "{", want: "promql parse error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worker := newAnomalyTestWorker()
			worker.Rule.RuleConfig = tc.config

			points, recovered, err := worker.GetAnomalyPoint(worker.Rule, worker.DatasourceId)

			require.ErrorContains(t, err, tc.want)
			require.Empty(t, points)
			require.Empty(t, recovered)
		})
	}
}

func TestGetAnomalyPoint_MissingDatasourcePreservesHistory(t *testing.T) {
	worker := newAnomalyTestWorker()
	worker.Rule.Cate = "unregistered-round5"
	previous := fakeSeries("A", model.Metric{"host": "node"}, 1)
	worker.LastSeriesStore[1] = previous
	setAnomalyRuleConfig(t, worker, models.RuleQuery{Queries: []interface{}{map[string]interface{}{"ref": "A"}}})

	points, recovered, err := worker.GetAnomalyPoint(worker.Rule, worker.DatasourceId)

	require.ErrorContains(t, err, "not exists")
	require.Empty(t, points)
	require.Empty(t, recovered)
	require.Equal(t, map[uint64]models.DataResp{1: previous}, worker.LastSeriesStore)
}

func TestGetAnomalyPoint_RecordsQuerySamples(t *testing.T) {
	round4EvalLog(t)
	worker := newAnomalyTestWorker()
	worker.EvalRec = &evallog.EvalRecord{}
	series := fakeSeries("A", model.Metric{"host": "node-1"}, 95)
	setAnomalyTestDatasource(t, worker, &fakeTriggerDatasource{series: map[string][]models.DataResp{"A": {series}}})
	setAnomalyRuleConfig(t, worker, models.RuleQuery{
		Queries: []interface{}{map[string]interface{}{"ref": "A"}}, ExpTriggerDisable: true,
	})

	points, recovered, err := worker.GetAnomalyPoint(worker.Rule, worker.DatasourceId)

	require.NoError(t, err)
	require.Empty(t, points)
	require.Empty(t, recovered)
	require.Len(t, worker.EvalRec.Queries, 1)
	query := worker.EvalRec.Queries[0]
	require.Equal(t, "A", query.Ref)
	require.JSONEq(t, `{"ref":"A"}`, query.Query)
	require.Equal(t, 1, query.SeriesTotal)
	require.Equal(t, []evallog.SeriesSample{{
		Labels: map[string]string{"host": "node-1"}, Points: [][2]float64{{1700000000, 95}},
	}}, query.Series)
}

func TestGetAnomalyPoint_RecordsQueryFailure(t *testing.T) {
	round4EvalLog(t)
	worker := newAnomalyTestWorker()
	worker.EvalRec = &evallog.EvalRecord{}
	setAnomalyTestDatasource(t, worker, &failingAnomalyDatasource{})
	setAnomalyRuleConfig(t, worker, models.RuleQuery{Queries: []interface{}{map[string]interface{}{"ref": "A"}}})

	_, _, err := worker.GetAnomalyPoint(worker.Rule, worker.DatasourceId)

	require.Error(t, err)
	require.Len(t, worker.EvalRec.Queries, 1)
	require.Equal(t, "A", worker.EvalRec.Queries[0].Ref)
	require.Equal(t, "datasource unavailable", worker.EvalRec.Queries[0].Error)
	require.Empty(t, worker.EvalRec.Queries[0].Series)
}

func TestAlertRuleWorker_HashTracksEvaluationConfiguration(t *testing.T) {
	worker := eventWorker(t, "worker-round5")
	original := worker.Hash()

	worker.Rule.Name = "renamed"

	require.Equal(t, original, worker.Hash())
	require.Equal(t, "alert-7301-7302", worker.Key())
	worker.Rule.RuleConfig = "{}"
	require.NotEqual(t, original, worker.Hash())
}

func TestAlertRuleWorker_StopClosesQuit(t *testing.T) {
	worker := eventWorker(t, "worker-round5")

	worker.Stop()

	select {
	case <-worker.Quit:
	default:
		t.Fatal("Stop did not close Quit")
	}
}

// Keep error identity available to callers of the legacy edge host lookup.
func TestGetHostIdents_EdgeTransportErrorIsParameterFetchError(t *testing.T) {
	worker := newAnomalyTestWorker()
	worker.Ctx = ctx.NewContext(context.Background(), nil, false)
	worker.Processor.TargetCache = nil

	idents, err := worker.getHostIdents(models.ParamQuery{Query: []models.HostQuery{}})

	require.Empty(t, idents)
	require.ErrorIs(t, err, errParamValueFetch)
}
