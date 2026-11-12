package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type channelRound7Transport struct {
	target    *url.URL
	transport http.RoundTripper
}

func (f channelRound7Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != "api.pagerduty.com" {
		return nil, fmt.Errorf("unexpected external host: %s", r.URL.Host)
	}
	cloned := r.Clone(r.Context())
	cloned.URL.Scheme, cloned.URL.Host = f.target.Scheme, f.target.Host
	cloned.Host = f.target.Host
	return f.transport.RoundTrip(cloned)
}

func pagerRound7Server(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	original := http.DefaultTransport
	http.DefaultTransport = channelRound7Transport{target: target, transport: server.Client().Transport}
	t.Cleanup(func() { http.DefaultTransport = original })
}

func pagerRound7Channel(t *testing.T, rt *Router) {
	t.Helper()
	require.NoError(t, rt.Ctx.DB.Create(&models.NotifyChannelConfig{
		ID: 1, Name: "pager", Ident: "pager", RequestType: "pagerduty",
		RequestConfig: &models.RequestConfig{PagerDutyRequestConfig: &models.PagerDutyRequestConfig{
			ApiKey: "fake-api-key", Timeout: 1000,
		}},
	}).Error)
}

func TestPagerDutyServices_PaginatesAndFlattensIntegrations(t *testing.T) {
	requests := make(chan *http.Request, 2)
	pagerRound7Server(t, func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		if r.URL.Query().Get("offset") == "0" {
			services := make([]PagerDutyService, 100)
			services[0] = PagerDutyService{ID: "svc-1", Name: "Primary", Integrations: []PagerDutyIntegration{
				{ID: "int-1", Summary: "Events API", Self: "https://api.pagerduty.com/integrations/int-1"},
				{ID: "int-2", Summary: "Email", Self: "https://api.pagerduty.com/integrations/int-2"},
			}}
			if err := json.NewEncoder(w).Encode(map[string]interface{}{"services": services, "more": true}); err != nil {
				t.Error(err)
			}
		} else {
			_, _ = io.WriteString(w, `{"services":[{"id":"svc-2","name":"Backup","integrations":[`+
				`{"id":"int-3","summary":"Webhook","self":"https://api.pagerduty.com/integrations/int-3"}]}],"more":false}`)
		}
	})
	rt := channelBehaviorRouter(t)
	pagerRound7Channel(t, rt)

	w := channelBehaviorRequest(t, rt.pagerDutyNotifyServicesGet, nil, "")

	var items []map[string]string
	require.Empty(t, channelBehaviorReply(t, w, &items))
	require.Equal(t, []map[string]string{
		{"service_id": "svc-1", "service_name": "Primary", "integration_id": "int-1",
			"integration_summary": "Events API", "integration_url": "https://api.pagerduty.com/integrations/int-1"},
		{"service_id": "svc-1", "service_name": "Primary", "integration_id": "int-2",
			"integration_summary": "Email", "integration_url": "https://api.pagerduty.com/integrations/int-2"},
		{"service_id": "svc-2", "service_name": "Backup", "integration_id": "int-3",
			"integration_summary": "Webhook", "integration_url": "https://api.pagerduty.com/integrations/int-3"},
	}, items)
	require.Len(t, requests, 2)
	for _, offset := range []string{"0", "100"} {
		request := <-requests
		require.Equal(t, http.MethodGet, request.Method)
		require.Equal(t, "/services", request.URL.Path)
		require.Equal(t, "100", request.URL.Query().Get("limit"))
		require.Equal(t, offset, request.URL.Query().Get("offset"))
		require.Equal(t, "Token token=fake-api-key", request.Header.Get("Authorization"))
		require.Equal(t, "application/vnd.pagerduty+json;version=2", request.Header.Get("Accept"))
	}
	require.Empty(t, requests)
}

func TestPagerDutyIntegrationKey_UsesRouteIDsAndConfiguredCredentials(t *testing.T) {
	requests := make(chan *http.Request, 1)
	pagerRound7Server(t, func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		_, _ = io.WriteString(w, `{"integration":{"integration_key":"fake-routing-key"}}`)
	})
	rt := channelBehaviorRouter(t)
	pagerRound7Channel(t, rt)
	handler := func(c *gin.Context) {
		c.Params = append(c.Params,
			gin.Param{Key: "service_id", Value: "svc-1"},
			gin.Param{Key: "integration_id", Value: "int-1"},
		)
		rt.pagerDutyIntegrationKeyGet(c)
	}

	w := channelBehaviorRequest(t, handler, nil, "")

	var result map[string]string
	require.Empty(t, channelBehaviorReply(t, w, &result))
	require.Equal(t, map[string]string{"integration_key": "fake-routing-key"}, result)
	require.Len(t, requests, 1)
	request := <-requests
	require.Equal(t, http.MethodGet, request.Method)
	require.Equal(t, "/services/svc-1/integrations/int-1", request.URL.Path)
	require.Equal(t, "Token token=fake-api-key", request.Header.Get("Authorization"))
}

func TestPagerDutyCatalog_ReportsInvalidUpstreamJSON(t *testing.T) {
	pagerRound7Server(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "invalid JSON") })
	rt := channelBehaviorRouter(t)
	pagerRound7Channel(t, rt)
	cases := []struct {
		name    string
		handler gin.HandlerFunc
		message string
	}{
		{name: "services", handler: rt.pagerDutyNotifyServicesGet, message: "failed to get pagerduty services"},
		{name: "integration", handler: pagerRound7IntegrationHandler(rt), message: "failed to get pagerduty integration key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := channelBehaviorRequest(t, tc.handler, nil, "")

			require.Equal(t, http.StatusInternalServerError, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
			require.NotContains(t, w.Body.String(), "fake-api-key")
		})
	}
}

func TestPagerDutyCatalog_RejectsMissingChannel(t *testing.T) {
	rt := channelBehaviorRouter(t)
	for _, handler := range []gin.HandlerFunc{rt.pagerDutyNotifyServicesGet, pagerRound7IntegrationHandler(rt)} {
		w := channelBehaviorRequest(t, handler, nil, "")

		require.Equal(t, http.StatusNotFound, w.Code)
		require.Contains(t, w.Body.String(), "notify channel not found")
	}
}

func TestFlashDutyChannels_PostsIdentityOnlyWhenAppKeyConfigured(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(fmt.Sprint(configured), func(t *testing.T) {
			requests := make(chan *http.Request, 1)
			bodies := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				requests <- r.Clone(r.Context())
				bodies <- string(body)
				_, _ = io.WriteString(w, `{"data":{"items":[{"channel_id":7,"channel_name":"Oncall","status":"enabled"}],"total":1}}`)
			}))
			t.Cleanup(server.Close)
			rt := channelBehaviorRouter(t, &models.Configs{})
			if configured {
				require.NoError(t, rt.Ctx.DB.Create(&models.Configs{Ckey: "flashduty_app_key", Cval: "fake-key"}).Error)
			}
			channel := models.NotifyChannelConfig{ID: 1, RequestType: "flashduty",
				RequestConfig: &models.RequestConfig{FlashDutyRequestConfig: &models.FlashDutyRequestConfig{
					IntegrationUrl: server.URL + "/event/push/alert?integration_key=fake-integration", Timeout: 1000,
				}}}
			require.NoError(t, rt.Ctx.DB.Create(&channel).Error)

			w := channelBehaviorRequest(t, rt.flashDutyNotifyChannelsGet, nil, "")

			var channels []FlashDutyChannel
			require.Empty(t, channelBehaviorReply(t, w, &channels))
			require.Equal(t, []FlashDutyChannel{{ChannelID: 7, ChannelName: "Oncall", Status: "enabled"}}, channels)
			request := <-requests
			require.Equal(t, http.MethodPost, request.Method)
			require.Equal(t, "/channel/list-by-integration", request.URL.Path)
			require.Equal(t, "fake-integration", request.URL.Query().Get("integration_key"))
			require.Equal(t, "application/json", request.Header.Get("Content-Type"))
			if configured {
				require.JSONEq(t, `{"member_name":"editor","email":"","phone":""}`, <-bodies)
			} else {
				require.JSONEq(t, `{}`, <-bodies)
			}
		})
	}
}

func TestFlashDutyChannels_ReportsUpstreamBusinessAndJSONErrors(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{
			name: "business", body: `{"error":{"code":"Denied","message":"integration disabled"}}`,
			message: "integration disabled",
		},
		{name: "json", body: `not JSON`, message: "invalid character"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(server.Close)

			channels, err := getFlashDutyChannels(server.URL+"/push?integration_key=fake", []byte(`{}`), 0)

			require.Nil(t, channels)
			require.ErrorContains(t, err, tc.message)
		})
	}
}

func pagerRound7IntegrationHandler(rt *Router) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Params = append(c.Params,
			gin.Param{Key: "service_id", Value: "svc-1"},
			gin.Param{Key: "integration_id", Value: "int-1"},
		)
		rt.pagerDutyIntegrationKeyGet(c)
	}
}
