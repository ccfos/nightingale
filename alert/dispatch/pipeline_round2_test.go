package dispatch

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/alert/sender"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/toolkits/pkg/container/list"
	"gorm.io/gorm"
)

func round2Dispatch(t *testing.T) *Dispatch {
	t.Helper()
	oldQueue := sender.NotifyRecordQueue
	sender.NotifyRecordQueue = list.NewSafeListLimited(100)
	t.Cleanup(func() { sender.NotifyRecordQueue = oldQueue })
	users, groups := recipientCaches()
	return &Dispatch{
		ctx:       ctx.NewContext(context.Background(), nil, true),
		userCache: users, userGroupCache: groups,
		notifyRuleCache:      &memsto.NotifyRuleCacheType{},
		notifyChannelCache:   &memsto.NotifyChannelCacheType{},
		messageTemplateCache: &memsto.MessageTemplateCacheType{},
		eventProcessorCache:  &memsto.EventProcessorCacheType{},
	}
}

func TestHandleEventWithNotifyRule_SkipsMissingAndDisabledRules(t *testing.T) {
	d := round2Dispatch(t)
	d.notifyRuleCache.Set(map[int64]*models.NotifyRule{2: {ID: 2, Enable: false}}, 1, 1)
	oldSkip := ShouldSkipNotify
	t.Cleanup(func() { ShouldSkipNotify = oldSkip })
	var checked []int64
	ShouldSkipNotify = func(_ *ctx.Context, _ *models.AlertCurEvent, id int64) bool {
		checked = append(checked, id)
		return true
	}

	d.HandleEventWithNotifyRule(&models.AlertCurEvent{NotifyRuleIds: []int64{1, 2}})

	require.Empty(t, checked)
	require.Zero(t, sender.NotifyRecordQueue.Len())
}

func TestHandleEventWithNotifyRule_HonorsRuleSpecificSkipHook(t *testing.T) {
	d := round2Dispatch(t)
	d.notifyRuleCache.Set(map[int64]*models.NotifyRule{3: {ID: 3, Name: "oncall", Enable: true}}, 1, 1)
	oldSkip := ShouldSkipNotify
	t.Cleanup(func() { ShouldSkipNotify = oldSkip })
	var checked *models.AlertCurEvent
	ShouldSkipNotify = func(_ *ctx.Context, event *models.AlertCurEvent, id int64) bool {
		require.Equal(t, int64(3), id)
		checked = event
		return true
	}
	original := &models.AlertCurEvent{NotifyRuleIds: []int64{3}, TagsJSON: []string{"env=prod"}}

	d.HandleEventWithNotifyRule(original)

	require.NotNil(t, checked)
	require.NotSame(t, original, checked)
	require.Equal(t, int64(3), checked.NotifyRuleId)
	require.Equal(t, "oncall", checked.NotifyRuleName)
	require.Equal(t, map[string]string{"env": "prod"}, checked.TagsMap)
	require.Zero(t, original.NotifyRuleId)
	require.Zero(t, sender.NotifyRecordQueue.Len())
}

func TestHandleEventWithNotifyRule_SelectsMatchingChannelConfiguration(t *testing.T) {
	d := round2Dispatch(t)
	d.notifyRuleCache.Set(map[int64]*models.NotifyRule{3: {
		ID: 3, Enable: true, NotifyConfigs: []models.NotifyConfig{
			{ChannelID: 4, Severities: []int{2}},
			{ChannelID: 9, Severities: []int{1}},
		},
	}}, 1, 1)

	d.HandleEventWithNotifyRule(&models.AlertCurEvent{Id: 12, Severity: 2, NotifyRuleIds: []int64{3}})

	// Only the matching config reaches channel lookup and emits a missing-channel record.
	require.Equal(t, 1, sender.NotifyRecordQueue.Len())
	record := sender.NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, "notify_channel_id:4", record.Channel)
	require.Equal(t, int64(3), record.NotifyRuleID)
}

func TestHandleEventWithNotifyRule_RecordsMissingChannel(t *testing.T) {
	d := round2Dispatch(t)
	d.notifyRuleCache.Set(map[int64]*models.NotifyRule{3: {
		ID: 3, Enable: true, NotifyConfigs: []models.NotifyConfig{{ChannelID: 4, Severities: []int{2}}},
	}}, 1, 1)

	d.HandleEventWithNotifyRule(&models.AlertCurEvent{Id: 12, Severity: 2, NotifyRuleIds: []int64{3}})

	require.Equal(t, 1, sender.NotifyRecordQueue.Len())
	record := sender.NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, int64(12), record.EventId)
	require.Equal(t, int64(3), record.NotifyRuleID)
	require.Equal(t, "notify_channel_id:4", record.Channel)
	require.Equal(t, models.NotiStatusFailure, record.Status)
	require.Equal(t, "notify_channel not found", record.Details)
}

func TestHandleEventPipeline_SkipsDisabledMissingAndFilteredPipelines(t *testing.T) {
	cache := &memsto.EventProcessorCacheType{}
	cache.Set(map[int64]*models.EventPipeline{
		2: {ID: 2, Disabled: true},
		3: {ID: 3, FilterEnable: true, LabelFilters: []models.TagFilter{{Key: "env", Func: "==", Value: "dev"}}},
		4: {ID: 4},
	}, 3, 1)
	event := &models.AlertCurEvent{Hash: "h", TagsJSON: []string{"env=prod"}, TagsMap: map[string]string{"env": "prod"}}
	configs := []models.PipelineConfig{
		{PipelineId: 4, Enable: false},
		{PipelineId: 1, Enable: true},
		{PipelineId: 2, Enable: true},
		{PipelineId: 3, Enable: true},
		{PipelineId: 4, Enable: true},
	}

	got := HandleEventPipeline(configs, event, event, cache, nil, 1, "alert_rule")

	require.Same(t, event, got)
	require.Equal(t, "env=prod", got.Tags)
	require.Equal(t, map[string]string{"env": "prod"}, got.TagsMap)
}

func TestHandleEventPipeline_DropStopsNotificationAndRecordsReason(t *testing.T) {
	d := round2Dispatch(t)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.EventPipelineExecution{}))
	d.ctx = ctx.NewContext(context.Background(), db, true)
	d.eventProcessorCache.Set(map[int64]*models.EventPipeline{7: {
		ID: 7, Name: "drop low priority",
		ProcessorConfigs: []models.ProcessorConfig{{Typ: "event_drop", Config: map[string]interface{}{"content": "true"}}},
	}}, 1, 1)
	origin := &models.AlertCurEvent{Id: 12, Hash: "h"}

	got := HandleEventPipeline([]models.PipelineConfig{{PipelineId: 7, Enable: true}},
		origin, origin.DeepCopy(), d.eventProcessorCache, d.ctx, 3, "notify_rule")

	require.Nil(t, got)
	require.Equal(t, 1, sender.NotifyRecordQueue.Len())
	record := sender.NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, int64(12), record.EventId)
	require.Equal(t, int64(3), record.NotifyRuleID)
	require.Contains(t, record.Details, "drop by pipeline")
	require.Equal(t, "h", origin.Hash)
}

func TestSendNotifyRuleMessage_RecordsUnavailableHTTPQueueWithResolvedTarget(t *testing.T) {
	d := round2Dispatch(t)
	channel := &models.NotifyChannelConfig{
		ID: 4, Ident: "custom", Name: "HTTP", RequestType: "http",
		RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{URL: "http://127.0.0.1/unused"}},
	}
	channel.ParamConfig = &models.NotifyParamConfig{UserInfo: &models.UserInfo{ContactKey: "email"}}
	config := &models.NotifyConfig{Params: map[string]interface{}{"user_ids": []int64{1}}}

	SendNotifyRuleMessage(d.ctx, d.userCache, d.userGroupCache, d.notifyChannelCache, &memsto.CvalCache{},
		[]*models.AlertCurEvent{{Id: 12, Hash: "h"}}, 3, config, channel, nil)

	require.Equal(t, 1, sender.NotifyRecordQueue.Len())
	record := sender.NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, "a@example.test", record.Target)
	require.Equal(t, int64(3), record.NotifyRuleID)
	require.Equal(t, "HTTP", record.Channel)
	require.Equal(t, models.NotiStatusFailure, record.Status)
	require.Contains(t, record.Details, "failed to enqueue notify task")
}

func TestConsumer_RecordNotifyMutedExplainsSuppression(t *testing.T) {
	d := round2Dispatch(t)
	mutes := &memsto.AlertMuteCacheType{}
	mutes.Set(map[int64][]*models.AlertMute{5: {{Id: 7, Note: "maintenance"}}}, 1, 1)
	consumer := &Consumer{ctx: d.ctx, alertMuteCache: mutes}
	event := &models.AlertCurEvent{Id: 12, SubRuleId: 9, GroupId: 5, MuteId: 7, IsRecovered: true}

	consumer.recordNotifyMuted(event)

	require.Equal(t, 1, sender.NotifyRecordQueue.Len())
	record := sender.NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, models.NotiChannelMuted, record.Channel)
	require.Equal(t, models.NotiStatusMuted, record.Status)
	require.Equal(t, int64(12), record.EventId)
	require.Equal(t, int64(9), record.SubId)
	require.Equal(t, "id=7", record.Target)
	require.Equal(t,
		"recovery notification suppressed by notify-only mute rule maintenance(id=7), recovery event still recorded", record.Details)
}

func TestConsumer_MuteRuleNameFallsBackToID(t *testing.T) {
	cache := &memsto.AlertMuteCacheType{}
	cache.Set(map[int64][]*models.AlertMute{5: {{Id: 7}}}, 1, 1)
	consumer := &Consumer{alertMuteCache: cache}

	require.Equal(t, "-", consumer.muteRuleName(5, 0))
	require.Equal(t, "id=7", consumer.muteRuleName(5, 7))
	require.Equal(t, "id=8", consumer.muteRuleName(5, 8))
	require.Equal(t, "id=7", consumer.muteRuleName(6, 7))
}
