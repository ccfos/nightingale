package dispatch

import (
	"testing"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/sender"
	"github.com/ccfos/nightingale/v6/alert/sender/provider"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestNewDispatch_DefaultSenderHookAllowsDelivery(t *testing.T) {
	base := round2Dispatch(t)
	previous := EventProcessorCache
	t.Cleanup(func() { EventProcessorCache = previous })
	d := NewDispatch(
		&memsto.AlertRuleCacheType{}, base.userCache, base.userGroupCache,
		&memsto.AlertSubscribeCacheType{}, &memsto.TargetCacheType{}, &memsto.NotifyConfigCacheType{},
		&memsto.TaskTplCache{}, base.notifyRuleCache, base.notifyChannelCache,
		base.messageTemplateCache, base.eventProcessorCache, &memsto.CvalCache{}, aconf.Alerting{}, base.ctx, nil,
	)
	stub := &round2Sender{}
	d.Senders["email"] = stub
	target := NewNotifyTarget()
	target.userMap[1] = NewNotifyChannels([]string{"email"})
	event := &models.AlertCurEvent{Hash: "cpu-high", RuleConfig: "{}"}

	d.Send(&models.AlertRule{Id: 10}, event, target, true)

	require.Len(t, stub.calls, 1)
	require.Equal(t, int64(10), stub.calls[0].Rule.Id)
	require.Equal(t, []*models.User{base.userCache.GetByUserId(1)}, stub.calls[0].Users)
	require.Equal(t, []*models.AlertCurEvent{event}, stub.calls[0].Events)
	require.Same(t, base.eventProcessorCache, EventProcessorCache)
}

func TestSendNotifyRuleMessage_NilSMTPResultRecordsDeliveryFailure(t *testing.T) {
	d := round2Dispatch(t)
	previous := provider.DefaultRegistry
	provider.DefaultRegistry = provider.NewRegistry()
	provider.DefaultRegistry.Register(&round2Provider{})
	t.Cleanup(func() { provider.DefaultRegistry = previous })
	event := &models.AlertCurEvent{Id: 12}
	channel := &models.NotifyChannelConfig{ID: 4, Name: "local email", Ident: "round2", RequestType: "smtp"}
	config := &models.NotifyConfig{Params: map[string]interface{}{"bot_name": "ops"}}

	SendNotifyRuleMessage(d.ctx, d.userCache, d.userGroupCache, d.notifyChannelCache, &memsto.CvalCache{},
		[]*models.AlertCurEvent{event}, 3, config, channel, nil)

	require.Equal(t, 1, sender.NotifyRecordQueue.Len())
	record := sender.NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, int64(12), record.EventId)
	require.Equal(t, "local email", record.Channel)
	require.Equal(t, "ops", record.Target)
	require.Equal(t, models.NotiStatusFailure, record.Status)
	require.Equal(t, "smtp provider returned nil result", record.Details)
}

func TestConsumeOne_InvalidNameTemplateBecomesDiagnostic(t *testing.T) {
	d := round2Dispatch(t)
	d.Astats = consumerStats()
	d.alertSubscribeCache = &memsto.AlertSubscribeCacheType{}
	consumer := &Consumer{ctx: d.ctx, dispatch: d, alertMuteCache: &memsto.AlertMuteCacheType{}}
	event := &models.AlertCurEvent{Id: 12, Status: 1, NotifyMuted: 1, RuleName: "{{unknownFunction}}"}

	consumer.consumeOne(event)

	require.Contains(t, event.RuleName, "failed to parse rule name:")
	require.Contains(t, event.RuleName, `function "unknownFunction" not defined`)
	require.Equal(t, 1, sender.NotifyRecordQueue.Len())
	record := sender.NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, int64(12), record.EventId)
	require.Equal(t, models.NotiStatusMuted, record.Status)
}

func TestConsumeOne_InvalidNoteTemplateBecomesDiagnostic(t *testing.T) {
	d := round2Dispatch(t)
	d.Astats = consumerStats()
	d.alertSubscribeCache = &memsto.AlertSubscribeCacheType{}
	consumer := &Consumer{ctx: d.ctx, dispatch: d, alertMuteCache: &memsto.AlertMuteCacheType{}}
	event := &models.AlertCurEvent{Id: 12, Status: 1, NotifyMuted: 1, RuleNote: "{{unknownFunction}}"}

	consumer.consumeOne(event)

	require.Contains(t, event.RuleNote, "failed to parse rule note:")
	require.Contains(t, event.RuleNote, `function "unknownFunction" not defined`)
	require.Equal(t, 1, sender.NotifyRecordQueue.Len())
	record := sender.NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, int64(12), record.EventId)
	require.Equal(t, models.NotiStatusMuted, record.Status)
}
