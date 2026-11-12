package router

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func draftRound7Form(endpoint string) NotifyChannelTestForm {
	channel := validBehaviorChannel()
	channel.Enable = false
	channel.RequestConfig.HTTPRequestConfig.URL = endpoint
	channel.RequestConfig.HTTPRequestConfig.Timeout = 1000
	channel.RequestConfig.HTTPRequestConfig.Headers = map[string]string{"Content-Type": "application/json"}
	channel.RequestConfig.HTTPRequestConfig.Request = models.RequestDetail{
		Body: `{"text":"{{$tpl.content}}","room":"{{$params.room}}","recovered":{{$event.IsRecovered}}}`,
	}
	return NotifyChannelTestForm{
		Config: channel, NotifyConfig: models.NotifyConfig{Params: map[string]interface{}{"room": "oncall"}},
		TplContent:    map[string]string{"content": "S{{$event.Severity}} {{$event.RuleName}}"},
		MockEventForm: MockEventForm{UseMockEvent: true, MockSeverity: 2},
	}
}

func TestNotifyChannelConfigTest_DraftSendsRenderedFiringAndRecoveryRequests(t *testing.T) {
	type payload struct {
		Text      string `json:"text"`
		Room      string `json:"room"`
		Recovered bool   `json:"recovered"`
	}
	received := make(chan payload, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body payload
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing content type")
		}
		received <- body
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)
	rt := identityRouter(t, &models.Configs{})
	form := draftRound7Form(server.URL)
	form.WithRecovery = true

	w := channelBehaviorRequest(t, rt.notifyChannelConfigTest, form, "")

	var result notifyChannelTestResult
	require.Empty(t, channelBehaviorReply(t, w, &result))
	require.True(t, result.Success)
	require.Empty(t, result.ErrorMessage)
	require.Empty(t, result.Detail)
	require.Len(t, received, 2)
	expectedText := "S2 " + buildChannelTestMockEvent("", form.MockEventForm).RuleName
	require.Equal(t, payload{Text: expectedText, Room: "oncall"}, <-received)
	require.Equal(t, payload{Text: expectedText, Room: "oncall", Recovered: true}, <-received)
	require.Empty(t, received)
	require.NotContains(t, w.Body.String(), "oncall")
}

func TestNotifyChannelConfigTest_SavedEventSuppliesTagsAndRuleToRequest(t *testing.T) {
	received := make(chan []models.AlertCurEvent, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []models.AlertCurEvent
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
			t.Error(err)
		}
		received <- events
	}))
	t.Cleanup(server.Close)
	rt := identityRouter(t, &models.Configs{}, &models.AlertHisEvent{})
	event := models.AlertHisEvent{Id: 42, RuleName: "CPU high", Severity: 1, Tags: "env=prod,,host=web-1"}
	require.NoError(t, rt.Ctx.DB.Create(&event).Error)
	form := draftRound7Form(server.URL)
	form.UseMockEvent = false
	form.EventIDs = []int64{42}
	form.TplContent = nil
	form.Config.RequestConfig.HTTPRequestConfig.Request.Body = `{{jsonMarshal $events}}`

	w := channelBehaviorRequest(t, rt.notifyChannelConfigTest, form, "")

	var result notifyChannelTestResult
	require.Empty(t, channelBehaviorReply(t, w, &result))
	require.True(t, result.Success)
	require.Len(t, received, 1)
	events := <-received
	require.Len(t, events, 1)
	require.Equal(t, int64(42), events[0].Id)
	require.Equal(t, event.RuleName, events[0].RuleName)
	require.Equal(t, map[string]string{"env": "prod", "host": "web-1"}, events[0].TagsMap)
	var saved models.AlertHisEvent
	require.NoError(t, rt.Ctx.DB.First(&saved, 42).Error)
	require.Equal(t, event.Tags, saved.Tags)
}

func TestNotifyChannelConfigTest_TemplateFailurePreventsDelivery(t *testing.T) {
	requests := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests <- struct{}{} }))
	t.Cleanup(server.Close)
	rt := identityRouter(t, &models.Configs{})
	form := draftRound7Form(server.URL)
	form.TplContent = map[string]string{"content": "{{$undefined}}"}

	w := channelBehaviorRequest(t, rt.notifyChannelConfigTest, form, "")

	var result notifyChannelTestResult
	require.Empty(t, channelBehaviorReply(t, w, &result))
	require.False(t, result.Success)
	require.Contains(t, result.ErrorMessage, "undefined variable")
	require.Empty(t, requests)
}

func TestNotifyChannelConfigTest_DeliveryFailureReturnsBusinessResult(t *testing.T) {
	requests := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		http.Error(w, "local delivery rejected", http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)
	rt := identityRouter(t, &models.Configs{})
	form := draftRound7Form(server.URL)
	form.WithRecovery = true

	w := channelBehaviorRequest(t, rt.notifyChannelConfigTest, form, "")

	var result notifyChannelTestResult
	require.Empty(t, channelBehaviorReply(t, w, &result))
	require.False(t, result.Success)
	require.Contains(t, result.ErrorMessage, "local delivery rejected")
	require.Len(t, requests, 1) // A failed firing request must stop the recovery round.
	require.NotContains(t, w.Body.String(), "Authorization")
}

func TestNotifyChannelConfigTest_InvalidDraftDoesNotDeliver(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*NotifyChannelTestForm)
		message string
	}{
		{name: "script", mutate: func(f *NotifyChannelTestForm) { f.Config.RequestType = "script" },
			message: "script media type must be saved before testing"},
		{
			name: "name", mutate: func(f *NotifyChannelTestForm) { f.Config.Name = "" },
			message: "channel name cannot be empty",
		},
		{name: "http config", mutate: func(f *NotifyChannelTestForm) { f.Config.RequestConfig.HTTPRequestConfig = nil },
			message: "http request config cannot be nil"},
		{name: "no events", mutate: func(f *NotifyChannelTestForm) { f.UseMockEvent = false },
			message: "event_ids or use_mock_event required"},
		{name: "unknown event", mutate: func(f *NotifyChannelTestForm) { f.UseMockEvent = false; f.EventIDs = []int64{404} },
			message: "event not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.AlertHisEvent{})
			requests := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- struct{}{}
			}))
			t.Cleanup(server.Close)
			form := draftRound7Form(server.URL)
			tc.mutate(&form)

			w := channelBehaviorRequest(t, rt.notifyChannelConfigTest, form, "")

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
			require.Empty(t, requests)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.AlertHisEvent{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}
