package sender

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	ctxpkg "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBuildCallBackContext_RendersURLAndResolvesRecipients(t *testing.T) {
	c := isolatedNotifyQueue(t)
	users := &memsto.UserCacheType{}
	user := &models.User{Id: 7, Username: "alice"}
	users.Set(map[int64]*models.User{7: user}, 1, 1, 0, 0)
	event := &models.AlertCurEvent{Id: 11, TagsMap: map[string]string{"env": "prod"}, TriggerValue: "95"}
	rule := &models.AlertRule{Id: 1}
	stats := newStaticWebhookTestStats()

	result := BuildCallBackContext(c, "http://localhost/{{$labels.env}}?value={{$value}}", rule,
		[]*models.AlertCurEvent{event}, []int64{7, 404, 7}, users, true, stats)

	require.Equal(t, "http://localhost/prod?value=95", result.CallBackURL)
	require.Equal(t, []*models.User{user}, result.Users)
	require.Equal(t, []*models.AlertCurEvent{event}, result.Events)
	require.Same(t, rule, result.Rule)
	require.Same(t, c, result.Ctx)
	require.Same(t, stats, result.Stats)
	require.True(t, result.BatchSend)
}

func TestMmCallback_PostsRenderedMessageToSelectedChannel(t *testing.T) {
	c := isolatedNotifyQueue(t)
	server, requests := newMessageServer(t, http.StatusOK)
	callback := &MmSender{tpl: messageTemplate(t)}

	callback.CallBack(CallBackContext{
		Ctx: c, CallBackURL: server.URL + "/hook?channel=ops&username=bot&atuser=alice",
		Events: messageEvents(), Stats: newStaticWebhookTestStats(),
	})

	request := takeMessageRequest(t, requests)
	require.Equal(t, "/hook", request.uri)
	require.JSONEq(t, `{"channel":"ops","username":"bot","text":"@alice\ncpu=91\n\nmemory=92\n\n"}`, string(request.body))
	require.Equal(t, 2, NotifyRecordQueue.Len())
	record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, "ops", record.Channel)
	require.Equal(t, models.NotiStatusSuccess, record.Status)
}

func TestSingleSendWebhooks_PostsEventAndRecordsDelivery(t *testing.T) {
	c := isolatedNotifyQueue(t)
	server, requests := newMessageServer(t, http.StatusOK)
	config := &models.Webhook{Enable: true, Url: server.URL + "/events", Timeout: 1}
	cleanupWebhookClient(t, config)
	event := messageEvents()[0]

	SingleSendWebhooks(c, map[string]*models.Webhook{"test": config}, event, newStaticWebhookTestStats())

	request := takeMessageRequest(t, requests)
	require.Equal(t, "/events", request.uri)
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	require.JSONEq(t, string(encoded), string(request.body))
	require.Equal(t, 1, NotifyRecordQueue.Len())
	record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, event.Id, record.EventId)
	require.Equal(t, "webhook", record.Channel)
	require.Equal(t, models.NotiStatusSuccess, record.Status)
	require.Contains(t, record.Details, "status_code:200")
}

func TestRecordNotifications_PreservesMutedStatusInCenterQueue(t *testing.T) {
	c := isolatedNotifyQueue(t)
	record := &models.NotificationRecord{EventId: 42, Status: models.NotiStatusMuted, Details: "maintenance"}

	RecordNotifications(c, []*models.NotificationRecord{record})
	RecordNotifications(c, nil)

	require.Equal(t, 1, NotifyRecordQueue.Len())
	require.Same(t, record, NotifyRecordQueue.PopBack())
	require.Equal(t, models.NotiStatusMuted, record.Status)
	require.Equal(t, "maintenance", record.Details)
}

func TestRecordNotifications_EdgePostsCustomStatusToCenter(t *testing.T) {
	isolatedNotifyQueue(t)
	received := make(chan []*models.NotificationRecord, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var records []*models.NotificationRecord
		err := json.NewDecoder(r.Body).Decode(&records)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/n9e/notify-record" || err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- records
		_, _ = w.Write([]byte(`{"err":""}`))
	}))
	t.Cleanup(server.Close)
	c := &ctxpkg.Context{CenterApi: conf.CenterApi{Addrs: []string{server.URL}}}
	record := &models.NotificationRecord{EventId: 42, Status: models.NotiStatusMuted, Details: "maintenance"}

	RecordNotifications(c, []*models.NotificationRecord{record})

	select {
	case records := <-received:
		require.Equal(t, []*models.NotificationRecord{record}, records)
	default:
		t.Fatal("notification record was not posted")
	}
	require.Zero(t, NotifyRecordQueue.Len())
}

func TestNotifyRecord_EdgePostsFailureForEveryEvent(t *testing.T) {
	isolatedNotifyQueue(t)
	received := make(chan []*models.NotificationRecord, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var records []*models.NotificationRecord
		if err := json.NewDecoder(r.Body).Decode(&records); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- records
		_, _ = w.Write([]byte(`{"err":""}`))
	}))
	t.Cleanup(server.Close)
	c := &ctxpkg.Context{CenterApi: conf.CenterApi{Addrs: []string{server.URL}}}

	NotifyRecord(c, messageEvents(), 9, "callback", "destination", "", errors.New("delivery refused"))

	select {
	case records := <-received:
		require.Len(t, records, 2)
		for i, record := range records {
			require.Equal(t, messageEvents()[i].Id, record.EventId)
			require.Equal(t, int64(9), record.NotifyRuleID)
			require.Equal(t, "callback", record.Channel)
			require.Equal(t, "destination", record.Target)
			require.Equal(t, models.NotiStatusFailure, record.Status)
			require.Equal(t, "delivery refused", record.Details)
		}
	default:
		t.Fatal("notification records were not posted")
	}
	require.Zero(t, NotifyRecordQueue.Len())
}

func TestNotifyRecordConsumer_PersistsNotificationBatch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.NotificationRecord{}))
	consumer := NewNotifyRecordConsumer(&ctxpkg.Context{DB: db, IsCenter: true})
	records := []*models.NotificationRecord{
		{Id: 1, EventId: 1, Channel: "webhook", Status: models.NotiStatusSuccess, Details: "ok"},
		{Id: 2, EventId: 2, Channel: "callback", Status: models.NotiStatusFailure, Details: "refused"},
	}

	consumer.consume(records)

	var persisted []*models.NotificationRecord
	require.NoError(t, db.Order("event_id").Find(&persisted).Error)
	require.Len(t, persisted, 2)
	for i := range records {
		require.NotZero(t, persisted[i].Id)
		require.Equal(t, records[i].EventId, persisted[i].EventId)
		require.Equal(t, records[i].Channel, persisted[i].Channel)
		require.Equal(t, records[i].Status, persisted[i].Status)
		require.Equal(t, records[i].Details, persisted[i].Details)
	}
}
