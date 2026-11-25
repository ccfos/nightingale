package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupDatasourceSecretsTest(t *testing.T) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.Datasource{}))
	for _, ds := range []models.Datasource{
		{Id: 1, Name: "prometheus", PluginType: models.PROMETHEUS, HTTPJson: models.HTTP{Url: "http://url-user:url-password@prom.example:9090/prometheus"}},
		{Id: 2, Name: "elasticsearch", PluginType: models.ELASTICSEARCH, HTTPJson: models.HTTP{Urls: []string{"https://url-user:url-password@es.example:9200", "https://url-token@es.example:9201"}}},
	} {
		ds.AuthJson = models.Auth{BasicAuth: true, BasicAuthUser: "auth-user", BasicAuthPassword: "auth-password"}
		ds.HTTPJson.Headers = map[string]string{"Authorization": "header-secret"}
		ds.HTTPJson.TLS.ClientKey = "key-secret"
		ds.SettingsJson = map[string]interface{}{"password": "settings-secret"}
		require.NoError(t, ds.FE2DB())
		require.NoError(t, db.Create(&ds).Error)
	}
	return &Router{
		Ctx: &ctx.Context{DB: db},
		DatasourceCache: &memsto.DatasourceCacheType{
			DatasourceCheckHook: func(*gin.Context) bool { return false },
			DatasourceFilter:    func(ds []*models.Datasource, _ *models.User) []*models.Datasource { return ds },
		},
	}
}

func TestDatasourceResponsesRedactURLCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, method, path string
		brief, admin, anon bool
	}{
		{"user list", http.MethodPost, "/api/n9e/datasource/list", false, false, false},
		{"admin list", http.MethodPost, "/api/n9e/datasource/list", false, true, false},
		{"user brief", http.MethodGet, "/api/n9e/datasource/brief", true, false, false},
		{"admin brief", http.MethodGet, "/api/n9e/datasource/brief", true, true, false},
		{"anonymous brief", http.MethodGet, "/api/n9e/datasource/brief", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := setupDatasourceSecretsTest(t)
			rt.Center.AnonymousAccess.PromQuerier = tc.anon
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
			c.Request.Header.Set("Content-Type", "application/json")
			if !tc.anon {
				user := &models.User{}
				if tc.admin {
					user.RolesLst = []string{models.AdminRole}
				}
				c.Set("user", user)
			}
			if tc.brief {
				rt.datasourceBriefs(c)
			} else {
				rt.datasourceList(c)
			}
			require.Equal(t, http.StatusOK, w.Code)
			var result struct {
				Dat   []*models.Datasource `json:"dat"`
				Data  []*models.Datasource `json:"data"`
				Err   string               `json:"err"`
				Error string               `json:"error"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
			require.Empty(t, result.Err)
			require.Empty(t, result.Error)
			if !tc.brief {
				result.Dat = result.Data
			}
			require.Len(t, result.Dat, 2)
			redacted := tc.brief || !tc.admin
			for _, ds := range result.Dat {
				wantURL := "http://prom.example:9090/prometheus"
				wantURLs := []string{"https://es.example:9200", "https://es.example:9201"}
				if !redacted {
					wantURL = "http://url-user:url-password@prom.example:9090/prometheus"
					wantURLs = []string{"https://url-user:url-password@es.example:9200", "https://url-token@es.example:9201"}
				}
				if ds.PluginType == models.PROMETHEUS {
					require.Equal(t, wantURL, ds.HTTPJson.Url)
				} else {
					require.Equal(t, wantURLs, ds.HTTPJson.Urls)
				}
			}
			if redacted {
				for _, secret := range []string{"url-user", "url-password", "url-token", "auth-user", "auth-password", "header-secret", "key-secret", "settings-secret"} {
					require.NotContains(t, w.Body.String(), secret)
				}
			}
			// Reading a redacted response must never write it back to storage.
			ds := models.Datasource{Id: 1}
			require.NoError(t, ds.Get(rt.Ctx))
			require.Equal(t, "http://url-user:url-password@prom.example:9090/prometheus", ds.HTTPJson.Url)
			require.Equal(t, "auth-password", ds.AuthJson.BasicAuthPassword)
		})
	}
}

func TestDatasourceDescRetainsURLCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rt := setupDatasourceSecretsTest(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/n9e/datasource/desc", strings.NewReader(`{"id":1}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &models.User{RolesLst: []string{models.AdminRole}})
	rt.datasourceGet(c)
	require.Equal(t, http.StatusOK, w.Code)
	var result struct {
		Data  models.Datasource `json:"data"`
		Error string            `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Empty(t, result.Error)
	require.Equal(t, "http://url-user:url-password@prom.example:9090/prometheus", result.Data.HTTPJson.Url)
	require.Equal(t, "auth-password", result.Data.AuthJson.BasicAuthPassword)
}

func TestDatasourceSharedBriefHidesConnectionInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rt := setupDatasourceSecretsTest(t)
	require.NoError(t, rt.Ctx.DB.AutoMigrate(&models.BoardPayload{}))
	const boardID int64 = 336200
	require.NoError(t, rt.Ctx.DB.Create(&models.BoardPayload{
		Id: boardID, Payload: `{"panels":[{"datasourceValue":2}]}`,
	}).Error)
	t.Cleanup(func() {
		boardDsSetMu.Lock()
		delete(boardDsSetItems, boardID)
		boardDsSetMu.Unlock()
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/n9e/datasource/brief", nil)
	c.Set(boardTokenBidKey, boardID)
	rt.datasourceBriefs(c)
	require.Equal(t, http.StatusOK, w.Code)
	var result struct {
		Dat []*models.Datasource `json:"dat"`
		Err string               `json:"err"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Empty(t, result.Err)
	require.Len(t, result.Dat, 1)
	require.Equal(t, int64(2), result.Dat[0].Id)
	require.Equal(t, models.HTTP{}, result.Dat[0].HTTPJson)
	require.Nil(t, result.Dat[0].SettingsJson)
}

func TestDatasourceBriefsPreservesUseEdgeWhileRedactingSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.Datasource{}))

	for _, ds := range []models.Datasource{
		{
			Id:          10,
			Name:        "edge-es",
			PluginType:  models.ELASTICSEARCH,
			ClusterName: "edge-bj",
			HTTPJson:    models.HTTP{Urls: []string{"https://user:secret@es.internal:9200"}},
			AuthJson:    models.Auth{BasicAuth: true, BasicAuthUser: "elastic", BasicAuthPassword: "es-password"},
			SettingsJson: map[string]interface{}{
				"use_edge": true,
				"password": "settings-secret",
			},
		},
		{
			Id:          11,
			Name:        "edge-mysql",
			PluginType:  "mysql",
			ClusterName: "edge-bj",
			SettingsJson: map[string]interface{}{
				"use_edge":                true,
				"mysql.password":          "db-secret",
				"mysql.connection_string": "secret",
			},
		},
	} {
		require.NoError(t, ds.FE2DB())
		require.NoError(t, db.Create(&ds).Error)
	}

	rt := &Router{
		Ctx: &ctx.Context{DB: db},
		DatasourceCache: &memsto.DatasourceCacheType{
			DatasourceFilter: func(ds []*models.Datasource, _ *models.User) []*models.Datasource { return ds },
		},
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/n9e/datasource/brief", nil)
	c.Set("user", &models.User{})
	rt.datasourceBriefs(c)

	require.Equal(t, http.StatusOK, w.Code)
	var result struct {
		Dat []*models.Datasource `json:"dat"`
		Err string               `json:"err"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Empty(t, result.Err)
	require.Len(t, result.Dat, 2)
	for _, ds := range result.Dat {
		require.Equal(t, map[string]interface{}{"use_edge": true}, ds.SettingsJson)
	}
	for _, secret := range []string{"secret", "es-password", "settings-secret", "db-secret"} {
		require.NotContains(t, w.Body.String(), secret)
	}
}

func TestDatasourceUpsertRejectsNonBoolUseEdgeAndOpenSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prevProxyHook := EdgeProxyTransportHook
	prevWrapHook := dscache.CenterDatasourceWrapHook
	EdgeProxyTransportHook = nil
	dscache.CenterDatasourceWrapHook = nil
	t.Cleanup(func() {
		EdgeProxyTransportHook = prevProxyHook
		dscache.CenterDatasourceWrapHook = prevWrapHook
	})

	rt := setupDatasourceSecretsTest(t)

	// 1. Non-boolean string "true" is rejected with a type validation error.
	wStr := httptest.NewRecorder()
	cStr, _ := gin.CreateTestContext(wStr)
	cStr.Request = httptest.NewRequest(http.MethodPost, "/api/n9e/datasource/upsert", strings.NewReader(`{
		"name":"str-edge",
		"plugin_type":"elasticsearch",
		"cluster_name":"edge-bj",
		"settings":{"use_edge":"true"},
		"http":{"url":"http://127.0.0.1:9200"}
	}`))
	cStr.Request.Header.Set("Content-Type", "application/json")
	cStr.Set("user", &models.User{Username: "root"})
	rt.datasourceUpsert(cStr)
	require.Contains(t, wStr.Body.String(), "boolean")

	// 2. Boolean true is rejected when open-source center lacks the commercial hooks.
	wOS := httptest.NewRecorder()
	cOS, _ := gin.CreateTestContext(wOS)
	cOS.Request = httptest.NewRequest(http.MethodPost, "/api/n9e/datasource/upsert", strings.NewReader(`{
		"name":"os-edge",
		"plugin_type":"elasticsearch",
		"cluster_name":"edge-bj",
		"settings":{"use_edge":true},
		"http":{"url":"http://127.0.0.1:9200"}
	}`))
	cOS.Request.Header.Set("Content-Type", "application/json")
	cOS.Set("user", &models.User{Username: "root"})
	rt.datasourceUpsert(cOS)
	require.Contains(t, wOS.Body.String(), "not supported")

	// 3. Boolean true is also rejected when commercial hooks exist but EdgeQueryEnabledHook returns false.
	prevEnabledHook := EdgeQueryEnabledHook
	EdgeProxyTransportHook = func(ds *models.Datasource) http.RoundTripper { return http.DefaultTransport }
	dscache.CenterDatasourceWrapHook = func(_ datasource.DatasourceInfo, ds datasource.Datasource) (datasource.Datasource, bool) {
		return ds, true
	}
	EdgeQueryEnabledHook = func() bool { return false }
	t.Cleanup(func() { EdgeQueryEnabledHook = prevEnabledHook })

	wDisabled := httptest.NewRecorder()
	cDisabled, _ := gin.CreateTestContext(wDisabled)
	cDisabled.Request = httptest.NewRequest(http.MethodPost, "/api/n9e/datasource/upsert", strings.NewReader(`{
		"name":"disabled-edge",
		"plugin_type":"elasticsearch",
		"cluster_name":"edge-bj",
		"settings":{"use_edge":true},
		"http":{"url":"http://127.0.0.1:9200"}
	}`))
	cDisabled.Request.Header.Set("Content-Type", "application/json")
	cDisabled.Set("user", &models.User{Username: "root"})
	rt.datasourceUpsert(cDisabled)
	require.Contains(t, wDisabled.Body.String(), "disabled")
}

func TestDatasourceGrafanaImportValidatesUseEdge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prevProxyHook := EdgeProxyTransportHook
	prevWrapHook := dscache.CenterDatasourceWrapHook
	prevEnabledHook := EdgeQueryEnabledHook
	EdgeProxyTransportHook = func(ds *models.Datasource) http.RoundTripper { return http.DefaultTransport }
	dscache.CenterDatasourceWrapHook = func(_ datasource.DatasourceInfo, ds datasource.Datasource) (datasource.Datasource, bool) {
		return ds, true
	}
	EdgeQueryEnabledHook = func() bool { return true }
	t.Cleanup(func() {
		EdgeProxyTransportHook = prevProxyHook
		dscache.CenterDatasourceWrapHook = prevWrapHook
		EdgeQueryEnabledHook = prevEnabledHook
	})

	rt := setupDatasourceSecretsTest(t)
	rt.Center.Plugins = []cconf.Plugin{
		{Type: models.ELASTICSEARCH},
		{Type: models.PROMETHEUS},
	}

	payload := `[
		{"name":"imp-empty-cluster","plugin_type":"elasticsearch","settings":{"use_edge":true},"http":{"url":"http://127.0.0.1:9200"}},
		{"name":"imp-str-bool","plugin_type":"elasticsearch","settings":{"use_edge":"true","es.cluster_name":"edge-bj"},"http":{"url":"http://127.0.0.1:9200"}},
		{"name":"imp-unsupported-type","plugin_type":"prometheus","settings":{"use_edge":true,"prometheus.cluster_name":"edge-bj"},"http":{"url":"http://127.0.0.1:9090"}},
		{"name":"imp-valid-edge","plugin_type":"elasticsearch","settings":{"use_edge":true,"es.cluster_name":" edge-bj "},"http":{"url":"http://127.0.0.1:9200"}},
		{"name":"imp-explicit-edge","plugin_type":"elasticsearch","cluster_name":" edge-sh ","settings":{"use_edge":true},"http":{"url":"http://127.0.0.1:9200"}}
	]`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/n9e/datasource/grafana/import", strings.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &models.User{Username: "root"})
	rt.datasourceGrafanaImport(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data struct {
			Items []grafanaImportResult `json:"items"`
		} `json:"data"`
		Err string `json:"err"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Empty(t, body.Err)
	require.Len(t, body.Data.Items, 5)

	require.Equal(t, "failed", body.Data.Items[0].Status)
	require.Contains(t, body.Data.Items[0].Reason, "cluster_name is required")
	require.Equal(t, "failed", body.Data.Items[1].Status)
	require.Contains(t, body.Data.Items[1].Reason, "boolean")
	require.Equal(t, "failed", body.Data.Items[2].Status)
	require.Contains(t, body.Data.Items[2].Reason, "does not support edge query")
	require.Equal(t, "imported", body.Data.Items[3].Status)
	require.Equal(t, "imported", body.Data.Items[4].Status)

	var imported []models.Datasource
	require.NoError(t, rt.Ctx.DB.Where("name LIKE ?", "imp-%").Order("name").Find(&imported).Error)
	require.Len(t, imported, 2)
	require.Equal(t, "imp-explicit-edge", imported[0].Name)
	require.Equal(t, "edge-sh", imported[0].ClusterName)
	require.Equal(t, "imp-valid-edge", imported[1].Name)
	require.Equal(t, "edge-bj", imported[1].ClusterName)
	require.NoError(t, imported[1].DB2FE())
	require.Equal(t, "edge-bj", imported[1].SettingsJson["es.cluster_name"])
}

func TestDatasourceUpsertValidatesAndSavesUseEdge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prevProxyHook := EdgeProxyTransportHook
	prevWrapHook := dscache.CenterDatasourceWrapHook
	prevEnabledHook := EdgeQueryEnabledHook
	EdgeProxyTransportHook = func(ds *models.Datasource) http.RoundTripper { return http.DefaultTransport }
	dscache.CenterDatasourceWrapHook = func(_ datasource.DatasourceInfo, ds datasource.Datasource) (datasource.Datasource, bool) {
		return ds, true
	}
	EdgeQueryEnabledHook = func() bool { return true }
	t.Cleanup(func() {
		EdgeProxyTransportHook = prevProxyHook
		dscache.CenterDatasourceWrapHook = prevWrapHook
		EdgeQueryEnabledHook = prevEnabledHook
	})

	rt := setupDatasourceSecretsTest(t)
	rt.Ctx.IsCenter = true
	testStats := &memsto.Stats{
		GaugeCronDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_ds_upsert_cron_duration"}, []string{"name"}),
		GaugeSyncNumber:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_ds_upsert_cron_sync_number"}, []string{"name"}),
	}
	rt.DatasourceCache = memsto.NewDatasourceCache(rt.Ctx, testStats)
	rt.DatasourceCache.DatasourceCheckHook = func(*gin.Context) bool { return false }
	rt.DatasourceCache.DatasourceFilter = func(ds []*models.Datasource, _ *models.User) []*models.Datasource { return ds }

	doUpsert := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/n9e/datasource/upsert", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", &models.User{Username: "root"})
		rt.datasourceUpsert(c)
		return w
	}

	// Reject empty, no_assigned_engine, whitespace-padded no_assigned_engine, and unsupported type.
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "empty cluster",
			body: `{"name":"edge-empty","plugin_type":"elasticsearch","cluster_name":"","settings":{"use_edge":true},"http":{"url":"http://127.0.0.1:1"}}`,
			want: "cluster_name is required",
		},
		{
			name: "no_assigned_engine",
			body: `{"name":"edge-none","plugin_type":"elasticsearch","cluster_name":"no_assigned_engine","settings":{"use_edge":true},"http":{"url":"http://127.0.0.1:1"}}`,
			want: "cluster_name is required",
		},
		{
			name: "whitespace no_assigned_engine",
			body: `{"name":"edge-ws-none","plugin_type":"elasticsearch","cluster_name":" no_assigned_engine ","settings":{"use_edge":true},"http":{"url":"http://127.0.0.1:1"}}`,
			want: "cluster_name is required",
		},
		{
			name: "unsupported type",
			body: `{"name":"edge-prom","plugin_type":"prometheus","cluster_name":"edge-bj","settings":{"use_edge":true},"http":{"url":"http://127.0.0.1:1"}}`,
			want: "does not support edge query",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := doUpsert(tc.body)
			require.Contains(t, w.Body.String(), tc.want)
		})
	}

	// Create succeeds with unreachable URL and whitespace-padded cluster_name (persisted trimmed), returning saved_unverified.
	wCreate := doUpsert(`{
		"name":"edge-es-ok",
		"plugin_type":"elasticsearch",
		"cluster_name":" edge-bj ",
		"settings":{"use_edge":true},
		"http":{"url":"http://127.0.0.1:1"}
	}`)
	require.Equal(t, http.StatusOK, wCreate.Code)
	var upsertResp struct {
		Data struct {
			ID           int64                  `json:"id"`
			Verification datasourceVerification `json:"verification"`
		} `json:"data"`
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(wCreate.Body.Bytes(), &upsertResp))
	require.Empty(t, upsertResp.Error)
	require.NotZero(t, upsertResp.Data.ID)
	require.Equal(t, "saved_unverified", upsertResp.Data.Verification.State)
	require.Equal(t, "none", upsertResp.Data.Verification.Stage)

	var created models.Datasource
	require.NoError(t, rt.Ctx.DB.First(&created, upsertResp.Data.ID).Error)
	require.Equal(t, "edge-bj", created.ClusterName)

	// Update path succeeds and persists trimmed cluster_name while staying saved_unverified.
	wUpdate := doUpsert(fmt.Sprintf(`{
		"id":%d,
		"name":"edge-es-ok",
		"plugin_type":"elasticsearch",
		"cluster_name":" edge-sh ",
		"settings":{"use_edge":true,"es.cluster_name":" edge-old "},
		"http":{"url":"http://127.0.0.1:1"}
	}`, upsertResp.Data.ID))
	require.Equal(t, http.StatusOK, wUpdate.Code)
	var updateResp struct {
		Data struct {
			ID           int64                  `json:"id"`
			Verification datasourceVerification `json:"verification"`
		} `json:"data"`
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(wUpdate.Body.Bytes(), &updateResp))
	require.Empty(t, updateResp.Error)
	require.Equal(t, upsertResp.Data.ID, updateResp.Data.ID)
	require.Equal(t, "saved_unverified", updateResp.Data.Verification.State)

	var updated models.Datasource
	require.NoError(t, rt.Ctx.DB.First(&updated, upsertResp.Data.ID).Error)
	require.Equal(t, "edge-sh", updated.ClusterName)
	require.NoError(t, updated.DB2FE())
	require.Equal(t, "edge-old", updated.SettingsJson["es.cluster_name"])

	// Spoofing plugin_type during edit to bypass unsupported-type validation is rejected.
	promDS := models.Datasource{
		Name:        "prom-direct-only",
		PluginType:  models.PROMETHEUS,
		ClusterName: "edge-bj",
		Status:      "enabled",
		HTTP:        `{"url":"http://127.0.0.1:9090"}`,
		Settings:    `{}`,
		Auth:        `{}`,
	}
	require.NoError(t, rt.Ctx.DB.Create(&promDS).Error)
	wSpoof := doUpsert(fmt.Sprintf(`{
		"id":%d,
		"name":"prom-direct-only",
		"plugin_type":"elasticsearch",
		"cluster_name":"edge-bj",
		"settings":{"use_edge":true},
		"http":{"url":"http://127.0.0.1:9090"}
	}`, promDS.Id))
	require.Contains(t, wSpoof.Body.String(), "does not match existing record")
	var reloadedProm models.Datasource
	require.NoError(t, rt.Ctx.DB.First(&reloadedProm, promDS.Id).Error)
	require.Equal(t, models.PROMETHEUS, reloadedProm.PluginType)
	require.NoError(t, reloadedProm.DB2FE())
	require.Nil(t, reloadedProm.SettingsJson["use_edge"])
}

type stubRoundTripper struct{}

func (stubRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, nil
}

func TestTransportGetAndEvictHandleCustomRoundTripper(t *testing.T) {
	const dsidCustom = 99001
	const dsidStd = 99002
	t.Cleanup(func() {
		transportEvict(dsidCustom)
		transportEvict(dsidStd)
	})

	transportPut(dsidCustom, 100, stubRoundTripper{})
	tr, ok := transportGet(dsidCustom, 100)
	require.True(t, ok)
	require.NotNil(t, tr)

	// Stale updatedAt on custom non-*http.Transport RoundTripper must not panic.
	tr, ok = transportGet(dsidCustom, 200)
	require.False(t, ok)
	require.Nil(t, tr)

	transportPut(dsidCustom, 200, stubRoundTripper{})
	transportEvict(dsidCustom)
	_, ok = transportGet(dsidCustom, 200)
	require.False(t, ok)

	transportPut(dsidStd, 300, &http.Transport{})
	transportEvict(dsidStd)
	_, ok = transportGet(dsidStd, 300)
	require.False(t, ok)
}
