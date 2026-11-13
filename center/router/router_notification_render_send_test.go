package router

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func notificationRenderContext(t *testing.T, tables ...interface{}) *ctx.Context {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(tables...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return &ctx.Context{DB: db, IsCenter: true, Ctx: context.Background()}
}

func TestSendNotifyChannelMessage_RendersSavedTemplateAndPostsConfiguredRequest(t *testing.T) {
	c := notificationRenderContext(t, &models.NotifyChannelConfig{}, &models.MessageTemplate{}, &models.Configs{})
	type received struct {
		method string
		path   string
		header string
		query  string
		body   string
	}
	requests := make(chan received, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- received{method: r.Method, path: r.URL.Path, header: r.Header.Get("X-Test"),
			query: r.URL.Query().Get("room"), body: string(body)}
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	t.Cleanup(server.Close)
	channel := models.NotifyChannelConfig{
		ID: 1, Name: "local callback", Ident: "callback", Enable: true, RequestType: "http",
		RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{
			URL: server.URL + "/notify", Method: "POST", Timeout: 5000,
			Headers: map[string]string{"Content-Type": "application/json", "X-Test": "configured-header"},
			Request: models.RequestDetail{
				Parameters: map[string]string{"room": "{{$params.room}}"},
				Body:       `{"message":"{{$tpl.content}}"}`,
			},
		}},
	}
	require.NoError(t, c.DB.Create(&channel).Error)
	template := models.MessageTemplate{
		ID: 2,
		Content: map[string]string{
			"content": "{{$event.RuleName}} S{{$event.Severity}}",
		},
	}
	require.NoError(t, c.DB.Create(&template).Error)
	event := &models.AlertCurEvent{RuleName: "CPU high", Severity: 2, Hash: "test"}

	response, err := SendNotifyChannelMessage(c, nil, nil,
		models.NotifyConfig{ChannelID: 1, TemplateID: 2, Params: map[string]interface{}{"room": "oncall"}},
		[]*models.AlertCurEvent{event})

	require.NoError(t, err)
	require.Contains(t, response, `{"accepted":true}`)
	got := <-requests
	require.Equal(t, "POST", got.method)
	require.Equal(t, "/notify", got.path)
	require.Equal(t, "configured-header", got.header)
	require.Equal(t, "oncall", got.query)
	require.JSONEq(t, `{"message":"CPU high S2"}`, got.body)
}

func TestSendNotifyChannelMessage_RejectsMissingDisabledAndUnconfiguredChannels(t *testing.T) {
	cases := []struct {
		name    string
		channel *models.NotifyChannelConfig
		message string
	}{
		{name: "missing channel", message: "notify channel not found"},
		{
			name: "disabled", channel: &models.NotifyChannelConfig{ID: 1, RequestType: "http", Enable: false},
			message: "not enabled",
		},
		{
			name: "missing template", channel: &models.NotifyChannelConfig{ID: 1, RequestType: "http", Enable: true},
			message: "message template not found",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := notificationRenderContext(t, &models.NotifyChannelConfig{}, &models.MessageTemplate{}, &models.Configs{})
			if tc.channel != nil {
				require.NoError(t, c.DB.Create(tc.channel).Error)
			}

			response, err := SendNotifyChannelMessage(c, nil, nil, models.NotifyConfig{ChannelID: 1, TemplateID: 2}, nil)

			require.ErrorContains(t, err, tc.message)
			require.Empty(t, response)
		})
	}
}

func renderNotificationRequest(t *testing.T, handler gin.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.POST("/preview", handler)
	req := httptest.NewRequest(http.MethodPost, "/preview", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestEventsMessage_RendersFieldsAndKeepsTemplateErrorsSeparate(t *testing.T) {
	c := notificationRenderContext(t, &models.Configs{})
	require.NoError(t, c.DB.Create(&models.Configs{Ckey: "site_info", Cval: `{"site_url":"http://center.invalid"}`}).Error)
	rt := &Router{Ctx: c}

	w := renderNotificationRequest(t, rt.eventsMessage, `{
  "use_mock_event":true,"mock_severity":1,
  "tpl":{"content":{"body":"S{{$event.Severity}} {{$.domain}}","broken":"{{if}}"}}
 }`)

	require.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Err string                        `json:"err"`
		Dat map[string]previewFieldResult `json:"dat"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Empty(t, response.Err)
	require.True(t, response.Dat["body"].Success)
	require.Equal(t, "S1 http://center.invalid", response.Dat["body"].Content)
	require.False(t, response.Dat["broken"].Success)
	require.NotEmpty(t, response.Dat["broken"].Message)
	require.Empty(t, response.Dat["broken"].Content)
}

func TestNotifyTest_RejectsMissingChannelAndEventSelection(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{name: "missing channel", body: `{"notify_config":{"template_id":2}}`, message: "channel_id required"},
		{name: "missing events", body: `{"notify_config":{"channel_id":1}}`, message: "event_ids or use_mock_event required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}

			w := renderNotificationRequest(t, rt.notifyTest, tc.body)

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
		})
	}
}
