package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func setupEdgeDatasourceRouterTest(t *testing.T) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.Datasource{}))
	for _, ds := range []models.Datasource{
		{Id: 1, Name: "prometheus", PluginType: models.PROMETHEUS, HTTPJson: models.HTTP{Url: "http://prom.example:9090/prometheus"}},
		{Id: 2, Name: "elasticsearch", PluginType: models.ELASTICSEARCH, HTTPJson: models.HTTP{Urls: []string{"https://es.example:9200", "https://es.example:9201"}}},
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

func TestDatasourceBriefSettingsHookAndRedaction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := DatasourceBriefSettingsHook
	DatasourceBriefSettingsHook = func(item *models.Datasource, safeSettings map[string]interface{}) map[string]interface{} {
		if v, ok := item.SettingsJson["custom_flag"]; ok {
			if safeSettings == nil {
				safeSettings = make(map[string]interface{})
			}
			safeSettings["custom_flag"] = v
		}
		return safeSettings
	}
	t.Cleanup(func() { DatasourceBriefSettingsHook = prev })

	rt := setupEdgeDatasourceRouterTest(t)
	var ds models.Datasource
	require.NoError(t, rt.Ctx.DB.First(&ds, 2).Error)
	require.NoError(t, ds.DB2FE())
	ds.SettingsJson["custom_flag"] = true
	require.NoError(t, ds.FE2DB())
	require.NoError(t, rt.Ctx.DB.Save(&ds).Error)

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
	byID := make(map[int64]*models.Datasource, len(result.Dat))
	for _, item := range result.Dat {
		byID[item.Id] = item
	}
	require.Empty(t, byID[1].SettingsJson)
	require.Equal(t, map[string]interface{}{"custom_flag": true}, byID[2].SettingsJson)
	for _, secret := range []string{"auth-password", "header-secret", "key-secret", "settings-secret"} {
		require.NotContains(t, w.Body.String(), secret)
	}
}
