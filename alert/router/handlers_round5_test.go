package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/naming"
	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/toolkits/pkg/consistent"
	"github.com/toolkits/pkg/container/list"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func round5Router(t *testing.T) (*Router, *gin.Engine) {
	t.Helper()
	rt := New(httpx.Config{Port: 19000, APIForService: httpx.BasicAuths{Enable: true}}, aconf.Alert{},
		&memsto.AlertMuteCacheType{}, nil, nil, nil, nil, process.NewExternalProcessors(), t.TempDir())
	rt.Alert.Heartbeat.IP = "127.0.0.1"
	engine := gin.New()
	engine.Use(aop.RecoveryWithWriter(io.Discard))
	return rt, engine
}

func round5Request(t *testing.T, engine *gin.Engine, method, path string, body interface{}) map[string]interface{} {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func round5EventQueue(t *testing.T, capacity int) {
	t.Helper()
	old := queue.EventQueue
	queue.EventQueue = list.NewSafeListLimited(capacity)
	t.Cleanup(func() { queue.EventQueue = old })
}

func TestRouterConfig_DisabledServiceDoesNotRegisterRoutes(t *testing.T) {
	rt, engine := round5Router(t)
	rt.HTTP.APIForService.Enable = false

	rt.Config(engine)

	require.Empty(t, engine.Routes())
}

func TestRouterConfig_BasicAuthProtectsServiceRoutes(t *testing.T) {
	rt, engine := round5Router(t)
	rt.HTTP.APIForService.BasicAuth = gin.Accounts{"engine": "secret"}
	rt.Config(engine)
	req := httptest.NewRequest(http.MethodGet, "/v1/n9e/event-detail/"+strings.Repeat("a", 32), nil)
	denied := httptest.NewRecorder()

	engine.ServeHTTP(denied, req)

	require.Equal(t, http.StatusUnauthorized, denied.Code)
	require.Contains(t, denied.Header().Get("WWW-Authenticate"), "Basic")
	req.SetBasicAuth("engine", "secret")
	allowed := httptest.NewRecorder()
	engine.ServeHTTP(allowed, req)
	require.Equal(t, http.StatusOK, allowed.Code)
	require.Contains(t, allowed.Body.String(), `"instance":"127.0.0.1:19000"`)
	require.Len(t, engine.Routes(), 7)
}

func TestLogDetailHandlers_ReturnMatchingLogsAndInstance(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		line    string
		rotated bool
	}{
		{name: "event hash", path: "/event-detail/" + strings.Repeat("a", 32), line: strings.Repeat("a", 32), rotated: true},
		{name: "rule evaluation", path: "/alert-eval-detail/42", line: "alert_eval_42 finished", rotated: true},
		{name: "trace", path: "/trace-logs/trace_42", line: "trace_id=trace_42 request completed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, engine := round5Router(t)
			rt.Config(engine)
			require.NoError(t, os.WriteFile(filepath.Join(rt.LogDir, "INFO.log"), []byte("unrelated\n"+tt.line+"\n"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(rt.LogDir, "INFO.log.1"), []byte(tt.line+" rotated\n"), 0600))

			response := round5Request(t, engine, http.MethodGet, "/v1/n9e"+tt.path, nil)

			require.Equal(t, "", response["err"])
			data := response["dat"].(map[string]interface{})
			require.Equal(t, "127.0.0.1:19000", data["instance"])
			require.Equal(t, false, data["truncated"])
			want := []interface{}{tt.line}
			if tt.rotated {
				want = append(want, tt.line+" rotated")
			}
			require.ElementsMatch(t, want, data["logs"])
		})
	}
}

func TestLogDetailHandlers_RejectInvalidIdentifiers(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		message string
	}{
		{name: "hash", path: "/event-detail/not-a-hash", message: "invalid hash format"},
		{name: "rule", path: "/alert-eval-detail/0", message: "invalid rule id format"},
		{name: "trace", path: "/trace-logs/bad!trace", message: "invalid trace id format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, engine := round5Router(t)
			rt.Config(engine)

			response := round5Request(t, engine, http.MethodGet, "/v1/n9e"+tt.path, nil)

			require.Equal(t, tt.message, response["err"])
			require.NotContains(t, response, "dat")
		})
	}
}

func TestLogGrepOptions_ClampsTimeoutAndParsesSince(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		timeout time.Duration
		since   time.Time
	}{
		{name: "default", timeout: loggrep.DefaultTimeout},
		{
			name: "custom", query: "?timeout=1250&since=1700000000",
			timeout: 1250 * time.Millisecond, since: time.Unix(1700000000, 0),
		},
		{name: "negative", query: "?timeout=-1&since=-1", timeout: loggrep.DefaultTimeout},
		{name: "too large", query: "?timeout=60001", timeout: loggrep.DefaultTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/"+tt.query, nil)
			before := time.Now()

			got := logGrepOptions(c)

			after := time.Now()
			require.False(t, got.Deadline.Before(before.Add(tt.timeout)))
			require.False(t, got.Deadline.After(after.Add(tt.timeout)))
			require.Equal(t, tt.since, got.Since)
		})
	}
}

func TestEvalRecordsGet_DisabledLoggingReturnsEmptyList(t *testing.T) {
	rt, engine := round5Router(t)
	rt.Config(engine)

	response := round5Request(t, engine, http.MethodGet,
		"/v1/n9e/eval-records?rule_id=42&from=-1&to=1700000000&limit=999999", nil)

	require.Equal(t, "", response["err"])
	require.Equal(t, map[string]interface{}{"enabled": false, "list": []interface{}{}}, response["dat"])
}

func TestPushEventToQueue_RendersTemplatesAndNormalizesFields(t *testing.T) {
	rt, engine := round5Router(t)
	rt.Config(engine)
	round5EventQueue(t, 2)
	event := &models.AlertCurEvent{
		RuleId: 42, Hash: "event", RuleName: "{{ $labels.host }} is down", RuleNote: "value={{ $value }}", TriggerValue: "5",
		TagsJSON:      []string{" host=node-1 ", "", "invalid", "expr=a=b"},
		CallbacksJSON: []string{"callback-a", "callback-b"}, NotifyChannelsJSON: []string{"email", "webhook"},
		NotifyGroupsJSON: []string{"1", "2"}, AnnotationsJSON: map[string]string{"summary": "{{ $labels.host }} failed"},
	}

	response := round5Request(t, engine, http.MethodPost, "/v1/n9e/event", event)

	require.Equal(t, "", response["err"])
	queued, ok := queue.EventQueue.PopBack().(*models.AlertCurEvent)
	require.True(t, ok)
	require.Equal(t, "node-1 is down", queued.RuleName)
	require.Equal(t, "value=5", queued.RuleNote)
	require.Equal(t, map[string]string{"host": "node-1", "expr": "a=b"}, queued.TagsMap)
	require.Equal(t, "node-1 failed", queued.AnnotationsJSON["summary"])
	require.Equal(t, strings.Join(event.TagsJSON, ",,"), queued.Tags)
	require.Equal(t, "callback-a callback-b", queued.Callbacks)
	require.Equal(t, "email webhook", queued.NotifyChannels)
	require.Equal(t, "1 2", queued.NotifyGroups)
}

func TestPushEventToQueue_RuleNoteCanReplaceTags(t *testing.T) {
	rt, engine := round5Router(t)
	rt.Config(engine)
	round5EventQueue(t, 1)

	response := round5Request(t, engine, http.MethodPost, "/v1/n9e/event",
		&models.AlertCurEvent{RuleId: 42, RuleNote: ";env=prod region=west", TagsJSON: []string{"original=value"}})

	require.Equal(t, "", response["err"])
	event := queue.EventQueue.PopBack().(*models.AlertCurEvent)
	require.Equal(t, "env=prod region=west", event.RuleNote)
	require.Equal(t, "env=prod,,region=west", event.Tags)
	require.Equal(t, []string{"env=prod", "region=west"}, event.TagsJSON)
}

func TestPushEventToQueue_ReportsTemplateErrors(t *testing.T) {
	rt, engine := round5Router(t)
	rt.Config(engine)
	round5EventQueue(t, 1)

	response := round5Request(t, engine, http.MethodPost, "/v1/n9e/event",
		&models.AlertCurEvent{RuleId: 42, RuleName: "{{", RuleNote: "{{"})

	require.Equal(t, "", response["err"])
	event := queue.EventQueue.PopBack().(*models.AlertCurEvent)
	require.Contains(t, event.RuleName, "failed to parse rule name:")
	require.Contains(t, event.RuleNote, "failed to parse rule note:")
}

func TestPushEventToQueue_RejectsMissingRuleID(t *testing.T) {
	rt, engine := round5Router(t)
	rt.Config(engine)
	round5EventQueue(t, 1)

	response := round5Request(t, engine, http.MethodPost, "/v1/n9e/event", &models.AlertCurEvent{})

	require.Equal(t, "event is illegal", response["err"])
	require.Zero(t, queue.EventQueue.Len())
}

func TestPushEventToQueue_ReportsFullQueue(t *testing.T) {
	rt, engine := round5Router(t)
	rt.Config(engine)
	round5EventQueue(t, 0)

	response := round5Request(t, engine, http.MethodPost, "/v1/n9e/event", &models.AlertCurEvent{RuleId: 42, Hash: "full"})

	require.Equal(t, "event:full push_queue err: queue is full", response["err"])
	require.Zero(t, queue.EventQueue.Len())
}

func round5PersistDB(t *testing.T, rt *Router) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.AlertCurEvent{}, &models.AlertHisEvent{}))
	rt.Ctx = ctx.NewContext(context.Background(), db, true)
	return db
}

func TestEventPersist_StoresNormalizedEventAndReturnsID(t *testing.T) {
	rt, engine := round5Router(t)
	rt.Config(engine)
	db := round5PersistDB(t, rt)
	event := &models.AlertCurEvent{RuleId: 42, Hash: "persistent", TagsJSON: []string{"env=prod"}}

	response := round5Request(t, engine, http.MethodPost, "/v1/n9e/event-persist", event)

	require.Equal(t, "", response["err"])
	id := int64(response["dat"].(float64))
	require.Positive(t, id)
	var current models.AlertCurEvent
	require.NoError(t, db.First(&current, id).Error)
	require.Equal(t, "persistent", current.Hash)
	require.Equal(t, "env=prod", current.Tags)
	var history models.AlertHisEvent
	require.NoError(t, db.First(&history, id).Error)
	require.Equal(t, current.Hash, history.Hash)
}

func TestEventPersist_ReportsDatabaseFailure(t *testing.T) {
	rt, engine := round5Router(t)
	rt.Config(engine)
	db := round5PersistDB(t, rt)
	require.NoError(t, db.Migrator().DropTable(&models.AlertCurEvent{}))

	response := round5Request(t, engine, http.MethodPost, "/v1/n9e/event-persist", &models.AlertCurEvent{RuleId: 42})

	require.Contains(t, response["err"], "event_persist_check_exists_fail")
}

func round5Ring(t *testing.T) {
	t.Helper()
	old := naming.DatasourceHashRing.Rings
	naming.DatasourceHashRing.Rings = make(map[string]*consistent.Consistent)
	t.Cleanup(func() { naming.DatasourceHashRing.Rings = old })
}

func TestForwardEvent_PostsEventArray(t *testing.T) {
	type forwardedRequest struct {
		events []*eventForm
		path   string
		method string
	}
	received := make(chan forwardedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []*eventForm
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
			t.Error(err)
		}
		received <- forwardedRequest{events: events, path: r.URL.Path, method: r.Method}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	input := &eventForm{RuleId: 42, DatasourceId: 7, Alert: true, Inhibit: true}

	err := forwardEvent(input, strings.TrimPrefix(server.URL, "http://"))

	require.NoError(t, err)
	got := <-received
	require.Equal(t, http.MethodPost, got.method)
	require.Equal(t, "/v1/n9e/make-event", got.path)
	require.Equal(t, []*eventForm{input}, got.events)
}

func TestMakeEvent_RejectsMissingOwner(t *testing.T) {
	rt, engine := round5Router(t)
	rt.Config(engine)
	round5Ring(t)

	response := round5Request(t, engine, http.MethodPost, "/v1/n9e/make-event",
		[]*eventForm{{RuleId: 42, DatasourceId: 7}})

	require.Equal(t, "event node not exists", response["err"])
}

func TestRender_EncodesDataAndErrors(t *testing.T) {
	tests := []struct {
		name    string
		data    interface{}
		message interface{}
		want    string
	}{
		{name: "empty data", want: `{"data":{},"error":""}`},
		{name: "data", data: []int{1, 2}, want: `{"data":[1,2],"error":""}`},
		{name: "error", message: "denied", want: `{"error":{"message":"denied"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)

			Render(c, tt.data, tt.message)

			require.Equal(t, http.StatusOK, recorder.Code)
			require.JSONEq(t, tt.want, recorder.Body.String())
		})
	}
}

func TestDangerous_EncodesNonemptyErrors(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  string
	}{
		{name: "nil"},
		{name: "empty string", value: ""},
		{name: "unsupported type", value: 42},
		{name: "string", value: "denied", want: `{"error":{"message":"denied"}}`},
		{name: "error", value: errors.New("failed"), want: `{"error":{"message":"failed"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)

			Dangerous(c, tt.value)

			if tt.want == "" {
				require.Empty(t, recorder.Body.String())
			} else {
				require.JSONEq(t, tt.want, recorder.Body.String())
			}
		})
	}
}

func TestPushEventToQueue_FullMuteDropsEvent(t *testing.T) {
	rt, engine := round5Router(t)
	rt.Config(engine)
	round5EventQueue(t, 1)
	rt.AlertMuteCache.Set(map[int64][]*models.AlertMute{5: {{
		Id: 9, GroupId: 5, Btime: 900, Etime: 1100, MuteTimeType: models.TimeRange, MuteType: models.MuteTypeAll,
	}}}, 1, 10)
	event := &models.AlertCurEvent{RuleId: 42, GroupId: 5, TriggerTime: 1000}

	response := round5Request(t, engine, http.MethodPost, "/v1/n9e/event", event)

	require.Equal(t, "", response["err"])
	require.Zero(t, queue.EventQueue.Len())
}

func TestPushEventToQueue_NotifyOnlyMuteMarksQueuedEvent(t *testing.T) {
	rt, engine := round5Router(t)
	rt.Config(engine)
	round5EventQueue(t, 1)
	rt.AlertMuteCache.Set(map[int64][]*models.AlertMute{5: {{
		Id: 9, GroupId: 5, Btime: 900, Etime: 1100, MuteTimeType: models.TimeRange, MuteType: models.MuteTypeNotifyOnly,
	}}}, 1, 10)
	event := &models.AlertCurEvent{RuleId: 42, GroupId: 5, TriggerTime: 1000}

	response := round5Request(t, engine, http.MethodPost, "/v1/n9e/event", event)

	require.Equal(t, "", response["err"])
	queued := queue.EventQueue.PopBack().(*models.AlertCurEvent)
	require.Equal(t, 1, queued.NotifyMuted)
	require.Equal(t, int64(9), queued.MuteId)
}
