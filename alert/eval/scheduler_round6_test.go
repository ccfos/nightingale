package eval

import (
	"testing"

	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func round6ExternalScheduler(t *testing.T) *Scheduler {
	t.Helper()
	worker := eventWorker(t, "external-round6")
	rule := worker.Rule
	rule.NotifyRuleIds = []int64{88}
	rule.DatasourceQueries = []models.DatasourceQuery{{MatchType: 0, Op: "in", Values: []interface{}{int64(0)}}}
	rules := &memsto.AlertRuleCacheType{}
	rules.Set(map[int64]*models.AlertRule{rule.Id: rule}, 1, 1)
	datasources := &memsto.DatasourceCacheType{}
	datasources.Set(map[int64]*models.Datasource{
		7301: {Id: 7301, PluginType: rule.Cate, Status: "enabled"},
		7303: {Id: 7303, PluginType: rule.Cate, Status: "disabled"},
	}, 2, 1)
	return &Scheduler{
		alertRules: make(map[string]*AlertRuleWorker), ExternalProcessors: process.NewExternalProcessors(),
		alertRuleCache: rules, datasourceCache: datasources,
		alertMuteCache: &memsto.AlertMuteCacheType{},
		targetCache:    &memsto.TargetCacheType{}, busiGroupCache: &memsto.BusiGroupCacheType{},
		ctx: worker.Ctx, stats: worker.Processor.Stats,
	}
}

func TestSyncAlertRules_ExternalRulesUseOnlyEnabledDatasources(t *testing.T) {
	scheduler := round6ExternalScheduler(t)

	scheduler.syncAlertRules()

	require.Len(t, scheduler.ExternalProcessors.Processors, 1)
	processor, exists := scheduler.ExternalProcessors.GetExternalAlertRule(7301, 7302)
	require.True(t, exists)
	require.Equal(t, int64(7301), processor.DatasourceId())
	_, exists = scheduler.ExternalProcessors.GetExternalAlertRule(7303, 7302)
	require.False(t, exists)
	require.Empty(t, scheduler.alertRules)

}

func TestSyncAlertRules_UnchangedExternalRuleRetainsProcessor(t *testing.T) {
	scheduler := round6ExternalScheduler(t)
	scheduler.syncAlertRules()
	original, exists := scheduler.ExternalProcessors.GetExternalAlertRule(7301, 7302)
	require.True(t, exists)

	scheduler.syncAlertRules()

	current, exists := scheduler.ExternalProcessors.GetExternalAlertRule(7301, 7302)
	require.True(t, exists)
	require.Same(t, original, current)
}

func TestSyncAlertRules_ChangedExternalConfigurationReplacesProcessor(t *testing.T) {
	scheduler := round6ExternalScheduler(t)
	scheduler.syncAlertRules()
	original, exists := scheduler.ExternalProcessors.GetExternalAlertRule(7301, 7302)
	require.True(t, exists)
	changed := *scheduler.alertRuleCache.Get(7302)
	changed.RuleConfig = `{"source":"changed"}`
	scheduler.alertRuleCache.Set(map[int64]*models.AlertRule{7302: &changed}, 1, 2)

	scheduler.syncAlertRules()

	current, exists := scheduler.ExternalProcessors.GetExternalAlertRule(7301, 7302)
	require.True(t, exists)
	require.NotSame(t, original, current)
	require.NotEqual(t, original.Hash(), current.Hash())
}

func TestSyncAlertRules_DeletedRulesRemoveExternalProcessorsAndStopWorkers(t *testing.T) {
	scheduler := round6ExternalScheduler(t)
	scheduler.syncAlertRules()
	stale := eventWorker(t, "stale-round6")
	scheduler.alertRules[stale.Hash()] = stale
	scheduler.alertRuleCache.Set(nil, 0, 2)

	scheduler.syncAlertRules()

	require.Empty(t, scheduler.ExternalProcessors.Processors)
	require.Empty(t, scheduler.alertRules)
	select {
	case <-stale.Quit:
	default:
		t.Fatal("the deleted worker was not stopped")
	}
}

func TestSyncAlertRules_ExternalProcessorRestoresPersistedFires(t *testing.T) {
	scheduler := round6ExternalScheduler(t)
	require.NoError(t, scheduler.ctx.DB.Create(&models.AlertCurEvent{
		Id: 1, Hash: "active", RuleId: 7302, DatasourceId: 7301, Tags: "host=node", NotifyRuleIds: []int64{99},
	}).Error)

	scheduler.syncAlertRules()

	processor, exists := scheduler.ExternalProcessors.GetExternalAlertRule(7301, 7302)
	require.True(t, exists)
	processor.Handle(nil, "inner", false)
	items := queue.EventQueue.PopBackBy(10)
	require.Len(t, items, 1)
	recovered := items[0].(*models.AlertCurEvent)
	require.True(t, recovered.IsRecovered)
	require.Equal(t, "active", recovered.Hash)
	require.Equal(t, "node", recovered.TagsMap["host"])
	require.Equal(t, []int64{88}, recovered.NotifyRuleIds)
}
