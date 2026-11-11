package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"
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
