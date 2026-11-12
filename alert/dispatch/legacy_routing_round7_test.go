package dispatch

import (
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/alert/sender"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

type round7RoutingSender struct{ messages chan sender.MessageContext }

func (s *round7RoutingSender) Send(message sender.MessageContext) { s.messages <- message }

func round7LegacyDispatch(t *testing.T) (*Dispatch, <-chan sender.MessageContext) {
	t.Helper()
	d := round2Dispatch(t)
	d.alertRuleCache = &memsto.AlertRuleCacheType{}
	d.alertRuleCache.Set(map[int64]*models.AlertRule{10: {Id: 10, RuleConfig: "{}"}}, 1, 1)
	d.alertSubscribeCache = &memsto.AlertSubscribeCacheType{}
	d.notifyConfigCache = &memsto.NotifyConfigCacheType{}
	d.BeforeSenderHook = func(*models.AlertCurEvent) bool { return true }
	d.Astats = &astats.Stats{CounterSubEventTotal: prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "round7_subscription_events"}, []string{"group"}),
	}
	messages := make(chan sender.MessageContext, 2)
	d.Senders = map[string]sender.Sender{"email": &round7RoutingSender{messages: messages}}
	return d, messages
}

func round7RoutedMessage(t *testing.T, messages <-chan sender.MessageContext) sender.MessageContext {
	t.Helper()
	select {
	case message := <-messages:
		return message
	case <-time.After(5 * time.Second):
		t.Fatal("notification did not reach the in-process sender")
		return sender.MessageContext{}
	}
}

func TestHandleEventNotify_SubscriptionRoutesGroupRecipients(t *testing.T) {
	d, messages := round7LegacyDispatch(t)
	event := &models.AlertCurEvent{
		RuleId: 10, Hash: "cpu-high", RuleConfig: "{}",
		NotifyGroupsJSON: []string{"7"}, NotifyChannelsJSON: []string{"email"},
	}

	d.HandleEventNotify(event, true)
	message := round7RoutedMessage(t, messages)

	require.Equal(t, int64(10), message.Rule.Id)
	require.Equal(t, []*models.AlertCurEvent{event}, message.Events)
	require.ElementsMatch(t, []*models.User{d.userCache.GetByUserId(1), d.userCache.GetByUserId(2), d.userCache.GetByUserId(3)},
		message.Users)
	require.Len(t, event.NotifyGroupsObj, 1)
}

func TestHandleSub_RewritesSubscriptionCopyBeforeDelivery(t *testing.T) {
	d, messages := round7LegacyDispatch(t)
	event := models.AlertCurEvent{
		RuleId: 10, Hash: "cpu-high", RuleConfig: "{}", GroupName: "ops", Severity: 1,
		NotifyChannelsJSON: []string{"unknown"}, NotifyGroupsJSON: []string{"99"},
	}
	subscription := &models.AlertSubscribe{
		Id: 20, RedefineSeverity: 1, NewSeverity: 3,
		RedefineChannels: 1, NewChannels: "email", UserGroupIds: "7",
	}

	d.handleSub(subscription, event)
	message := round7RoutedMessage(t, messages)

	require.Len(t, message.Events, 1)
	delivered := message.Events[0]
	require.Equal(t, int64(20), delivered.SubRuleId)
	require.Equal(t, 3, delivered.Severity)
	require.Equal(t, []string{"email"}, delivered.NotifyChannelsJSON)
	require.Equal(t, []string{"7"}, delivered.NotifyGroupsJSON)
	require.Equal(t, 1.0, testutil.ToFloat64(d.Astats.CounterSubEventTotal.WithLabelValues("ops")))
	require.Equal(t, 1, event.Severity)
	require.Zero(t, event.SubRuleId)
	require.Equal(t, []string{"unknown"}, event.NotifyChannelsJSON)
	require.Equal(t, []string{"99"}, event.NotifyGroupsJSON)
}

func TestHandleSubs_OnlyMatchingSubscriptionsReachSender(t *testing.T) {
	d, messages := round7LegacyDispatch(t)
	d.alertSubscribeCache.Set(map[int64][]*models.AlertSubscribe{10: {
		{Id: 20, Disabled: 1, UserGroupIds: "7"},
		{Id: 21, SeveritiesJson: []int{2}, UserGroupIds: "7"},
		{Id: 22, SeveritiesJson: []int{1}, UserGroupIds: "7"},
	}}, 3, 1)
	event := &models.AlertCurEvent{
		RuleId: 10, Hash: "cpu-high", RuleConfig: "{}", GroupName: "ops", Severity: 1,
		NotifyChannelsJSON: []string{"email"},
	}

	d.handleSubs(event)
	message := round7RoutedMessage(t, messages)

	require.Equal(t, int64(22), message.Events[0].SubRuleId)
	require.Equal(t, 1.0, testutil.ToFloat64(d.Astats.CounterSubEventTotal.WithLabelValues("ops")))
	require.Empty(t, messages)
	require.Zero(t, event.SubRuleId)
}

func TestSendNotifyRuleMessage_MissingHTTPQueueRecordsDeliveryFailure(t *testing.T) {
	d := round2Dispatch(t)
	channel := &models.NotifyChannelConfig{
		ID: 4, Name: "local callback", Ident: "callback", RequestType: "http",
		RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{
			URL: "http://127.0.0.1", Method: "POST",
		}},
	}
	event := &models.AlertCurEvent{Id: 12, Hash: "cpu-high"}
	config := &models.NotifyConfig{Params: map[string]interface{}{"bot_name": "ops"}}

	SendNotifyRuleMessage(d.ctx, d.userCache, d.userGroupCache, d.notifyChannelCache, &memsto.CvalCache{},
		[]*models.AlertCurEvent{event}, 3, config, channel, nil)

	require.Equal(t, 1, sender.NotifyRecordQueue.Len())
	record := sender.NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, int64(12), record.EventId)
	require.Equal(t, int64(3), record.NotifyRuleID)
	require.Equal(t, "local callback", record.Channel)
	require.Equal(t, "ops", record.Target)
	require.Equal(t, models.NotiStatusFailure, record.Status)
	require.Equal(t, "failed to enqueue notify task, queue is full", record.Details)
}
