package process

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/alert/dispatch"
	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/evallog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"github.com/toolkits/pkg/container/list"
)

func round6Processor(t *testing.T) *Processor {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "n9e-process-round6-")
	require.NoError(t, err)
	require.False(t, evallog.Enabled())
	t.Cleanup(func() {
		evallog.Shutdown()
		require.NoError(t, os.RemoveAll(dir))
	})
	require.NoError(t, evallog.Init(evallog.Config{Dir: dir}, evallog.Hooks{}))
	p := stateProcessor(t, &models.AlertRule{Id: 61, GroupId: 7, Name: "CPU", Annotations: "{}"})
	p.datasourceId = 62
	p.ctx = &ctx.Context{IsCenter: true}
	p.TargetCache = &memsto.TargetCacheType{}
	p.BusiGroupCache = &memsto.BusiGroupCacheType{}
	p.BusiGroupCache.Set(map[int64]*models.BusiGroup{7: {Id: 7, Name: "ops"}}, 1, 1)
	p.datasourceCache = &memsto.DatasourceCacheType{}
	p.alertRuleCache = &memsto.AlertRuleCacheType{}
	p.alertRuleCache.Set(map[int64]*models.AlertRule{61: p.rule}, 1, 1)
	p.Stats = &astats.Stats{
		CounterRuleEvalErrorTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "round6_process_errors"}, []string{"datasource", "stage", "group", "rule"}),
		CounterMuteTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "round6_process_mutes"}, []string{"group", "rule", "mute", "datasource"}),
	}
	p.EvalRec = &evallog.EvalRecord{}
	return p
}

func TestHandle_RefreshesNotificationConfigurationBeforeFiring(t *testing.T) {
	p := round6Processor(t)
	originalHash := p.rule.Hash()
	updated := *p.rule
	updated.Name = "CPU renamed"
	updated.NotifyRuleIds = []int64{81}
	p.alertRuleCache.Set(map[int64]*models.AlertRule{61: &updated}, 1, 2)
	point := models.AnomalyPoint{Labels: model.Metric{"host": "node"}, Timestamp: 100, Value: 95, Severity: 2}

	p.Handle([]models.AnomalyPoint{point}, "external", false)

	items := queue.EventQueue.PopBackBy(10)
	require.Len(t, items, 1)
	event := items[0].(*models.AlertCurEvent)
	require.Equal(t, []int64{81}, event.NotifyRuleIds)
	require.Equal(t, "CPU renamed", event.TagsMap["rulename"])
	require.Equal(t, originalHash, event.RuleHash)
	require.Equal(t, int64(100), event.LastEvalTime)
	require.Equal(t, 1, p.EvalRec.Fired)
	require.Equal(t, evallog.StageFired, p.EvalRec.Events[0].Stage)
}

func TestHandle_DeletedRuleLeavesActiveEventsUntouched(t *testing.T) {
	p := round6Processor(t)
	p.alertRuleCache.Set(nil, 0, 2)
	active := &models.AlertCurEvent{Hash: "active", LastEvalTime: 100}
	p.fires.Set(active.Hash, active)

	p.Handle(nil, "inner", false)

	require.Zero(t, queue.EventQueue.Len())
	stored, exists := p.fires.Get(active.Hash)
	require.True(t, exists)
	require.Same(t, active, stored)
	require.Equal(t, int64(100), stored.LastEvalTime)
	counter := p.Stats.CounterRuleEvalErrorTotal.WithLabelValues("62", "handle_event", "ops", "61")
	require.Equal(t, 1.0, testutil.ToFloat64(counter))
}

func TestHandle_FullyMutedPointKeepsExistingFireActive(t *testing.T) {
	p := round6Processor(t)
	p.rule.Disabled = 1
	point := models.AnomalyPoint{Labels: model.Metric{"host": "node"}, Timestamp: 100, Severity: 2}
	hash := Hash(p.rule.Id, p.datasourceId, point)
	p.fires.Set(hash, &models.AlertCurEvent{Hash: hash, LastEvalTime: 90})

	p.Handle([]models.AnomalyPoint{point}, "inner", false)

	require.Zero(t, queue.EventQueue.Len())
	_, exists := p.fires.Get(hash)
	require.True(t, exists)
	require.Equal(t, 1, p.EvalRec.Muted)
	require.Equal(t, evallog.StageMuted, p.EvalRec.Events[0].Stage)
	counter := p.Stats.CounterMuteTotal.WithLabelValues("ops", "61", "0", "62")
	require.Equal(t, 1.0, testutil.ToFloat64(counter))
}

func TestHandle_NotifyOnlyMuteQueuesMarkedSnapshot(t *testing.T) {
	p := round6Processor(t)
	p.alertMuteCache.Set(map[int64][]*models.AlertMute{7: {{
		Id: 71, MuteType: models.MuteTypeNotifyOnly, MuteTimeType: models.TimeRange, Btime: 1, Etime: 200,
	}}}, 1, 1)
	point := models.AnomalyPoint{Labels: model.Metric{"host": "node"}, Timestamp: 100, Severity: 2}

	p.Handle([]models.AnomalyPoint{point}, "external", false)

	items := queue.EventQueue.PopBackBy(10)
	require.Len(t, items, 1)
	event := items[0].(*models.AlertCurEvent)
	require.Equal(t, 1, event.NotifyMuted)
	require.Equal(t, int64(71), event.MuteId)
	require.Zero(t, event.NotifyCurNumber)
	require.Equal(t, 1, event.MutedPersistNumber)
	require.Zero(t, event.LastSentTime)
	require.Equal(t, evallog.StageMutedNotifyOnly, p.EvalRec.Events[0].Stage)
}

func TestHandle_MuteHookReceivesEnrichedEventAndPreventsQueueing(t *testing.T) {
	p := round6Processor(t)
	old := dispatch.EventMuteHook
	t.Cleanup(func() { dispatch.EventMuteHook = old })
	var seen *models.AlertCurEvent
	dispatch.EventMuteHook = func(event *models.AlertCurEvent) bool {
		seen = event.DeepCopy()
		return true
	}

	p.Handle([]models.AnomalyPoint{{Labels: model.Metric{"host": "node"}, Severity: 2}}, "external", false)

	require.NotNil(t, seen)
	require.Equal(t, "ops", seen.GroupName)
	require.Equal(t, "node", seen.TagsMap["host"])
	require.Zero(t, queue.EventQueue.Len())
	require.Empty(t, p.fires.Keys())
	require.Equal(t, evallog.StageMutedByHook, p.EvalRec.Events[0].Stage)
}

func TestHandle_ExternalBatchDoesNotRecoverUnmentionedEvents(t *testing.T) {
	p := round6Processor(t)
	p.fires.Set("active", &models.AlertCurEvent{Hash: "active"})

	p.Handle(nil, "external", false)

	require.Equal(t, []string{"active"}, p.fires.Keys())
	require.Zero(t, queue.EventQueue.Len())
}

func TestPushEventToQueue_FullQueueRetainsFireAndRecordsFailure(t *testing.T) {
	p := round6Processor(t)
	old := queue.EventQueue
	queue.EventQueue = list.NewSafeListLimited(1)
	t.Cleanup(func() { queue.EventQueue = old })
	require.True(t, queue.EventQueue.PushFront("occupied"))
	event := &models.AlertCurEvent{Hash: "new", LastEvalTime: 100}

	p.pushEventToQueue(event)

	require.Equal(t, []interface{}{"occupied"}, queue.EventQueue.PopBackBy(10))
	stored, exists := p.fires.Get("new")
	require.True(t, exists)
	require.Same(t, event, stored)
	require.Equal(t, evallog.StagePushQueueFailed, p.EvalRec.Events[0].Stage)
	counter := p.Stats.CounterRuleEvalErrorTotal.WithLabelValues("62", "push_event_queue", "ops", "61")
	require.Equal(t, 1.0, testutil.ToFloat64(counter))
}

func TestBuildEvent_NoDataAnnotatesRuleConfiguration(t *testing.T) {
	p := round6Processor(t)
	p.rule.RuleConfig = `{"inhibit":true}`
	p.rule.Annotations = "invalid JSON"

	event := p.BuildEvent(models.AnomalyPoint{TriggerType: models.TriggerTypeNodata}, "inner", 100, "hash")

	require.Equal(t, "nodata", event.TriggerValue)
	var config models.RuleQuery
	require.NoError(t, json.Unmarshal([]byte(event.RuleConfig), &config))
	require.True(t, config.Inhibit)
	require.Equal(t, models.TriggerTypeNodata, config.TriggerType)
	require.Empty(t, event.AnnotationsJSON)
}

func TestBuildEvent_MultipleValuesPreserveEachExpressionResult(t *testing.T) {
	p := round6Processor(t)

	event := p.BuildEvent(models.AnomalyPoint{Values: "$A:10 $B:20", Value: 10}, "inner", 100, "hash")

	require.Equal(t, "$A:10 $B:20", event.TriggerValue)
	require.Equal(t, "$A:10 $B:20", event.TriggerValues)
}

func TestRecoverSingle_ExplicitValuesReplaceFiringValues(t *testing.T) {
	p := round6Processor(t)
	p.fires.Set("active", &models.AlertCurEvent{
		Hash: "active", TriggerValue: "95", TriggerValues: "$A:95", NotifyMuted: 1, MuteId: 71,
		RecoverConfig: models.RecoverConfig{JudgeType: models.RecoverOnCondition},
	})
	value := "75"

	p.RecoverSingle(true, "active", 200, &value, "$A:75")

	items := queue.EventQueue.PopBackBy(10)
	require.Len(t, items, 1)
	event := items[0].(*models.AlertCurEvent)
	require.True(t, event.IsRecovered)
	require.Equal(t, "75", event.TriggerValue)
	require.Equal(t, "$A:75", event.TriggerValues)
	require.Zero(t, event.NotifyMuted)
	require.Zero(t, event.MuteId)
	require.Empty(t, p.fires.Keys())
}
