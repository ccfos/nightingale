package process

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// TestRecoverAlertCurEventFromDbRestoresRecoverConfig covers a restart: events
// reloaded from the db must get back their trigger's recover config, so an
// event whose recover condition is not met stays firing when it is missing
// from the next eval round instead of being recovered as models.Origin.
func TestRecoverAlertCurEventFromDbRestoresRecoverConfig(t *testing.T) {
	ruleConfig := `{"queries":[{"prom_ql":"cpu_usage","ref":"A"}],"triggers":[
		{"exp":"$A > 90","severity":1,"recover_config":{"judge_type":1}},
		{"exp":"$A > 80","severity":2,"recover_config":{"judge_type":2,"recover_exp":"$A < 70"}},
		{"exp":"$A > 70","severity":3}]}`
	var ruleQuery models.RuleQuery
	require.NoError(t, json.Unmarshal([]byte(ruleConfig), &ruleQuery))
	promQl := func(queries []interface{}, trigger models.Trigger) string {
		// the anomaly point's Query, built as in eval
		return fmt.Sprintf("query:%+v trigger:%+v", queries, trigger)
	}
	editedTrigger := ruleQuery.Triggers[1]
	editedTrigger.Exp = "$A > 85"
	editedQueries := []interface{}{map[string]interface{}{"prom_ql": "cpu_usage_old", "ref": "A"}}

	events := []*models.AlertCurEvent{
		{Hash: "nodata", RuleId: 1, PromQl: promQl(ruleQuery.Queries, ruleQuery.Triggers[0])},
		{Hash: "condition", RuleId: 1, PromQl: promQl(ruleQuery.Queries, ruleQuery.Triggers[1])},
		{Hash: "origin", RuleId: 1, PromQl: promQl(ruleQuery.Queries, ruleQuery.Triggers[2])},
		// the rule was edited after these fired, so their hash is gone
		{Hash: "edited-trigger", RuleId: 1, PromQl: promQl(ruleQuery.Queries, editedTrigger)},
		{Hash: "edited-query", RuleId: 1, PromQl: promQl(editedQueries, ruleQuery.Triggers[1])},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var dat interface{}
		switch r.URL.Path {
		case "/v1/n9e/statistic":
			dat = &models.Statistics{Total: 0, LastUpdated: 1}
		case "/v1/n9e/targets":
			dat = []*models.Target{}
		case "/v1/n9e/alert-cur-events-get-by-rid":
			dat = events
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"dat": dat, "err": ""})
	}))
	defer srv.Close()

	edgeCtx := &ctx.Context{CenterApi: conf.CenterApi{Addrs: []string{srv.URL}}}
	stats := &memsto.Stats{
		GaugeCronDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_cron_duration"}, []string{"name"}),
		GaugeSyncNumber:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_sync_number"}, []string{"name"}),
	}
	rule := &models.AlertRule{Id: 1, RuleConfig: ruleConfig}
	ruleCache := &memsto.AlertRuleCacheType{}
	ruleCache.Set(map[int64]*models.AlertRule{1: rule}, 1, 1)

	p := stateProcessor(t, rule)
	p.TargetCache = memsto.NewTargetCacheOfEngine(edgeCtx, stats, nil, "")
	p.BusiGroupCache = &memsto.BusiGroupCacheType{}
	p.alertRuleCache = ruleCache
	p.ctx = edgeCtx

	p.RecoverAlertCurEventFromDb()

	for hash, want := range map[string]models.RecoverConfig{
		"nodata":         {JudgeType: models.NotRecoverWhenNoData},
		"condition":      {JudgeType: models.RecoverOnCondition, RecoverExp: "$A < 70"},
		"origin":         {},
		"edited-trigger": {},
		"edited-query":   {},
	} {
		e, has := p.fires.Get(hash)
		require.True(t, has, hash)
		require.Equal(t, want, e.RecoverConfig, hash)
	}

	// next eval round: the query succeeds but none of the events is alerting
	p.HandleRecover(map[string]struct{}{}, 200, false)

	require.ElementsMatch(t, []string{"nodata", "condition"}, p.fires.Keys())
	var recovered []string
	for _, item := range queue.EventQueue.PopBackBy(10) {
		e := item.(*models.AlertCurEvent)
		require.True(t, e.IsRecovered)
		recovered = append(recovered, e.Hash)
	}
	require.ElementsMatch(t, []string{"origin", "edited-trigger", "edited-query"}, recovered)
}
