package sender

import (
	"encoding/json"
	"html/template"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	ctxpkg "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDingtalkCallback_WithoutMentionsPostsUnmodifiedMessage(t *testing.T) {
	c := isolatedNotifyQueue(t)
	server, requests := newMessageServer(t, http.StatusOK)
	sender := &DingtalkSender{tpl: messageTemplate(t)}

	sender.CallBack(CallBackContext{
		Ctx: c, CallBackURL: server.URL + "/hook", Events: messageEvents()[:1], Stats: newStaticWebhookTestStats(),
	})

	request := takeMessageRequest(t, requests)
	require.JSONEq(t, `{
		"msgtype":"markdown","markdown":{"title":"cpu","text":"cpu=91\n\n"},
		"at":{"atMobiles":null,"isAtAll":false}
	}`, string(request.body))
	require.Equal(t, 1, NotifyRecordQueue.Len())
}

func TestCardCallbacks_MixedRecoveryAndTriggerUseOrangeHeader(t *testing.T) {
	tests := []struct {
		name     string
		callback CallBacker
	}{
		{name: "Feishu", callback: &FeishuCardSender{tpl: template.Must(template.New("card").Parse("Recovered Triggered"))}},
		{name: "Lark", callback: &LarkCardSender{tpl: template.Must(template.New("card").Parse("Recovered Triggered"))}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := isolatedNotifyQueue(t)
			server, requests := newMessageServer(t, http.StatusOK)

			tc.callback.CallBack(CallBackContext{
				Ctx: c, CallBackURL: server.URL + "/card", Events: messageEvents(), Stats: newStaticWebhookTestStats(),
			})

			request := takeMessageRequest(t, requests)
			var body feishuCard
			require.NoError(t, json.Unmarshal(request.body, &body))
			require.Equal(t, "orange", body.Card.Header.Template)
			require.Contains(t, body.Card.Elements[0].Text.Content, "Recovered Triggered")
			require.Equal(t, 2, NotifyRecordQueue.Len())
		})
	}
}

func TestTelegramSender_BareTokenBuildsBotPathAndChatQuery(t *testing.T) {
	c := isolatedNotifyQueue(t)
	server, requests := newMessageServer(t, http.StatusOK)
	localURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(transport.CloseIdleConnections)
	previous := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		request := r.Clone(r.Context())
		request.URL.Scheme, request.URL.Host = localURL.Scheme, localURL.Host
		return transport.RoundTrip(request)
	})
	t.Cleanup(func() { http.DefaultTransport = previous })

	SendTelegram(c, TelegramMessage{
		Text: "CPU high", Tokens: []string{"bot-token/chat-42"}, Stats: newStaticWebhookTestStats(),
	}, messageEvents()[:1], models.Telegram)

	request := takeMessageRequest(t, requests)
	require.Equal(t, "/botbot-token/sendMessage?chat_id=chat-42", request.uri)
	require.Equal(t, "api.telegram.org", request.host)
	require.JSONEq(t, `{"parse_mode":"markdown","text":"CPU high"}`, string(request.body))
	require.Equal(t, 1, NotifyRecordQueue.Len())
	record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, "bot-token/chat-42", record.Target)
	require.Equal(t, models.NotiStatusSuccess, record.Status)
}

func TestEmailSender_RendersSubjectFromFirstEvent(t *testing.T) {
	previous := mailch
	mailch = make(chan *EmailContext, 1)
	t.Cleanup(func() { mailch = previous })
	sender := &EmailSender{
		subjectTpl: template.Must(template.New("subject").Parse("Alert: {{.RuleName}}")),
		contentTpl: messageTemplate(t),
	}

	sender.Send(MessageContext{
		Events: messageEvents(), Users: []*models.User{{Email: "ops@example.test"}}, Stats: newStaticWebhookTestStats(),
	})

	require.Len(t, mailch, 1)
	message := <-mailch
	require.Len(t, message.mail.GetHeader("Subject"), 1)
	subject, err := new(mime.WordDecoder).DecodeHeader(message.mail.GetHeader("Subject")[0])
	require.NoError(t, err)
	require.Equal(t, "Alert: cpu\n\n", subject)
	require.Equal(t, []string{"ops@example.test"}, message.mail.GetHeader("To"))
	require.Equal(t, messageEvents(), message.events)
}

func TestSendWebhook_RedirectLoopReturnsDeliveryFailure(t *testing.T) {
	requests := make(chan string, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		http.Redirect(w, r, "/loop", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(server.Close)
	conf := &models.Webhook{Enable: true, Url: server.URL + "/loop", Timeout: 2}
	cleanupWebhookClient(t, conf)
	stats := newStaticWebhookTestStats()

	retry, response, err := sendWebhook(conf, messageEvents()[0], stats)

	require.True(t, retry)
	require.ErrorContains(t, err, "stopped after 10 redirects")
	require.Empty(t, response)
	require.Len(t, requests, 10)
	require.Equal(t, float64(1), testutil.ToFloat64(stats.AlertNotifyErrorTotal.WithLabelValues("webhook")))
}

func TestNotifyRecordConsumer_FailedLaterBatchRollsBackEarlierRecords(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.NotificationRecord{}))
	consumer := NewNotifyRecordConsumer(&ctxpkg.Context{DB: db, IsCenter: true})
	records := make([]*models.NotificationRecord, 0, 101)
	for id := int64(1); id <= 100; id++ {
		records = append(records, &models.NotificationRecord{Id: id, EventId: id})
	}
	// The duplicate key is in batch two; the first batch must be rolled back too.
	records = append(records, &models.NotificationRecord{Id: 1, EventId: 101})

	consumer.consume(records)

	var count int64
	require.NoError(t, db.Model(&models.NotificationRecord{}).Count(&count).Error)
	require.Zero(t, count)
}
