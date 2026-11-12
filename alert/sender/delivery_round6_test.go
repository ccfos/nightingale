package sender

import (
	"html/template"
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestTelegramCallback_PostsMarkdownToExplicitEndpoint(t *testing.T) {
	c := isolatedNotifyQueue(t)
	server, requests := newMessageServer(t, http.StatusOK)
	sender := &TelegramSender{tpl: messageTemplate(t)}

	sender.CallBack(CallBackContext{
		Ctx: c, CallBackURL: server.URL + "/telegram", Events: messageEvents(), Stats: newStaticWebhookTestStats(),
	})

	request := takeMessageRequest(t, requests)
	require.Equal(t, "/telegram", request.uri)
	require.JSONEq(t, `{"parse_mode":"markdown","text":"cpu=91\n\nmemory=92\n\n"}`, string(request.body))
	require.Equal(t, 2, NotifyRecordQueue.Len())
	for _, event := range messageEvents() {
		record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
		require.Equal(t, event.Id, record.EventId)
		require.Equal(t, "callback", record.Channel)
		require.Equal(t, models.NotiStatusSuccess, record.Status)
	}
}

func TestChannelCallbacks_IgnoreMissingURLOrEvents(t *testing.T) {
	c := isolatedNotifyQueue(t)
	previous := BuildTplMessage
	calls := 0
	BuildTplMessage = func(string, *template.Template, []*models.AlertCurEvent) string {
		calls++
		return "unexpected"
	}
	t.Cleanup(func() { BuildTplMessage = previous })
	callbacks := []struct {
		name   string
		sender CallBacker
	}{
		{name: "default", sender: &DefaultCallBacker{}},
		{name: "dingtalk", sender: &DingtalkSender{}},
		{name: "wecom", sender: &WecomSender{}},
		{name: "feishu", sender: &FeishuSender{}},
		{name: "feishu card", sender: &FeishuCardSender{}},
		{name: "lark", sender: &LarkSender{}},
		{name: "lark card", sender: &LarkCardSender{}},
		{name: "telegram", sender: &TelegramSender{}},
		{name: "mattermost", sender: &MmSender{}},
	}
	for _, tc := range callbacks {
		t.Run(tc.name, func(t *testing.T) {
			tc.sender.CallBack(CallBackContext{Ctx: c, Events: messageEvents()})
			tc.sender.CallBack(CallBackContext{Ctx: c, CallBackURL: "://unused"})

			require.Zero(t, calls)
			require.Zero(t, NotifyRecordQueue.Len())
		})
	}
}

func TestSendMM_RecordsMalformedDestinationAndContinues(t *testing.T) {
	c := isolatedNotifyQueue(t)
	server, requests := newMessageServer(t, http.StatusOK)
	events := messageEvents()[:1]

	SendMM(c, MatterMostMessage{
		Text: "CPU high", Tokens: []string{"http://127.0.0.1/%zz", server.URL + "/hook?channel=ops"},
		Stats: newStaticWebhookTestStats(),
	}, events, models.Mm)

	request := takeMessageRequest(t, requests)
	require.JSONEq(t, `{"channel":"ops","username":"","text":"CPU high"}`, string(request.body))
	require.Equal(t, 2, NotifyRecordQueue.Len())
	failure := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, models.NotiStatusFailure, failure.Status)
	require.Contains(t, failure.Details, "invalid URL escape")
	success := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, models.NotiStatusSuccess, success.Status)
	require.Equal(t, "ops", success.Channel)
}

func TestPushEvent_FullQueueKeepsPendingEventAndCountsFailure(t *testing.T) {
	previous := EventQueue
	queue := NewSafeEventQueue(1)
	first := &models.AlertCurEvent{Id: 1, Severity: High}
	require.True(t, queue.Push(first))
	EventQueue = map[string]*WebhookQueue{"local": {eventQueue: queue}}
	t.Cleanup(func() { EventQueue = previous })
	stats := newStaticWebhookTestStats()

	PushEvent(nil, &models.Webhook{Url: "local"}, &models.AlertCurEvent{Id: 2, Severity: High}, stats)

	require.Equal(t, 1, queue.Len())
	require.Same(t, first, queue.Pop())
	require.Equal(t, float64(1), testutil.ToFloat64(stats.AlertNotifyErrorTotal.WithLabelValues("push_event_queue")))
}

func TestPushCallbackEvent_FullQueueKeepsPendingEvent(t *testing.T) {
	previous := CallbackEventQueue
	queue := NewSafeEventQueue(1)
	first := &models.AlertCurEvent{Id: 1, Severity: High}
	require.True(t, queue.Push(first))
	CallbackEventQueue = map[string]*WebhookQueue{"local": {eventQueue: queue}}
	t.Cleanup(func() { CallbackEventQueue = previous })

	PushCallbackEvent(nil, &models.Webhook{Url: "local"}, &models.AlertCurEvent{Id: 2, Severity: High}, nil)

	require.Equal(t, 1, queue.Len())
	require.Same(t, first, queue.Pop())
}

func TestStaticGlobalWebhook_RecordsEventEncodingFailure(t *testing.T) {
	c := isolatedNotifyQueue(t)
	server, requests := newMessageServer(t, http.StatusOK)
	previousConf, previousClient := staticGlobalWebhookConf, staticGlobalWebhookClient
	t.Cleanup(func() {
		staticGlobalWebhookClient.CloseIdleConnections()
		staticGlobalWebhookConf, staticGlobalWebhookClient = previousConf, previousClient
	})
	InitStaticGlobalWebhook(aconf.GlobalWebhook{Enable: true, Url: server.URL})
	event := &models.AlertCurEvent{Id: 42, RuleConfigJson: make(chan int)}

	SendStaticGlobalWebhook(c, event, newStaticWebhookTestStats())

	require.Empty(t, requests)
	require.Equal(t, 1, NotifyRecordQueue.Len())
	record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, int64(42), record.EventId)
	require.Equal(t, models.NotiStatusFailure, record.Status)
	require.Contains(t, record.Details, "unsupported type: chan int")
}

func TestCallIbex_DeniesHostOutsideTemplateGroup(t *testing.T) {
	templates := &memsto.TaskTplCache{}
	templates.Set(map[int64]*models.TaskTpl{7: {Id: 7, GroupId: 1, UpdateBy: "alice"}}, 1, 10)
	targets := &memsto.TargetCacheType{}
	targets.Set(map[string]*models.Target{"node": {Ident: "node", GroupIds: []int64{2}}}, 1, 10)

	id, err := CallIbex(nil, 7, "node", templates, targets, &memsto.UserCacheType{}, &models.AlertCurEvent{}, "")

	require.Zero(t, id)
	require.ErrorContains(t, err, "user(alice) no permission")
}
