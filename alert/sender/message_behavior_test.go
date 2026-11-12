package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	ctxpkg "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
	"github.com/toolkits/pkg/container/list"
)

type capturedMessageRequest struct {
	method string
	uri    string
	host   string
	header http.Header
	body   []byte
	err    error
}

func newMessageServer(t *testing.T, status int) (*httptest.Server, <-chan capturedMessageRequest) {
	t.Helper()
	requests := make(chan capturedMessageRequest, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		requests <- capturedMessageRequest{
			method: r.Method, uri: r.URL.RequestURI(), host: r.Host,
			header: r.Header.Clone(), body: body, err: err,
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)
	return server, requests
}

func takeMessageRequest(t *testing.T, requests <-chan capturedMessageRequest) capturedMessageRequest {
	t.Helper()
	select {
	case request := <-requests:
		require.NoError(t, request.err)
		require.Equal(t, http.MethodPost, request.method)
		require.Equal(t, "application/json", request.header.Get("Content-Type"))
		return request
	default:
		t.Fatal("sender did not issue a request")
		return capturedMessageRequest{}
	}
}

func isolatedNotifyQueue(t *testing.T) *ctxpkg.Context {
	t.Helper()
	previous := NotifyRecordQueue
	NotifyRecordQueue = list.NewSafeListLimited(100)
	t.Cleanup(func() { NotifyRecordQueue = previous })
	return ctxpkg.NewContext(context.Background(), nil, true)
}

func messageEvents() []*models.AlertCurEvent {
	return []*models.AlertCurEvent{
		{Id: 101, RuleName: "cpu", TriggerValue: "91"},
		{Id: 102, RuleName: "memory", TriggerValue: "92"},
	}
}

func messageTemplate(t *testing.T) *template.Template {
	t.Helper()
	return template.Must(template.New("message").Parse(`{{.RuleName}}={{.TriggerValue}}`))
}

func assertChatSenderBody(t *testing.T, channel, contactKey, wantJSON string) {
	t.Helper()
	ctx := isolatedNotifyQueue(t)
	server, requests := newMessageServer(t, http.StatusOK)
	contacts, err := json.Marshal(map[string]string{contactKey: server.URL + "/hook"})
	require.NoError(t, err)
	sender := NewSender(channel, map[string]*template.Template{channel: messageTemplate(t)})
	events := messageEvents()

	sender.Send(MessageContext{
		Ctx: ctx, Stats: newStaticWebhookTestStats(), Events: events,
		Users: []*models.User{{Phone: "123", Contacts: contacts}},
	})

	request := takeMessageRequest(t, requests)
	require.Equal(t, "/hook", request.uri)
	require.JSONEq(t, wantJSON, string(request.body))
	require.Empty(t, requests)
	// One HTTP message contains both events, each with its own delivery record.
	require.Equal(t, 2, NotifyRecordQueue.Len())
	for _, event := range events {
		record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
		require.Equal(t, event.Id, record.EventId)
		require.Equal(t, channel, record.Channel)
		require.Equal(t, models.NotiStatusSuccess, record.Status)
		require.Equal(t, server.URL+"/hook", record.Target)
		require.Contains(t, record.Details, "status_code:200")
	}
}

func TestWecomSender_PostsRenderedEventBatch(t *testing.T) {
	assertChatSenderBody(t, models.Wecom, models.WecomKey,
		`{"msgtype":"markdown","markdown":{"content":"cpu=91\n\nmemory=92\n\n"}}`)
}

func TestFeishuSender_PostsRenderedEventBatch(t *testing.T) {
	assertChatSenderBody(t, models.Feishu, models.FeishuKey,
		`{"msg_type":"text","content":{"text":"cpu=91\n\nmemory=92\n\n"},"at":{"atMobiles":["123"],"isAtAll":false}}`)
}

func TestLarkSender_PostsRenderedEventBatch(t *testing.T) {
	assertChatSenderBody(t, models.Lark, models.LarkKey,
		`{"msg_type":"text","content":{"text":"cpu=91\n\nmemory=92\n\n"},"at":{"atMobiles":null,"isAtAll":false}}`)
}

func TestTelegramSender_PostsRenderedEventBatch(t *testing.T) {
	assertChatSenderBody(t, models.Telegram, models.TelegramKey,
		`{"parse_mode":"markdown","text":"cpu=91\n\nmemory=92\n\n"}`)
}

func TestDingtalkSender_NoAtSuppressesMentions(t *testing.T) {
	ctx := isolatedNotifyQueue(t)
	server, requests := newMessageServer(t, http.StatusOK)
	contacts, err := json.Marshal(map[string]string{models.DingtalkKey: server.URL + "/hook?noat=1"})
	require.NoError(t, err)
	sender := &DingtalkSender{tpl: messageTemplate(t)}

	sender.Send(MessageContext{
		Ctx: ctx, Stats: newStaticWebhookTestStats(), Events: messageEvents(),
		Users: []*models.User{{Phone: "123", Contacts: contacts}},
	})

	request := takeMessageRequest(t, requests)
	var body dingtalk
	require.NoError(t, json.Unmarshal(request.body, &body))
	require.Equal(t, "markdown", body.Msgtype)
	require.Equal(t, "cpu", body.Markdown.Title)
	require.Equal(t, "cpu=91\n\nmemory=92\n\n", body.Markdown.Text)
	require.Empty(t, body.At.AtMobiles)
}

func TestFeishuSender_NoAtSuppressesMentions(t *testing.T) {
	ctx := isolatedNotifyQueue(t)
	server, requests := newMessageServer(t, http.StatusOK)
	contacts, err := json.Marshal(map[string]string{models.FeishuKey: server.URL + "/hook?noat=1"})
	require.NoError(t, err)
	sender := &FeishuSender{tpl: messageTemplate(t)}

	sender.Send(MessageContext{
		Ctx: ctx, Stats: newStaticWebhookTestStats(), Events: messageEvents(),
		Users: []*models.User{{Phone: "123", Contacts: contacts}},
	})

	request := takeMessageRequest(t, requests)
	var body feishu
	require.NoError(t, json.Unmarshal(request.body, &body))
	require.Equal(t, "cpu=91\n\nmemory=92\n\n", body.Content.Text)
	require.Empty(t, body.At.AtMobiles)
}

func TestCardSenders_ColorReflectsBatchState(t *testing.T) {
	tests := []struct {
		name    string
		message string
		color   string
	}{
		{name: "firing", message: "Triggered", color: "red"},
		{name: "recovered", message: "Recovered", color: "green"},
		{name: "mixed", message: "Recovered and Triggered", color: "orange"},
	}
	for _, channel := range []string{models.FeishuCard, models.LarkCard} {
		t.Run(channel, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					ctx := isolatedNotifyQueue(t)
					server, requests := newMessageServer(t, http.StatusOK)
					key := models.FeishuKey
					if channel == models.LarkCard {
						key = models.LarkKey
					}
					contacts, err := json.Marshal(map[string]string{key: server.URL + "/card"})
					require.NoError(t, err)
					tpl := template.Must(template.New("card").Parse(tt.message))
					sender := NewSender(channel, map[string]*template.Template{channel: tpl})

					sender.Send(MessageContext{
						Ctx: ctx, Stats: newStaticWebhookTestStats(), Events: messageEvents(),
						Users: []*models.User{{Contacts: contacts}},
					})

					request := takeMessageRequest(t, requests)
					var body feishuCard
					require.NoError(t, json.Unmarshal(request.body, &body))
					require.Equal(t, "interactive", body.Msgtype)
					require.Equal(t, "🔔 cpu", body.Card.Header.Title.Content)
					require.Equal(t, tt.color, body.Card.Header.Template)
					require.True(t, body.Card.Config.WideScreenMode)
					require.Len(t, body.Card.Elements, 3)
					require.Equal(t, tt.message+"\n\n"+tt.message+"\n\n", body.Card.Elements[0].Text.Content)
					require.Equal(t, "🔔 cpu", body.Card.Elements[2].Elements[0].Content)
				})
			}
		})
	}
}

func TestChatCallbacks_PostChannelSpecificBodies(t *testing.T) {
	tests := []struct {
		name     string
		sender   CallBacker
		wantJSON string
	}{
		{
			name: "dingtalk", sender: &DingtalkSender{tpl: messageTemplate(t)},
			wantJSON: `{"msgtype":"markdown","markdown":{"title":"cpu","text":"cpu=91\n\nmemory=92\n\n\n@a@b"},` +
				`"at":{"atMobiles":["a","b"],"isAtAll":false}}`,
		},
		{
			name: "wecom", sender: &WecomSender{tpl: messageTemplate(t)},
			wantJSON: `{"msgtype":"markdown","markdown":{"content":"cpu=91\n\nmemory=92\n\n"}}`,
		},
		{
			name: "feishu", sender: &FeishuSender{tpl: messageTemplate(t)},
			wantJSON: `{"msg_type":"text","content":{"text":"<at user_id=\"a\"></at> <at user_id=\"b\"></at> ` +
				`cpu=91\n\nmemory=92\n\n"},"at":{"atMobiles":null,"isAtAll":false}}`,
		},
		{
			name: "lark", sender: &LarkSender{tpl: messageTemplate(t)},
			wantJSON: `{"msg_type":"text","content":{"text":"cpu=91\n\nmemory=92\n\n"},` +
				`"at":{"atMobiles":null,"isAtAll":false}}`,
		},
		{
			name: "telegram", sender: &TelegramSender{tpl: messageTemplate(t)},
			wantJSON: `{"parse_mode":"markdown","text":"cpu=91\n\nmemory=92\n\n"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := isolatedNotifyQueue(t)
			server, requests := newMessageServer(t, http.StatusOK)

			tt.sender.CallBack(CallBackContext{
				Ctx: ctx, Stats: newStaticWebhookTestStats(), Events: messageEvents(),
				CallBackURL: server.URL + "/callback?ats=a,b",
			})

			request := takeMessageRequest(t, requests)
			require.Equal(t, "/callback?ats=a,b", request.uri)
			require.JSONEq(t, tt.wantJSON, string(request.body))
		})
	}
}

func TestFeishuCardCallback_StripsQueryAndRendersMentions(t *testing.T) {
	ctx := isolatedNotifyQueue(t)
	server, requests := newMessageServer(t, http.StatusOK)
	sender := &FeishuCardSender{tpl: template.Must(template.New("card").Parse("Recovered"))}

	sender.CallBack(CallBackContext{
		Ctx: ctx, Stats: newStaticWebhookTestStats(), Events: messageEvents(),
		CallBackURL: server.URL + "/card?ats=alice@example.com,bob",
	})

	request := takeMessageRequest(t, requests)
	var body feishuCard
	require.NoError(t, json.Unmarshal(request.body, &body))
	require.Equal(t, "/card", request.uri)
	require.Equal(t, "green", body.Card.Header.Template)
	require.Equal(t, `<at email="alice@example.com" ></at><at id="bob" ></at>Recovered`+
		"\n\nRecovered\n\n", body.Card.Elements[0].Text.Content)
}

func TestLarkCardCallback_StripsQueryAndRendersMentions(t *testing.T) {
	ctx := isolatedNotifyQueue(t)
	server, requests := newMessageServer(t, http.StatusOK)
	sender := &LarkCardSender{tpl: template.Must(template.New("card").Parse("Recovered"))}

	sender.CallBack(CallBackContext{
		Ctx: ctx, Stats: newStaticWebhookTestStats(), Events: messageEvents(),
		CallBackURL: server.URL + "/card?ats=alice@example.com,bob",
	})

	// The Lark card API rejects query strings, so mentions move into the body.
	request := takeMessageRequest(t, requests)
	var body feishuCard
	require.NoError(t, json.Unmarshal(request.body, &body))
	require.Equal(t, "/card", request.uri)
	require.Equal(t, `<at email="alice@example.com" ></at><at id="bob" ></at>Recovered`+
		"\n\nRecovered\n\n", body.Card.Elements[0].Text.Content)
}

func TestMmSender_FansOutChannelsAndMentions(t *testing.T) {
	ctx := isolatedNotifyQueue(t)
	server, requests := newMessageServer(t, http.StatusOK)
	token := server.URL + "/mm?channel=ops&channel=dev&atuser=alice&atuser=bob&username=n9e"
	contacts, err := json.Marshal(map[string]string{models.MmKey: token})
	require.NoError(t, err)
	sender := &MmSender{tpl: messageTemplate(t)}

	sender.Send(MessageContext{
		Ctx: ctx, Stats: newStaticWebhookTestStats(), Events: messageEvents(),
		Users: []*models.User{{Contacts: contacts}},
	})

	for _, channel := range []string{"ops", "dev"} {
		request := takeMessageRequest(t, requests)
		var body mm
		require.NoError(t, json.Unmarshal(request.body, &body))
		require.Equal(t, "/mm", request.uri)
		require.Equal(t, channel, body.Channel)
		require.Equal(t, "n9e", body.Username)
		require.Equal(t, "@alice,@bob\ncpu=91\n\nmemory=92\n\n", body.Text)
	}
	require.Empty(t, requests)
}

func TestEmailSender_QueuesRenderedHTMLAndRecipients(t *testing.T) {
	previous := mailch
	mailch = make(chan *EmailContext, 1)
	t.Cleanup(func() { mailch = previous })
	sender := &EmailSender{
		contentTpl: messageTemplate(t), smtp: aconf.SMTPConfig{From: "n9e@example.com"},
	}
	events := messageEvents()

	sender.Send(MessageContext{
		Stats: newStaticWebhookTestStats(), Events: events,
		Users: []*models.User{{Email: "ops@example.com"}, {Email: ""}, {Email: "dev@example.com"}},
	})

	require.Len(t, mailch, 1)
	queued := <-mailch
	require.Equal(t, events, queued.events)
	require.Equal(t, []string{"ops@example.com", "dev@example.com"}, queued.mail.GetHeader("To"))
	require.Equal(t, []string{"n9e@example.com"}, queued.mail.GetHeader("From"))
	var wire bytes.Buffer
	_, err := queued.mail.WriteTo(&wire)
	require.NoError(t, err)
	message, err := mail.ReadMessage(&wire)
	require.NoError(t, err)
	subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	require.NoError(t, err)
	require.Equal(t, "cpu", subject)
	require.Contains(t, message.Header.Get("Content-Type"), "text/html")
	body, err := io.ReadAll(quotedprintable.NewReader(message.Body))
	require.NoError(t, err)
	require.Equal(t, "cpu=91\r\n\r\nmemory=92\r\n\r\n", string(body))
}

func TestBuildTplMessage_RendersEventsInOrder(t *testing.T) {
	got := buildTplMessage("test", messageTemplate(t), messageEvents())

	require.Equal(t, "cpu=91\n\nmemory=92\n\n", got)
}

func TestBuildTplMessage_ReportsMissingTemplate(t *testing.T) {
	got := buildTplMessage("test", nil, messageEvents())

	require.Contains(t, got, "tpl for current sender not found")
}

func TestBuildTplMessage_ReportsExecutionError(t *testing.T) {
	bad := template.Must(template.New("bad").Parse(`{{.MissingField}}`))

	got := buildTplMessage("test", bad, messageEvents())

	require.Contains(t, got, "can't evaluate field MissingField")
}

func TestTelegramSender_InvalidTokenRecordsFailure(t *testing.T) {
	ctx := isolatedNotifyQueue(t)

	SendTelegram(ctx, TelegramMessage{Tokens: []string{"invalid"}}, messageEvents(), models.Telegram)

	require.Equal(t, 2, NotifyRecordQueue.Len())
	for range messageEvents() {
		record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
		require.Equal(t, models.NotiStatusFailure, record.Status)
		require.Equal(t, "invalid token", record.Details)
	}
}

func TestChatSenders_IgnoreMissingUsersOrEvents(t *testing.T) {
	for _, channel := range []string{
		models.Dingtalk, models.Wecom, models.Feishu, models.FeishuCard,
		models.Lark, models.LarkCard, models.Telegram, models.Mm, models.Email,
	} {
		t.Run(channel, func(t *testing.T) {
			sender := NewSender(channel, nil, aconf.SMTPConfig{})
			previous := BuildTplMessage
			calls := 0
			BuildTplMessage = func(string, *template.Template, []*models.AlertCurEvent) string {
				calls++
				return "unexpected rendering"
			}
			t.Cleanup(func() { BuildTplMessage = previous })

			sender.Send(MessageContext{Events: messageEvents()})
			sender.Send(MessageContext{Users: []*models.User{{}}})

			require.Zero(t, calls)
		})
	}
}

func TestBuildMessageContext_DeduplicatesUsersAndSkipsMissingIDs(t *testing.T) {
	cache := &memsto.UserCacheType{}
	first := &models.User{Id: 1, Username: "alice"}
	second := &models.User{Id: 2, Username: "bob"}
	cache.Set(map[int64]*models.User{1: first, 2: second}, 2, 1, 0, 0)
	ctx := ctxpkg.NewContext(context.Background(), nil, true)
	rule := &models.AlertRule{Id: 7}
	events := messageEvents()
	stats := newStaticWebhookTestStats()

	got := BuildMessageContext(ctx, rule, events, []int64{2, 1, 2, 99}, cache, stats)

	require.Equal(t, []*models.User{second, first}, got.Users)
	require.Same(t, ctx, got.Ctx)
	require.Same(t, rule, got.Rule)
	require.Same(t, stats, got.Stats)
	require.Equal(t, events, got.Events)
}

func TestChatSenders_ExpandBareTokensAndPreserveExplicitURLs(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		prefix  string
		extract func([]*models.User) ([]string, []string)
	}{
		{
			name: "dingtalk", key: models.DingtalkKey,
			prefix: "https://oapi.dingtalk.com/robot/send?access_token=",
			extract: func(users []*models.User) ([]string, []string) {
				urls, _, tokens := new(DingtalkSender).extract(users)
				return urls, tokens
			},
		},
		{
			name: "wecom", key: models.WecomKey,
			prefix: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=", extract: new(WecomSender).extract,
		},
		{
			name: "feishu", key: models.FeishuKey, prefix: "https://open.feishu.cn/open-apis/bot/v2/hook/",
			extract: func(users []*models.User) ([]string, []string) {
				urls, _, tokens := new(FeishuSender).extract(users)
				return urls, tokens
			},
		},
		{
			name: "feishu card", key: models.FeishuKey,
			prefix: "https://open.feishu.cn/open-apis/bot/v2/hook/", extract: new(FeishuCardSender).extract,
		},
		{
			name: "lark", key: models.LarkKey,
			prefix: "https://open.larksuite.com/open-apis/bot/v2/hook/", extract: new(LarkSender).extract,
		},
		{
			name: "lark card", key: models.LarkKey,
			prefix: "https://open.larksuite.com/open-apis/bot/v2/hook/", extract: new(LarkCardSender).extract,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bare, err := json.Marshal(map[string]string{tt.key: "test-token"})
			require.NoError(t, err)
			explicit, err := json.Marshal(map[string]string{tt.key: "http://127.0.0.1/hook"})
			require.NoError(t, err)
			users := []*models.User{{Contacts: bare}, {Contacts: []byte(`{}`)}, {Contacts: explicit}}

			urls, tokens := tt.extract(users)

			require.Equal(t, []string{tt.prefix + "test-token", "http://127.0.0.1/hook"}, urls)
			require.Equal(t, []string{"test-token", "http://127.0.0.1/hook"}, tokens)
		})
	}
}

func cleanupWebhookClient(t *testing.T, conf *models.Webhook) {
	t.Helper()
	key := conf.Hash()
	previous, existed := webhookClientCache.Load(key)
	t.Cleanup(func() {
		if value, ok := webhookClientCache.LoadAndDelete(key); ok && value != previous {
			value.(*http.Client).CloseIdleConnections()
		}
		if existed {
			webhookClientCache.Store(key, previous)
		}
	})
}

func TestSendWebhook_PostsEventWithAuthAndHeaders(t *testing.T) {
	server, requests := newMessageServer(t, http.StatusAccepted)
	conf := &models.Webhook{
		Enable: true, Url: server.URL + "/events?source=n9e", Timeout: 2,
		BasicAuthUser: "user", BasicAuthPass: "secret",
		Headers: []string{"Host", "receiver.internal", "X-Tenant", "west"},
	}
	cleanupWebhookClient(t, conf)
	event := messageEvents()[0]

	retry, response, err := sendWebhook(conf, event, newStaticWebhookTestStats())

	require.NoError(t, err)
	require.False(t, retry)
	require.Equal(t, `status_code:202, response:{"ok":true}`, response)
	request := takeMessageRequest(t, requests)
	require.Equal(t, "/events?source=n9e", request.uri)
	require.Equal(t, "receiver.internal", request.host)
	require.Equal(t, "west", request.header.Get("X-Tenant"))
	parsed := &http.Request{Header: request.header}
	user, pass, ok := parsed.BasicAuth()
	require.True(t, ok)
	require.Equal(t, "user", user)
	require.Equal(t, "secret", pass)
	want, err := json.Marshal(event)
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(request.body))
}

func TestSendWebhook_RateLimitRequestsRetry(t *testing.T) {
	server, requests := newMessageServer(t, http.StatusTooManyRequests)
	conf := &models.Webhook{Enable: true, Url: server.URL, Type: models.RuleCallback}
	cleanupWebhookClient(t, conf)

	retry, response, err := sendWebhook(conf, messageEvents(), newStaticWebhookTestStats())

	require.True(t, retry)
	require.ErrorContains(t, err, "429")
	require.Contains(t, response, "status_code:429")
	request := takeMessageRequest(t, requests)
	var got []*models.AlertCurEvent
	require.NoError(t, json.Unmarshal(request.body, &got))
	require.Len(t, got, 2)
	require.Equal(t, int64(101), got[0].Id)
	require.Equal(t, int64(102), got[1].Id)
}

func TestSendWebhook_DisabledOrMissingURLDoesNotPost(t *testing.T) {
	server, requests := newMessageServer(t, http.StatusOK)
	for _, conf := range []*models.Webhook{{Url: server.URL}, {Enable: true}} {
		retry, response, err := sendWebhook(conf, messageEvents(), nil)

		require.NoError(t, err)
		require.False(t, retry)
		require.Empty(t, response)
		require.Empty(t, requests)
	}
}

func TestSendWebhook_RejectsUnserializableBody(t *testing.T) {
	server, requests := newMessageServer(t, http.StatusOK)
	conf := &models.Webhook{Enable: true, Url: server.URL}

	retry, _, err := sendWebhook(conf, make(chan int), newStaticWebhookTestStats())

	require.ErrorContains(t, err, "unsupported type")
	require.False(t, retry)
	require.Empty(t, requests)
}

func TestSendWebhook_InvalidURLReturnsError(t *testing.T) {
	conf := &models.Webhook{Enable: true, Url: "://invalid"}

	retry, _, err := sendWebhook(conf, messageEvents(), newStaticWebhookTestStats())

	require.Error(t, err)
	require.True(t, retry)
}

func TestGetWebhookClient_ReusesClientAndAppliesTimeout(t *testing.T) {
	conf := &models.Webhook{Enable: true, Url: "http://127.0.0.1/webhook", Timeout: 2, SkipVerify: true}
	cleanupWebhookClient(t, conf)

	first := getWebhookClient(conf)
	second := getWebhookClient(conf)

	require.Same(t, first, second)
	require.Equal(t, 2*time.Second, first.Timeout)
	transport := first.Transport.(*http.Transport)
	require.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestDefaultCallback_PostsEventBody(t *testing.T) {
	ctx := isolatedNotifyQueue(t)
	server, requests := newMessageServer(t, http.StatusOK)
	events := messageEvents()[:1]

	new(DefaultCallBacker).CallBack(CallBackContext{
		Ctx: ctx, CallBackURL: server.URL + "/callback", Events: events, Stats: newStaticWebhookTestStats(),
	})

	request := takeMessageRequest(t, requests)
	want, err := json.Marshal(events[0])
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(request.body))
	require.Equal(t, "/callback", request.uri)
	require.Equal(t, 1, NotifyRecordQueue.Len())
}

func TestDefaultCallback_InvalidURLRecordsFailure(t *testing.T) {
	ctx := isolatedNotifyQueue(t)

	new(DefaultCallBacker).CallBack(CallBackContext{
		Ctx: ctx, CallBackURL: "://invalid", Events: messageEvents()[:1], Stats: newStaticWebhookTestStats(),
	})

	require.Equal(t, 1, NotifyRecordQueue.Len())
	record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, int64(101), record.EventId)
	require.Equal(t, "callback", record.Channel)
	require.Equal(t, "://invalid", record.Target)
	require.Equal(t, models.NotiStatusFailure, record.Status)
	require.Contains(t, record.Details, "missing protocol scheme")
}

func TestBatchSendWebhooks_QueuesByDestination(t *testing.T) {
	previous := EventQueue
	first := &WebhookQueue{eventQueue: NewSafeEventQueue(10)}
	second := &WebhookQueue{eventQueue: NewSafeEventQueue(10)}
	EventQueue = map[string]*WebhookQueue{"first": first, "second": second}
	t.Cleanup(func() { EventQueue = previous })
	event := &models.AlertCurEvent{Id: 7, Severity: High}
	webhooks := map[string]*models.Webhook{
		"first": {Url: "first"}, "second": {Url: "second"},
	}

	BatchSendWebhooks(nil, webhooks, event, newStaticWebhookTestStats())

	require.Equal(t, 1, first.eventQueue.Len())
	require.Equal(t, 1, second.eventQueue.Len())
	require.Same(t, event, first.eventQueue.Pop())
	require.Same(t, event, second.eventQueue.Pop())
}

func TestDefaultCallback_BatchQueuesEvent(t *testing.T) {
	previous := CallbackEventQueue
	queue := &WebhookQueue{eventQueue: NewSafeEventQueue(10)}
	CallbackEventQueue = map[string]*WebhookQueue{"local-callback": queue}
	t.Cleanup(func() { CallbackEventQueue = previous })
	event := &models.AlertCurEvent{Id: 7, Severity: High}

	new(DefaultCallBacker).CallBack(CallBackContext{
		CallBackURL: "local-callback", Events: []*models.AlertCurEvent{event}, BatchSend: true,
	})

	require.Equal(t, 1, queue.eventQueue.Len())
	require.Same(t, event, queue.eventQueue.Pop())
}

func TestPushNotifyRecords_ReportsFullQueue(t *testing.T) {
	isolatedNotifyQueue(t)
	NotifyRecordQueue = list.NewSafeListLimited(1)
	first := &models.NotificationRecord{EventId: 1}
	second := &models.NotificationRecord{EventId: 2}

	err := PushNotifyRecords([]*models.NotificationRecord{first, second})

	require.ErrorContains(t, err, "queue is full")
	require.Equal(t, 1, NotifyRecordQueue.Len())
	require.Same(t, first, NotifyRecordQueue.PopBack())
}

func TestStaticGlobalWebhook_RecordsHTTPResult(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			ctx := isolatedNotifyQueue(t)
			server, requests := newMessageServer(t, status)
			previousConf, previousClient := staticGlobalWebhookConf, staticGlobalWebhookClient
			t.Cleanup(func() {
				staticGlobalWebhookClient.CloseIdleConnections()
				staticGlobalWebhookConf, staticGlobalWebhookClient = previousConf, previousClient
			})
			InitStaticGlobalWebhook(aconf.GlobalWebhook{
				Enable: true, Url: server.URL + "/global", BasicAuthUser: "user", BasicAuthPass: "secret",
				Headers: []string{"host", "global.internal", "X-Tenant", "global"},
			})

			SendStaticGlobalWebhook(ctx, messageEvents()[0], newStaticWebhookTestStats())

			request := takeMessageRequest(t, requests)
			require.Equal(t, "/global", request.uri)
			require.Equal(t, "global.internal", request.host)
			require.Equal(t, "global", request.header.Get("X-Tenant"))
			require.NotEmpty(t, request.header.Get("Authorization"))
			var event models.AlertCurEvent
			require.NoError(t, json.Unmarshal(request.body, &event))
			require.Equal(t, int64(101), event.Id)
			require.Equal(t, 1, NotifyRecordQueue.Len())
			record := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
			if status == http.StatusOK {
				require.Equal(t, models.NotiStatusSuccess, record.Status)
				require.Contains(t, record.Details, "status_code:200")
			} else {
				require.Equal(t, models.NotiStatusFailure, record.Status)
				require.Contains(t, record.Details, "503")
			}
		})
	}
}

func TestNewCallBacker_SelectsChannelTemplate(t *testing.T) {
	tpl := messageTemplate(t)
	got := NewCallBacker(models.FeishuDomain, nil, nil, nil, map[string]*template.Template{models.Feishu: tpl})

	sender, ok := got.(*FeishuSender)
	require.True(t, ok)
	require.Same(t, tpl, sender.tpl)
	require.Nil(t, NewCallBacker("unknown", nil, nil, nil, nil))
}
