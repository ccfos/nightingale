package eval

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/hash"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestGetSamples_MatrixUsesLatestPointAndPreservesLabels(t *testing.T) {
	labels := model.Metric{"host": "node", "__name__": "cpu"}
	value := model.Matrix{{Metric: labels, Values: []model.SamplePair{
		{Timestamp: 1000, Value: 2}, {Timestamp: 2000, Value: 7},
	}}}

	samples := getSamples(value)

	require.Equal(t, []sample{{Metric: labels, Value: 7, Timestamp: 2000}}, samples)
}

func TestGetParamKeyAllLabel_DeduplicatesValuesFromQueryResults(t *testing.T) {
	cases := []struct {
		name  string
		value model.Value
	}{
		{
			name: "vector",
			value: model.Vector{
				{Metric: model.Metric{"host": "a"}}, {Metric: model.Metric{"host": "a"}},
				{Metric: model.Metric{"host": "b"}}, {Metric: model.Metric{"zone": "west"}},
			},
		},
		{
			name: "matrix",
			value: model.Matrix{
				{Metric: model.Metric{"host": "a"}}, {Metric: model.Metric{"host": "a"}},
				{Metric: model.Metric{"host": "b"}}, {Metric: model.Metric{"zone": "west"}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := &fixedAnomalyPromAPI{value: tc.value}

			values, err := getParamKeyAllLabel("host", `cpu{host="$host"} > 5`, api, 1, 2, getTestStats())

			require.NoError(t, err)
			require.ElementsMatch(t, []string{"a", "b"}, values)
			require.Equal(t, []string{`cpu{}`}, api.queries)
		})
	}
}

func TestGetPromAnomalyPoint_ChoosesVariableFillingStrategy(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		wantQuery string
	}{
		{name: "exact label", query: `cpu{ident="$ident"} > 1`, wantQuery: `cpu{} > 1`},
		{name: "aggregation", query: `sum(cpu{ident="$ident"}) > 1`, wantQuery: `sum(cpu{ident="node"}) > 1`},
		{name: "regex", query: `cpu{ident=~"$ident"} > 1`, wantQuery: `cpu{ident=~"node"} > 1`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			worker := newAnomalyTestWorker()
			worker.Processor.Stats = getTestStats()
			api := &fixedAnomalyPromAPI{value: model.Vector{{
				Metric: model.Metric{"ident": "node"}, Timestamp: 1700000000000, Value: 5,
			}}}
			attachAnomalyPromAPI(worker, api)
			config, err := json.Marshal(models.PromRuleConfig{Queries: []models.PromQuery{{
				PromQl: tc.query, Severity: 2, VarEnabled: true,
				VarConfig: models.VarConfig{ParamVal: []models.ParamQueryForFirst{{
					Name: "ident", ParamType: "enum", Query: []string{"node"},
				}}},
			}}})
			require.NoError(t, err)

			points, err := worker.GetPromAnomalyPoint(string(config))

			require.NoError(t, err)
			require.Len(t, points, 1)
			require.Equal(t, 5.0, points[0].Value)
			require.Equal(t, 2, points[0].Severity)
			require.Equal(t, "node", string(points[0].Labels["ident"]))
			require.Equal(t, []string{tc.wantQuery}, api.queries)
		})
	}
}

func TestGetPromAnomalyPoint_SkipsBlankQueries(t *testing.T) {
	worker := newAnomalyTestWorker()
	api := &fixedAnomalyPromAPI{}
	attachAnomalyPromAPI(worker, api)

	points, err := worker.GetPromAnomalyPoint(`{"queries":[{"prom_ql":"  "}]}`)

	require.NoError(t, err)
	require.Empty(t, points)
	require.Empty(t, api.queries)
}

func TestGetHostAnomalyPoint_RejectsMalformedAndNullConfiguration(t *testing.T) {
	for _, config := range []string{"{", "null"} {
		t.Run(config, func(t *testing.T) {
			worker := newAnomalyTestWorker()

			points, err := worker.GetHostAnomalyPoint(config)

			require.Error(t, err)
			require.Empty(t, points)
		})
	}
}

func TestGetHostAnomalyPoint_MissingTargetAssignmentDoesNotProduceAnomalies(t *testing.T) {
	worker := newAnomalyTestWorker()
	worker.Ctx = &ctx.Context{IsCenter: true}
	worker.Processor.TargetsOfAlertRuleCache = &memsto.TargetsOfAlertRuleCacheType{}
	worker.Processor.TargetCache = &memsto.TargetCacheType{}
	config := `{"inhibit":true,"triggers":[{"type":"target_miss"},{"type":"offset"},{"type":"pct_target_miss"}]}`

	points, err := worker.GetHostAnomalyPoint(config)

	require.NoError(t, err)
	require.Empty(t, points)
	require.True(t, worker.Inhibit)
}

func TestGetAnomalyPoint_NoDataExpiresPreviouslySeenSeries(t *testing.T) {
	worker := newAnomalyTestWorker()
	series := fakeSeries("A", model.Metric{"__name__": "cpu", "host": "node"}, 5)
	// The first series is safely in the past; the second is safely inside the retention window.
	series.Values = [][]float64{{float64(time.Now().Add(-2 * time.Hour).Unix()), 5}}
	recent := fakeSeries("A", model.Metric{"__name__": "cpu", "host": "recent"}, 6)
	recent.Values = [][]float64{{float64(time.Now().Unix()), 6}}
	worker.LastSeriesStore[1], worker.LastSeriesStore[2] = series, recent
	setAnomalyTestDatasource(t, worker, &fakeTriggerDatasource{series: map[string][]models.DataResp{}})
	setAnomalyRuleConfig(t, worker, models.RuleQuery{
		Queries: []interface{}{map[string]interface{}{"ref": "A"}}, ExpTriggerDisable: true,
		NodataTrigger: models.NodataTrigger{Enable: true, Severity: 3, ResolveAfterEnable: true, ResolveAfter: 3600},
	})

	points, recovered, err := worker.GetAnomalyPoint(worker.Rule, worker.DatasourceId)

	require.NoError(t, err)
	require.Empty(t, recovered)
	require.Len(t, points, 1)
	require.Equal(t, "recent", string(points[0].Labels["host"]))
	require.Equal(t, models.TriggerTypeNodata, points[0].TriggerType)
	require.NotContains(t, worker.LastSeriesStore, uint64(1))
	require.Contains(t, worker.LastSeriesStore, uint64(2))
}

func TestProcessJoins_SelectsSeriesBySharedLabels(t *testing.T) {
	cases := []struct {
		name     string
		joinType string
		want     [][]string
	}{
		{name: "inner", joinType: "inner_join", want: [][]string{{"A/shared", "B/shared"}}},
		{name: "left", joinType: "left_join", want: [][]string{{"A/shared", "B/shared"}, {"A/left"}}},
		{name: "right", joinType: "right_join", want: [][]string{{"A/shared", "B/shared"}, {"B/right"}}},
		{name: "exclude left", joinType: "left_exclude", want: [][]string{{"A/left"}}},
		{name: "exclude right", joinType: "right_exclude", want: [][]string{{"B/right"}}},
		{
			name: "cartesian", joinType: "cartesian",
			want: [][]string{
				{"A/shared", "B/shared"}, {"A/shared", "B/right"},
				{"A/left", "B/shared"}, {"A/left", "B/right"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := make(map[uint64]models.DataResp)
			indexes := map[string]map[uint64][]uint64{"A": {}, "B": {}}
			MakeSeriesMap([]models.DataResp{
				fakeSeries("A", model.Metric{"host": "shared", "metric": "cpu"}, 1),
				fakeSeries("A", model.Metric{"host": "left", "metric": "cpu"}, 2),
			}, indexes["A"], store)
			MakeSeriesMap([]models.DataResp{
				fakeSeries("B", model.Metric{"host": "shared", "metric": "memory"}, 3),
				fakeSeries("B", model.Metric{"host": "right", "metric": "memory"}, 4),
			}, indexes["B"], store)
			trigger := models.Trigger{
				JoinRef: "A",
				Joins:   []models.Join{{JoinType: tc.joinType, Ref: "B", On: []string{"host"}}},
			}

			joined := ProcessJoins(1, trigger, indexes, store)

			groups := make([][]string, 0, len(joined))
			for _, ids := range joined {
				group := make([]string, 0, len(ids))
				for _, id := range ids {
					series := store[id]
					require.Equal(t, id, hash.GetHash(series.Metric, series.Ref))
					group = append(group, series.Ref+"/"+string(series.Metric["host"]))
				}
				// Join groups have no ordering contract.
				if len(group) == 2 && group[0] > group[1] {
					group[0], group[1] = group[1], group[0]
				}
				groups = append(groups, group)
			}
			require.ElementsMatch(t, tc.want, groups)
		})
	}
}
