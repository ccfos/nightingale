package router

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/grafana"
	pkgprom "github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestDatasourceGrafanaFetch_MapsSupportedSourcesAndMarksDuplicatesWithoutWriting(t *testing.T) {
	requests := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		_, _ = io.WriteString(w, `[
   {"name":"existing","type":"prometheus","url":"http://metrics.invalid","isDefault":true},
   {"name":"needs-auth","type":"prometheus","url":"http://private.invalid","basicAuth":true,"basicAuthUser":"reader"},
   {"name":"unsupported","type":"unknown","url":"http://other.invalid"}
  ]`)
	}))
	t.Cleanup(server.Close)
	rt := grafanaImportRouter(t)
	require.NoError(t, rt.Ctx.DB.Create(&models.Datasource{Id: 1, Name: "existing", Description: "original"}).Error)

	w := identityRequest(t, rt, rt.datasourceGrafanaFetch,
		grafana.Conn{URL: server.URL + "/grafana", AuthType: "token", Token: "fake-token"}, "0", nil)

	require.Equal(t, http.StatusOK, w.Code)
	var reply struct {
		Data struct {
			Items []grafanaPreviewItem `json:"items"`
		} `json:"data"`
		Error interface{} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &reply))
	require.Equal(t, "", reply.Error)
	require.Len(t, reply.Data.Items, 3)
	existing, pending, unsupported := reply.Data.Items[0], reply.Data.Items[1], reply.Data.Items[2]
	require.True(t, existing.Supported)
	require.True(t, existing.Duplicate)
	require.Equal(t, models.PROMETHEUS, existing.Datasource.PluginType)
	require.Equal(t, "http://metrics.invalid", existing.Datasource.HTTPJson.Url)
	require.True(t, pending.Supported)
	require.True(t, pending.NeedAuth)
	require.False(t, pending.Duplicate)
	require.False(t, unsupported.Supported)
	require.Nil(t, unsupported.Datasource)
	received := <-requests
	require.Equal(t, "/grafana/api/datasources", received.URL.Path)
	require.Equal(t, "Bearer fake-token", received.Header.Get("Authorization"))
	var stored []models.Datasource
	require.NoError(t, rt.Ctx.DB.Find(&stored).Error)
	require.Len(t, stored, 1)
	require.Equal(t, "original", stored[0].Description)
}

func TestDatasourceGrafanaFetch_ReportsUpstreamErrorWithoutWriting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)
	rt := grafanaImportRouter(t)

	w := identityRequest(t, rt, rt.datasourceGrafanaFetch,
		grafana.Conn{URL: server.URL, AuthType: "token", Token: "fake-token"}, "0", nil)

	require.Contains(t, w.Body.String(), "401")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Datasource{}).Count(&count).Error)
	require.Zero(t, count)
}

func templateRound6Router(t *testing.T, index *tplMatchIndex, client pkgprom.API) *Router {
	t.Helper()
	rt := proxyRound6Router(t, &models.Datasource{Id: 660002, PluginType: models.PROMETHEUS})
	builtinRound2Cache(t)
	rt.PromClients = &prom.PromClientMap{ReaderClients: map[int64]pkgprom.API{660002: client}}
	tplMatchIdxMu.Lock()
	original := tplMatchIdx
	tplMatchIdx = map[string]*tplMatchIndex{"zh_CN": index, "en_US": index}
	tplMatchIdxMu.Unlock()
	t.Cleanup(func() { tplMatchIdxMu.Lock(); tplMatchIdx = original; tplMatchIdxMu.Unlock() })
	return rt
}

func TestDatasourceTemplateMatch_QueriesSentinelsInBoundedBatchesAndReturnsMatches(t *testing.T) {
	// Two long sentinels force batching without needing thousands of fixture entries.
	first, second := "a"+strings.Repeat("_", 4500), "b"+strings.Repeat("_", 4500)
	entry := &tplComponentEntry{ComponentID: 1, Component: "redis", Dashboards: []tplDashboard{
		{UUID: 7, Name: "Redis", Sentinels: []string{first}},
		{UUID: 8, Name: "Absent", Sentinels: []string{"absent_metric"}},
	}}
	index := &tplMatchIndex{builtAt: time.Now(), sentinels: []string{first, second}, entries: []*tplComponentEntry{entry}}
	var queries []string
	client := &batchRound6PromAPI{query: func(_ context.Context, q string, _ time.Time) (model.Value, pkgprom.Warnings, error) {
		queries = append(queries, q)
		return model.Vector{&model.Sample{Metric: model.Metric{model.MetricNameLabel: model.LabelValue(first)}, Value: 1}}, nil, nil
	}}
	rt := templateRound6Router(t, index, client)

	w := identityRequest(t, rt, rt.datasourceTemplateMatch, map[string]int64{"id": 660002}, "0", nil)

	var got struct {
		Matched []*tplComponentEntry `json:"matched"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, []*tplComponentEntry{{ComponentID: 1, Component: "redis", Dashboards: entry.Dashboards[:1]}}, got.Matched)
	require.Equal(t, []string{
		`count by (__name__) ({__name__=~"` + first + `"})`,
		`count by (__name__) ({__name__=~"` + second + `"})`,
	}, queries)
}

func TestDatasourceTemplateMatch_EmptyIndexDoesNotQuery(t *testing.T) {
	calls := 0
	client := &batchRound6PromAPI{query: func(context.Context, string, time.Time) (model.Value, pkgprom.Warnings, error) {
		calls++
		return nil, nil, nil
	}}
	rt := templateRound6Router(t, &tplMatchIndex{builtAt: time.Now()}, client)

	w := identityRequest(t, rt, rt.datasourceTemplateMatch, map[string]int64{"id": 660002}, "0", nil)

	var got struct {
		Matched []*tplComponentEntry `json:"matched"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.NotNil(t, got.Matched)
	require.Empty(t, got.Matched)
	require.Zero(t, calls)
}

func TestDatasourceTemplateMatch_ReportsQueryError(t *testing.T) {
	client := &batchRound6PromAPI{query: func(context.Context, string, time.Time) (model.Value, pkgprom.Warnings, error) {
		return nil, nil, errors.New("query rejected")
	}}
	rt := templateRound6Router(t, &tplMatchIndex{builtAt: time.Now(), sentinels: []string{"redis_up"}}, client)

	w := identityRequest(t, rt, rt.datasourceTemplateMatch, map[string]int64{"id": 660002}, "0", nil)

	require.Contains(t, w.Body.String(), "query rejected")
	require.NotContains(t, w.Body.String(), `"matched"`)
}

func TestDatasourceTemplateMatch_RejectsUnavailableDatasource(t *testing.T) {
	cases := []struct {
		name    string
		id      int64
		plugin  string
		status  int
		message string
	}{
		{name: "missing", id: 999, plugin: models.PROMETHEUS, status: 404, message: "datasource not found"},
		{name: "wrong plugin", id: 660002, plugin: models.LOKI, status: 400, message: "only prometheus-like datasource"},
		{name: "not ready", id: 660002, plugin: models.PROMETHEUS, status: 400, message: "prometheus client not ready"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := proxyRound6Router(t, &models.Datasource{Id: 660002, PluginType: tc.plugin})
			rt.PromClients = &prom.PromClientMap{}

			w := identityRequest(t, rt, rt.datasourceTemplateMatch, map[string]int64{"id": tc.id}, "0", nil)

			require.Equal(t, tc.status, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
		})
	}
}

func TestDatasourceQuery_RestrictsMatchingDatasourcesToRequestedCategory(t *testing.T) {
	rt := identityRouter(t, &models.Datasource{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Datasource{
		{Id: 1, Name: "prod-a", PluginType: models.PROMETHEUS},
		{Id: 2, Name: "prod-b", PluginType: models.PROMETHEUS},
		{Id: 3, Name: "prod-logs", PluginType: models.LOKI},
	}).Error)

	w := identityRequest(t, rt, rt.datasourceQuery, datasourceQueryForm{
		Cate: models.PROMETHEUS, DatasourceQueries: []models.DatasourceQuery{models.DataSourceQueryAll},
	}, "0", nil)

	var got []datasourceQueryResp
	require.Empty(t, identityReply(t, w, &got))
	require.ElementsMatch(t, []datasourceQueryResp{{ID: 1, Name: "prod-a"}, {ID: 2, Name: "prod-b"}}, got)
}

func TestDatasourceRsaConfigGet_ExposesKeysOnlyWhenEnabled(t *testing.T) {
	cases := []struct {
		name     string
		enabled  bool
		public   string
		private  string
		password string
	}{
		{name: "disabled"},
		{name: "enabled", enabled: true, public: "cHVibGlj", private: "cHJpdmF0ZQ==", password: "test-password"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}
			rt.Center.RSA.OpenRSA = tc.enabled
			rt.HTTP.RSA.RSAPublicKey = []byte("public")
			rt.HTTP.RSA.RSAPrivateKey = []byte("private")
			rt.HTTP.RSA.RSAPassWord = "test-password"

			w := identityRequest(t, rt, rt.datasourceRsaConfigGet, nil, "0", nil)

			var got models.RsaConfig
			require.Empty(t, identityReply(t, w, &got))
			require.Equal(t, tc.enabled, got.OpenRSA)
			require.Equal(t, tc.public, got.RSAPublicKey)
			require.Equal(t, tc.private, got.RSAPrivateKey)
			require.Equal(t, tc.password, got.RSAPassWord)
		})
	}
}
