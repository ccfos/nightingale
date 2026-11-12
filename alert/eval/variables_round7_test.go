package eval

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestVarFillingAfterQuery_SubstitutesThresholdValues(t *testing.T) {
	for _, tc := range []struct {
		name      string
		threshold interface{}
	}{
		{name: "string", threshold: "7.5"},
		{name: "number", threshold: float64(7.5)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worker := newAnomalyTestWorker()
			api := &fixedAnomalyPromAPI{value: model.Vector{{
				Metric: model.Metric{"host": "node"}, Timestamp: 1700000000000, Value: 9,
			}}}
			query := models.PromQuery{
				PromQl: `cpu{host="$host"} > $limit`, Severity: 2,
				VarConfig: models.VarConfig{ParamVal: []models.ParamQueryForFirst{
					{Name: "host", ParamType: "enum", Query: []string{"node"}},
					{Name: "limit", ParamType: "threshold", Query: tc.threshold},
				}},
			}

			points, err := worker.VarFillingAfterQuery(query, api)

			require.NoError(t, err)
			require.Equal(t, []string{`cpu{} > 7.5`}, api.queries)
			require.Len(t, points, 1)
			require.Equal(t, `cpu{host="node"} > 7.5`, points[0].Query)
			require.Equal(t, 9.0, points[0].Value)
			require.Equal(t, 2, points[0].Severity)
			require.Equal(t, int64(1700000000), points[0].Timestamp)
		})
	}
}

func TestVarFillingBeforeQuery_SubstitutesThresholdValues(t *testing.T) {
	worker := newAnomalyTestWorker()
	api := &fixedAnomalyPromAPI{value: model.Vector{{
		Metric: model.Metric{"host": "node"}, Timestamp: 1700000000000, Value: 9,
	}}}
	query := models.PromQuery{
		PromQl: `sum(cpu{host="$host"}) > $limit`, Severity: 2,
		VarConfig: models.VarConfig{ParamVal: []models.ParamQueryForFirst{
			{Name: "host", ParamType: "enum", Query: []string{"node"}},
			{Name: "limit", ParamType: "threshold", Query: float64(7.5)},
		}},
	}

	points, err := worker.VarFillingBeforeQuery(query, api)

	require.NoError(t, err)
	require.Equal(t, []string{`sum(cpu{host="node"}) > 7.5`}, api.queries)
	require.Len(t, points, 1)
	require.Equal(t, `sum(cpu{host="node"}) > 7.5`, points[0].Query)
	require.Equal(t, 9.0, points[0].Value)
	require.Equal(t, 2, points[0].Severity)
}

func TestGetParamPermutation_RejectsMissingParameterDefinition(t *testing.T) {
	worker := newAnomalyTestWorker()

	combinations, err := worker.getParamPermutation(nil, []string{"host"}, nil, "", nil)

	require.EqualError(t, err, "param key not found: host")
	require.Nil(t, combinations)
}

func TestGetParamPermutation_RejectsUnknownParameterType(t *testing.T) {
	worker := newAnomalyTestWorker()
	parameters := map[string]models.ParamQuery{"host": {ParamType: "unsupported"}}

	combinations, err := worker.getParamPermutation(parameters, []string{"host"}, nil, "", nil)

	require.EqualError(t, err, "unknown param type: unsupported")
	require.Nil(t, combinations)
}

func TestVarFillingAfterQuery_InvalidParameterNodeDoesNotProducePoints(t *testing.T) {
	worker := newAnomalyTestWorker()
	api := &fixedAnomalyPromAPI{value: model.Vector{{Metric: model.Metric{"host": "node"}, Value: 9}}}
	query := models.PromQuery{
		PromQl:    `cpu{host="$host"} > 5`,
		VarConfig: models.VarConfig{ParamVal: []models.ParamQueryForFirst{{Name: "host", ParamType: "unsupported"}}},
	}

	points, err := worker.VarFillingAfterQuery(query, api)

	require.NoError(t, err)
	require.Empty(t, points)
	require.Equal(t, []string{`cpu{} > 5`}, api.queries)
}

func TestVarFillingBeforeQuery_InvalidParameterNodeDoesNotQuery(t *testing.T) {
	worker := newAnomalyTestWorker()
	api := &fixedAnomalyPromAPI{}
	query := models.PromQuery{
		PromQl:    `sum(cpu{host="$host"}) > 5`,
		VarConfig: models.VarConfig{ParamVal: []models.ParamQueryForFirst{{Name: "host", ParamType: "unsupported"}}},
	}

	points, err := worker.VarFillingBeforeQuery(query, api)

	require.NoError(t, err)
	require.Empty(t, points)
	require.Empty(t, api.queries)
}

func TestProcessJoins_RejectsMoreJoinsThanQueries(t *testing.T) {
	indexes := map[string]map[uint64][]uint64{"A": {1: {1}}}
	trigger := models.Trigger{JoinRef: "A", Joins: []models.Join{{Ref: "B", JoinType: "inner_join"}}}

	joined := ProcessJoins(1, trigger, indexes, nil)

	require.Nil(t, joined)
	require.Equal(t, map[string]map[uint64][]uint64{"A": {1: {1}}}, indexes)
}

func TestProcessJoins_NoneKeepsQueriesAsIndependentSeries(t *testing.T) {
	indexes := map[string]map[uint64][]uint64{"A": {1: {1}}, "B": {2: {2}}}
	trigger := models.Trigger{JoinRef: "A", Joins: []models.Join{{Ref: "B", JoinType: "none"}}}
	store := map[uint64]models.DataResp{1: {Ref: "A"}, 2: {Ref: "B"}}

	joined := ProcessJoins(1, trigger, indexes, store)

	groups := make([][]uint64, 0, len(joined))
	for _, ids := range joined {
		groups = append(groups, ids)
	}
	require.ElementsMatch(t, [][]uint64{{1}, {2}}, groups)
}

func TestGetQueryRefAndUnit_RejectsUnserializableQuery(t *testing.T) {
	query := map[string]interface{}{"ref": "A", "unit": make(chan int)}

	ref, unit, err := GetQueryRefAndUnit(query)

	require.ErrorContains(t, err, "unsupported type: chan int")
	require.Empty(t, ref)
	require.Empty(t, unit)
}

func TestVarFillingBeforeQuery_EmptyResultProducesNoAnomaly(t *testing.T) {
	worker := newAnomalyTestWorker()
	api := &fixedAnomalyPromAPI{value: model.Vector{}}
	query := models.PromQuery{
		PromQl: `sum(cpu{host="$host"}) > 5`, Severity: 2,
		VarConfig: models.VarConfig{ParamVal: []models.ParamQueryForFirst{{
			Name: "host", ParamType: "enum", Query: []string{"node"},
		}}},
	}

	points, err := worker.VarFillingBeforeQuery(query, api)

	require.NoError(t, err)
	require.Empty(t, points)
	require.Equal(t, []string{`sum(cpu{host="node"}) > 5`}, api.queries)
}

func TestRehashSet_IgnoresEmptyAndMissingSeriesGroups(t *testing.T) {
	indexes := map[uint64][]uint64{1: {}, 2: {99}, 3: {3, 4}}
	store := map[uint64]models.DataResp{3: {Ref: "A", Metric: model.Metric{"host": "node"}}}

	rehashed := rehashSet(indexes, store, []string{"host"})

	require.Len(t, rehashed, 1)
	for _, groups := range rehashed {
		require.Equal(t, [][]uint64{{3, 4}}, groups)
	}
}

func TestProcessJoins_OriginalMergesMatchingLabelGroups(t *testing.T) {
	indexes := map[string]map[uint64][]uint64{"A": {1: {10}}, "B": {1: {20}, 2: {30}}}
	trigger := models.Trigger{JoinRef: "A", Joins: []models.Join{{Ref: "B", JoinType: "original"}}}
	store := map[uint64]models.DataResp{10: {Ref: "A"}, 20: {Ref: "B"}, 30: {Ref: "B"}}

	joined := ProcessJoins(1, trigger, indexes, store)

	require.Equal(t, map[uint64][]uint64{1: {10, 20}, 2: {30}}, joined)
	// Joining must allocate the combined slices rather than change a cached input group.
	require.Equal(t, []uint64{10}, indexes["A"][1])
}

func TestProcessJoins_EmptyQueriesProduceNoSeriesGroups(t *testing.T) {
	joined := ProcessJoins(1, models.Trigger{}, nil, nil)

	require.NotNil(t, joined)
	require.Empty(t, joined)
}
