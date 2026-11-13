package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/pstat"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func round4RouteRouter(t *testing.T) *Router {
	t.Helper()
	previous := pstat.RequestDuration
	pstat.RequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "round4_requests"},
		[]string{"service", "code", "path", "method"})
	t.Cleanup(func() { pstat.RequestDuration = previous })
	rt, _ := ingestionRouter(t)
	rt.Ctx = &ctx.Context{}
	return rt
}

func TestRouterConfig_DisablesAgentRoutes(t *testing.T) {
	rt := round4RouteRouter(t)
	engine := gin.New()
	rt.Config(engine)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/opentsdb/put", nil))

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestRouterConfig_ProtectsServiceRoutes(t *testing.T) {
	rt := round4RouteRouter(t)
	rt.HTTP.APIForService.BasicAuth = gin.Accounts{"service": "secret"}
	engine := gin.New()
	rt.Config(engine)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/n9e/target-update", nil))

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestRouterConfig_AgentCredentialsAcceptServiceAccounts(t *testing.T) {
	rt := round4RouteRouter(t)
	rt.HTTP.APIForAgent.Enable = true
	rt.HTTP.APIForAgent.BasicAuth = gin.Accounts{"agent": "agent-secret"}
	rt.HTTP.APIForService.BasicAuth = gin.Accounts{"service": "service-secret"}
	engine := gin.New()
	rt.Config(engine)
	for _, username := range []string{"agent", "service", "invalid"} {
		t.Run(username, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/opentsdb/put", bytes.NewBufferString("[]"))
			req.SetBasicAuth(username, username+"-secret")
			recorder := httptest.NewRecorder()

			engine.ServeHTTP(recorder, req)

			if username == "invalid" {
				require.Equal(t, http.StatusUnauthorized, recorder.Code)
			} else {
				requireIngestCounts(t, recorder, 0, 0)
			}
		})
	}
}

func TestRouterConfig_DatadogCompatibilityEndpoints(t *testing.T) {
	rt := round4RouteRouter(t)
	rt.HTTP.APIForAgent.Enable = true
	engine := gin.New()
	rt.Config(engine)
	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "validation", method: http.MethodGet, path: "/datadog/api/v1/validate", body: "not implemented"},
		{name: "checks", method: http.MethodPost, path: "/datadog/api/v1/check_run", body: "not implemented"},
		{name: "metadata", method: http.MethodPost, path: "/datadog/api/v1/metadata", body: "not implemented"},
		{name: "intake", method: http.MethodPost, path: "/datadog/intake/", body: "not implemented"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()

			engine.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.path, nil))

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, tc.body, recorder.Body.String())
		})
	}
}

func TestHeartbeat_RejectsMissingHostnameThroughGin(t *testing.T) {
	rt := round4RouteRouter(t)
	rt.HTTP.APIForAgent.Enable = true
	engine := gin.New()
	engine.Use(aop.RecoveryWithWriter(nil))
	rt.Config(engine)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/n9e/edge/heartbeat", bytes.NewBufferString(`{"cpu_num":4}`))
	req.Header.Set("Content-Type", "application/json")

	engine.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "hostname is required")
}

func TestHeartbeat_RejectsMalformedJSONThroughGin(t *testing.T) {
	rt := round4RouteRouter(t)
	rt.HTTP.APIForAgent.Enable = true
	engine := gin.New()
	engine.Use(aop.RecoveryWithWriter(nil))
	rt.Config(engine)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/n9e/edge/heartbeat", bytes.NewBufferString("{"))

	engine.ServeHTTP(recorder, req)

	require.Contains(t, recorder.Body.String(), "unexpected end of JSON input")
}

func TestNewRouter_PrecompilesConfiguredDropFilters(t *testing.T) {
	rt := New(httpx.Config{}, pconf.Pushgw{DropSample: []map[string]string{
		{"__name__": "discard"}, {"env": "dev"},
	}}, aconf.Alert{}, nil, nil, nil, nil, nil, &ctx.Context{})

	require.Contains(t, rt.dropByNameOnly, "discard")
	require.Equal(t, []map[string]string{{"env": "dev"}}, rt.dropComplex)
}

func TestHTTPMetric_JSONRoundTripPreservesPayload(t *testing.T) {
	input := `{"metric":"cpu","timestamp":1700000000,"value":12.5,"tags":{"host":"node","env":"prod"}}`
	var metric HTTPMetric
	require.NoError(t, json.Unmarshal([]byte(input), &metric))

	encoded, err := json.Marshal(metric)

	require.NoError(t, err)
	require.JSONEq(t, input, string(encoded))
}

func TestFalconMetric_JSONRoundTripPreservesPayload(t *testing.T) {
	input := `{"metric":"cpu","endpoint":"node","timestamp":1700000000,"value":12.5,` +
		`"tags":"env=prod"}`
	var metric FalconMetric
	require.NoError(t, json.Unmarshal([]byte(input), &metric))

	encoded, err := json.Marshal(metric)

	require.NoError(t, err)
	require.JSONEq(t, input, string(encoded))
}

func TestTimeSeries_JSONRoundTripPreservesPayload(t *testing.T) {
	input := `{"series":[{"metric":"cpu","points":[[1700000000,12.5]],"host":"node",` +
		`"tags":["env:prod"]}]}`
	var request TimeSeries
	require.NoError(t, json.Unmarshal([]byte(input), &request))

	encoded, err := json.Marshal(request)

	require.NoError(t, err)
	require.JSONEq(t, input, string(encoded))
}
