package router

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"
	pkgprom "github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func fireRound7Rule(t *testing.T, config interface{}) *models.AlertRule {
	t.Helper()
	data, err := json.Marshal(config)
	require.NoError(t, err)
	return &models.AlertRule{Id: 71, Cate: "prometheus", RuleConfig: string(data), Delay: 15}
}

func fireRound7Prom(client pkgprom.API) *Router {
	return &Router{PromClients: &prom.PromClientMap{ReaderClients: map[int64]pkgprom.API{77: client}}}
}

func TestCheckTestFirePromQueries_ReturnsFirstSampleAndWarnsOnEmptyResult(t *testing.T) {
	var queries []string
	var contexts []context.Context
	fake := &batchRound6PromAPI{query: func(c context.Context, query string, _ time.Time) (model.Value, pkgprom.Warnings, error) {
		queries = append(queries, query)
		contexts = append(contexts, c)
		if query == "empty" {
			return model.Vector{}, nil, nil
		}
		return model.Vector{{Metric: model.Metric{"host": "web-1"}, Value: 85}}, nil, nil
	}}
	rt := fireRound7Prom(fake)
	rule := fireRound7Rule(t, map[string]interface{}{"queries": []interface{}{
		map[string]interface{}{"prom_ql": " up "},
		map[string]interface{}{"prom_ql": "empty"},
	}})

	stage, sample := rt.checkTestFirePromQueries(queryRound6Context(), rule, 77, true)

	require.Equal(t, "query_check", stage.Stage)
	require.Equal(t, testFireStageWarn, stage.Status)
	require.Equal(t, []string{"up", "empty"}, queries)
	require.Equal(t, []gin.H{
		{"promql": "up", "ok": true, "series_count": 1, "latest_value": float64(85)},
		{"promql": "empty", "ok": true, "series_count": 0},
	}, stage.Data["queries"])
	require.Equal(t, &testFireAutoSample{labels: map[string]string{"host": "web-1"}, value: 85}, sample)
	for _, c := range contexts {
		_, ok := c.Deadline()
		require.True(t, ok)
		require.ErrorIs(t, c.Err(), context.Canceled)
	}
}

func TestCheckTestFirePromQueries_RedactsUpstreamCredentials(t *testing.T) {
	fake := &batchRound6PromAPI{query: func(context.Context, string, time.Time) (model.Value, pkgprom.Warnings, error) {
		return nil, nil, errors.New("query failed token=topsecret password=private")
	}}
	rule := fireRound7Rule(t, map[string]interface{}{"queries": []interface{}{map[string]interface{}{"prom_ql": "up"}}})

	stage, sample := fireRound7Prom(fake).checkTestFirePromQueries(queryRound6Context(), rule, 77, true)

	require.Equal(t, testFireStageFail, stage.Status)
	require.Nil(t, sample)
	items := stage.Data["queries"].([]gin.H)
	require.Len(t, items, 1)
	require.Equal(t, false, items[0]["ok"])
	require.Equal(t, "(details hidden for security)", items[0]["error"])
	require.NotContains(t, items[0]["error"], "topsecret")
	require.NotContains(t, items[0]["error"], "private")
}

func TestCheckTestFirePromQueries_PermissionDenialPreventsQuery(t *testing.T) {
	previous := CheckDsPerm
	t.Cleanup(func() { CheckDsPerm = previous })
	checks := 0
	CheckDsPerm = func(_ *gin.Context, id int64, cate string, _ interface{}) bool {
		checks++
		require.Equal(t, int64(77), id)
		require.Equal(t, "prometheus", cate)
		return false
	}
	calls := 0
	fake := &batchRound6PromAPI{query: func(context.Context, string, time.Time) (model.Value, pkgprom.Warnings, error) {
		calls++
		return model.Vector{}, nil, nil
	}}
	rule := fireRound7Rule(t, map[string]interface{}{"queries": []interface{}{map[string]interface{}{"prom_ql": "up"}}})

	stage, sample := fireRound7Prom(fake).checkTestFirePromQueries(queryRound6Context(), rule, 77, true)

	require.Equal(t, testFireStageFail, stage.Status)
	require.Equal(t, 1, checks)
	require.Zero(t, calls)
	require.Nil(t, sample)
	require.Equal(t, []gin.H{{"promql": "up", "ok": false, "error": testFireDsPermDenied}}, stage.Data["queries"])
}

func TestCheckTestFirePromQueries_LimitsUpstreamRequests(t *testing.T) {
	calls := 0
	fake := &batchRound6PromAPI{query: func(context.Context, string, time.Time) (model.Value, pkgprom.Warnings, error) {
		calls++
		return &model.Scalar{Value: 1}, nil, nil
	}}
	queries := make([]interface{}, testFireMaxCheckQueries+1)
	for i := range queries {
		queries[i] = map[string]interface{}{"prom_ql": "up"}
	}
	rule := fireRound7Rule(t, map[string]interface{}{"queries": queries})

	stage, sample := fireRound7Prom(fake).checkTestFirePromQueries(queryRound6Context(), rule, 77, true)

	require.Equal(t, testFireStageWarn, stage.Status)
	require.Equal(t, testFireMaxCheckQueries, calls)
	items := stage.Data["queries"].([]gin.H)
	require.Len(t, items, testFireMaxCheckQueries+1)
	require.Equal(t, gin.H{"skipped": true, "reason": "too_many_queries"}, items[len(items)-1])
	require.Equal(t, float64(1), sample.value)
}

func TestCheckTestFirePromV2Queries_EvaluatesTriggersUsingReturnedSeries(t *testing.T) {
	var queries []string
	fake := &batchRound6PromAPI{query: func(_ context.Context, query string, _ time.Time) (model.Value, pkgprom.Warnings, error) {
		queries = append(queries, query)
		return model.Vector{{Metric: model.Metric{"host": "web-1"}, Timestamp: 100000, Value: 85}}, nil, nil
	}}
	rule := fireRound7Rule(t, models.RuleQuery{
		Version: "v2", Queries: []interface{}{map[string]interface{}{"ref": "A", "query": "up"}},
		Triggers: []models.Trigger{{Exp: "$A > 80", Severity: 2}},
	})

	stage := fireRound7Prom(fake).checkTestFirePromV2Queries(queryRound6Context(), rule, 77)

	require.Equal(t, testFireStagePass, stage.Status)
	require.Equal(t, []string{"up"}, queries)
	require.Equal(t, []gin.H{{"ref": "A", "promql": "up", "ok": true, "series_count": 1, "latest_value": float64(85)}},
		stage.Data["queries"])
	triggers := stage.Data["triggers"].([]gin.H)
	require.Len(t, triggers, 1)
	require.Equal(t, 1, triggers[0]["fired_groups"])
	require.Equal(t, true, triggers[0]["ok"])
}

func TestCheckTestFirePromV2Queries_EmptyResultWarnsWithoutFiring(t *testing.T) {
	fake := &batchRound6PromAPI{query: func(context.Context, string, time.Time) (model.Value, pkgprom.Warnings, error) {
		return model.Vector{}, nil, nil
	}}
	rule := fireRound7Rule(t, models.RuleQuery{
		Version: "v2", Queries: []interface{}{map[string]interface{}{"ref": "A", "query": "up"}},
		Triggers: []models.Trigger{{Exp: "$A > 80"}},
	})

	stage := fireRound7Prom(fake).checkTestFirePromV2Queries(queryRound6Context(), rule, 77)

	require.Equal(t, testFireStageWarn, stage.Status)
	require.Equal(t, []gin.H{{"ref": "A", "promql": "up", "ok": true, "series_count": 0}}, stage.Data["queries"])
	triggers := stage.Data["triggers"].([]gin.H)
	require.Len(t, triggers, 1)
	require.Equal(t, true, triggers[0]["no_data"])
}

func TestCheckTestFirePromV2Queries_RedactsUpstreamFailure(t *testing.T) {
	fake := &batchRound6PromAPI{query: func(context.Context, string, time.Time) (model.Value, pkgprom.Warnings, error) {
		return nil, nil, errors.New("failed token=private")
	}}
	rule := fireRound7Rule(t, models.RuleQuery{Version: "v2",
		Queries: []interface{}{map[string]interface{}{"ref": "A", "query": "up"}}})

	stage := fireRound7Prom(fake).checkTestFirePromV2Queries(queryRound6Context(), rule, 77)

	require.Equal(t, testFireStageFail, stage.Status)
	items := stage.Data["queries"].([]gin.H)
	require.Len(t, items, 1)
	require.Equal(t, false, items[0]["ok"])
	require.Equal(t, "(details hidden for security)", items[0]["error"])
	require.NotContains(t, items[0]["error"], "private")
}

func TestCheckTestFirePromV2Queries_DenialPreventsQueryAndPreservesTriggerReport(t *testing.T) {
	previous := CheckDsPerm
	t.Cleanup(func() { CheckDsPerm = previous })
	CheckDsPerm = func(*gin.Context, int64, string, interface{}) bool { return false }
	rule := fireRound7Rule(t, models.RuleQuery{Version: "v2",
		Queries:  []interface{}{map[string]interface{}{"ref": "A", "query": "up"}},
		Triggers: []models.Trigger{{Exp: "$A > 80"}}})

	stage := (&Router{}).checkTestFirePromV2Queries(queryRound6Context(), rule, 77)

	require.Equal(t, testFireStageFail, stage.Status)
	require.Equal(t, []gin.H{{"ref": "A", "promql": "up", "ok": false, "error": testFireDsPermDenied}}, stage.Data["queries"])
	require.Len(t, stage.Data["triggers"], 1)
}

func TestCheckTestFireGenericQueries_RendersSQLAndEvaluatesReturnedSeries(t *testing.T) {
	calls := 0
	var queryContext context.Context
	fake := &queryRound6Datasource{data: func(c context.Context, q interface{}) ([]models.DataResp, error) {
		calls++
		queryContext = c
		require.Equal(t, "select 7", q.(map[string]interface{})["sql"])
		require.Equal(t, int64(15), c.Value("delay"))
		cc, ok := types.CallContextFromCtx(c)
		require.True(t, ok)
		require.Equal(t, int64(770007), cc.DatasourceID)
		require.Equal(t, int64(71), cc.RuleID)
		require.Equal(t, "alert_rule_test_fire", cc.Operator)
		return []models.DataResp{{Ref: "A", Metric: model.Metric{"host": "web-1"}, Values: [][]float64{{100, 85}}}}, nil
	}}
	putRound6Datasource(t, "mysql", 770007, fake)
	rule := fireRound7Rule(t, models.RuleQuery{
		Queries:  []interface{}{map[string]interface{}{"ref": "A", "sql": "select {{add 3 4}}"}},
		Triggers: []models.Trigger{{Exp: "$A > 80", Severity: 2}},
	})
	rule.Cate = "mysql"

	stage, sample := (&Router{}).checkTestFireGenericQueries(queryRound6Context(), rule, 770007)

	require.Equal(t, testFireStagePass, stage.Status)
	require.Equal(t, 1, calls)
	require.ErrorIs(t, queryContext.Err(), context.Canceled)
	require.Equal(t, []gin.H{{"ref": "A", "ok": true, "series_count": 1, "latest_value": float64(85)}}, stage.Data["queries"])
	triggers := stage.Data["triggers"].([]gin.H)
	require.Len(t, triggers, 1)
	require.Equal(t, 1, triggers[0]["fired_groups"])
	require.NotNil(t, sample)
	require.Equal(t, map[string]string{"host": "web-1"}, sample.labels)
	require.Equal(t, float64(85), sample.value)
}

func TestCheckTestFireGenericQueries_TemplateErrorPreventsDatasourceCall(t *testing.T) {
	calls := 0
	fake := &queryRound6Datasource{data: func(context.Context, interface{}) ([]models.DataResp, error) {
		calls++
		return nil, nil
	}}
	putRound6Datasource(t, "mysql", 770007, fake)
	rule := fireRound7Rule(t, models.RuleQuery{
		Queries: []interface{}{map[string]interface{}{"ref": "A", "sql": "select {{$undefined}}"}},
	})
	rule.Cate = "mysql"

	stage, sample := (&Router{}).checkTestFireGenericQueries(queryRound6Context(), rule, 770007)

	require.Equal(t, testFireStageFail, stage.Status)
	require.Zero(t, calls)
	require.Nil(t, sample)
	items := stage.Data["queries"].([]gin.H)
	require.Len(t, items, 1)
	require.Equal(t, false, items[0]["ok"])
	require.Contains(t, items[0]["error"], "query tmpl parse error")
}

func TestCheckTestFireGenericQueries_UpstreamFailureIsRedacted(t *testing.T) {
	fake := &queryRound6Datasource{data: func(context.Context, interface{}) ([]models.DataResp, error) {
		return nil, errors.New("query failed password=private")
	}}
	putRound6Datasource(t, "mysql", 770007, fake)
	rule := fireRound7Rule(t, models.RuleQuery{
		Queries: []interface{}{map[string]interface{}{"ref": "A", "sql": "select 1"}},
	})
	rule.Cate = "mysql"

	stage, sample := (&Router{}).checkTestFireGenericQueries(queryRound6Context(), rule, 770007)

	require.Equal(t, testFireStageFail, stage.Status)
	require.Nil(t, sample)
	items := stage.Data["queries"].([]gin.H)
	require.Len(t, items, 1)
	require.Equal(t, false, items[0]["ok"])
	require.Equal(t, "(details hidden for security)", items[0]["error"])
	require.NotContains(t, items[0]["error"], "private")
}

func TestCheckTestFireGenericQueries_EmptyResultWarns(t *testing.T) {
	fake := &queryRound6Datasource{data: func(context.Context, interface{}) ([]models.DataResp, error) { return nil, nil }}
	putRound6Datasource(t, "mysql", 770007, fake)
	rule := fireRound7Rule(t, models.RuleQuery{
		Queries: []interface{}{map[string]interface{}{"ref": "A", "sql": "select 1"}},
	})
	rule.Cate = "mysql"

	stage, sample := (&Router{}).checkTestFireGenericQueries(queryRound6Context(), rule, 770007)

	require.Equal(t, testFireStageWarn, stage.Status)
	require.Nil(t, sample)
	require.Equal(t, []gin.H{{"ref": "A", "ok": true, "series_count": 0}}, stage.Data["queries"])
}
