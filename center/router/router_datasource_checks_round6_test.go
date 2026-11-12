package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDatasourceCheck_BuildsAuthenticatedProbeForEachPlugin(t *testing.T) {
	cases := []struct {
		name        string
		plugin      string
		basePath    string
		method      string
		path        string
		query       string
		body        string
		contentType string
	}{
		{name: "prometheus", plugin: models.PROMETHEUS, method: "GET", path: "/api/v1/query", query: "1+1"},
		{name: "loki", plugin: models.LOKI, basePath: "/loki", method: "GET", path: "/loki/api/v1/labels"},
		{
			name: "prometheus loki compatibility", plugin: models.PROMETHEUS, basePath: "/loki",
			method: "GET", path: "/loki/api/v1/labels",
		},
		{
			name: "tdengine", plugin: models.TDENGINE, method: "POST", path: "/rest/sql", body: "show databases",
			contentType: "application/x-www-form-urlencoded",
		},
		{
			name: "iotdb", plugin: models.IOTDB, method: "POST", path: "/rest/table/v1/query",
			body: `{"sql":"show databases"}`, contentType: "application/json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan *http.Request, 1)
			bodies := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				requests <- r.Clone(r.Context())
				bodies <- string(b)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(server.Close)
			ds := models.Datasource{
				PluginType: tc.plugin,
				HTTPJson:   models.HTTP{Url: server.URL + tc.basePath + "/", Headers: map[string]string{"X-Tenant": "test"}},
				AuthJson:   models.Auth{BasicAuthUser: "reader", BasicAuthPassword: "secret"},
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

			err := DatasourceCheck(c, ds)

			require.NoError(t, err)
			received := <-requests
			require.Equal(t, tc.method, received.Method)
			require.Equal(t, tc.path, received.URL.Path)
			require.Equal(t, tc.query, received.URL.Query().Get("query"))
			require.Equal(t, tc.contentType, received.Header.Get("Content-Type"))
			require.Equal(t, "test", received.Header.Get("X-Tenant"))
			user, password, ok := received.BasicAuth()
			require.True(t, ok)
			require.Equal(t, "reader", user)
			require.Equal(t, "secret", password)
			require.Equal(t, tc.body, <-bodies)
		})
	}
}

func TestDatasourceCheck_RejectsInvalidConnectionConfiguration(t *testing.T) {
	cases := []struct {
		name    string
		ds      models.Datasource
		message string
	}{
		{name: "empty url", ds: models.Datasource{PluginType: models.PROMETHEUS}, message: "url is empty"},
		{
			name: "wrong scheme", ds: models.Datasource{PluginType: models.LOKI, HTTPJson: models.HTTP{Url: "ftp://localhost"}},
			message: "url must start with http or https",
		},
		{
			name: "malformed url", ds: models.Datasource{PluginType: models.TDENGINE, HTTPJson: models.HTTP{Url: "http://["}},
			message: "failed",
		},
		{
			name: "invalid TLS", ds: models.Datasource{PluginType: models.IOTDB,
				HTTPJson: models.HTTP{Url: "http://127.0.0.1", TLS: models.TLS{CACert: "invalid"}}},
			message: "failed to create TLS config",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			err := DatasourceCheck(c, tc.ds)

			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestDatasourceCheck_ReportsUpstreamFailureAndLokiSuffixHint(t *testing.T) {
	cases := []struct {
		name    string
		plugin  string
		path    string
		code    int
		message string
	}{
		{name: "body", plugin: models.PROMETHEUS, code: 503, message: "failed code:503 body:maintenance"},
		{name: "missing loki suffix", plugin: models.LOKI, code: 404, message: "/loki suffix is miss"},
		{name: "loki with suffix", plugin: models.LOKI, path: "/loki", code: 404, message: "failed code:404 body:maintenance"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = io.WriteString(w, "maintenance")
			}))
			t.Cleanup(server.Close)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			c.Request.Header.Set("X-Language", "en_US")

			err := DatasourceCheck(c, models.Datasource{PluginType: tc.plugin, HTTPJson: models.HTTP{Url: server.URL + tc.path}})

			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestDatasourceUpsert_FailedProbeDoesNotPersistDatasource(t *testing.T) {
	rt := identityRouter(t, &models.Datasource{})
	rt.DatasourceCache = &memsto.DatasourceCacheType{DatasourceCheckHook: func(*gin.Context) bool { return false }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	form := models.Datasource{Name: "broken", PluginType: models.PROMETHEUS, HTTPJson: models.HTTP{Url: server.URL}}

	w := identityRequest(t, rt, rt.datasourceUpsert, form, "0", &models.User{Username: "alice"})

	require.Contains(t, w.Body.String(), "failed code:503")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Datasource{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestDatasourceUpsert_DeniedHookPreventsProbeAndWrite(t *testing.T) {
	rt := identityRouter(t, &models.Datasource{})
	rt.DatasourceCache = &memsto.DatasourceCacheType{DatasourceCheckHook: func(*gin.Context) bool { return true }}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	t.Cleanup(server.Close)

	w := identityRequest(t, rt, rt.datasourceUpsert, models.Datasource{
		Name: "denied", PluginType: models.PROMETHEUS, HTTPJson: models.HTTP{Url: server.URL},
	}, "0", &models.User{Username: "alice"})

	require.JSONEq(t, `{"data":[],"error":""}`, w.Body.String())
	require.Zero(t, calls.Load())
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Datasource{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestGetElasticsearchVersion_FallsBackToWorkingURLWithCredentials(t *testing.T) {
	paths := make(chan string, 3)
	auth := make(chan string, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		auth <- r.Header.Get("Authorization") + "|" + r.Header.Get("X-Tenant")
		switch r.URL.Path {
		case "/unavailable/":
			http.Error(w, "unavailable", 503)
		case "/invalid/":
			_, _ = io.WriteString(w, "invalid JSON")
		default:
			_, _ = io.WriteString(w, `{"version":{"number":"8.12.1"}}`)
		}
	}))
	t.Cleanup(server.Close)
	ds := models.Datasource{
		HTTPJson: models.HTTP{
			Urls: []string{server.URL + "/unavailable", server.URL + "/invalid"}, Url: server.URL + "/healthy///",
			Headers: map[string]string{"X-Tenant": "one"},
		},
		AuthJson: models.Auth{BasicAuthUser: "a", BasicAuthPassword: "b"},
	}

	got, err := getElasticsearchVersion(ds, time.Second)

	require.NoError(t, err)
	require.Equal(t, "8.12.1", got)
	require.Equal(t, "/unavailable/", <-paths)
	require.Equal(t, "/invalid/", <-paths)
	require.Equal(t, "/healthy/", <-paths)
	for i := 0; i < 3; i++ {
		require.Equal(t, "Basic YTpi|one", <-auth)
	}
}

func TestGetElasticsearchVersion_ReportsMissingOrInvalidVersion(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "missing", body: `{}`},
		{name: "empty", body: `{"version":{"number":""}}`},
		{name: "wrong type", body: `{"version":{"number":8}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, tc.body) }))
			t.Cleanup(server.Close)

			got, err := getElasticsearchVersion(models.Datasource{HTTPJson: models.HTTP{Url: server.URL}}, time.Second)

			require.Empty(t, got)
			require.ErrorContains(t, err, "version not found in response")
		})
	}
}

func TestGetElasticsearchVersion_RejectsAbsentAndMalformedURLs(t *testing.T) {
	cases := []struct {
		name    string
		urls    []string
		message string
	}{
		{name: "absent", message: "no url provided"},
		{name: "malformed", urls: []string{"http://["}, message: "missing ']'"},
		{name: "unsupported scheme", urls: []string{"invalid://localhost"}, message: "unsupported protocol scheme"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := getElasticsearchVersion(models.Datasource{HTTPJson: models.HTTP{Urls: tc.urls}}, time.Second)

			require.Empty(t, got)
			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestDatasourceUpsert_ProbeErrorRedactsEmbeddedURLCredentials(t *testing.T) {
	rt := identityRouter(t, &models.Datasource{})
	rt.DatasourceCache = &memsto.DatasourceCacheType{DatasourceCheckHook: func(*gin.Context) bool { return false }}
	// The malformed URL fails before any connection, but still contains credentials in the client error.
	form := models.Datasource{
		Name: "broken", PluginType: models.PROMETHEUS,
		HTTPJson: models.HTTP{Url: "http://reader:secret@127.0.0.1:invalid"},
	}

	w := identityRequest(t, rt, rt.datasourceUpsert, form, "0", &models.User{Username: "alice"})

	require.NotContains(t, w.Body.String(), "reader:secret")
	require.Contains(t, w.Body.String(), "***:***")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Datasource{}).Count(&count).Error)
	require.Zero(t, count)
}
