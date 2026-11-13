package router

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func grafanaImportRouter(t *testing.T) *Router {
	t.Helper()
	rt := identityRouter(t, &models.Datasource{})
	rt.DatasourceCache = &memsto.DatasourceCacheType{DatasourceCheckHook: func(*gin.Context) bool { return false }}
	rt.Center.Plugins = []cconf.Plugin{{Type: models.PROMETHEUS}}
	return rt
}

func grafanaImportReply(t *testing.T, rt *Router, items []models.Datasource) []grafanaImportResult {
	t.Helper()
	w := identityRequest(t, rt, rt.datasourceGrafanaImport, items, "0", &models.User{Username: "alice"})
	require.Equal(t, http.StatusOK, w.Code)
	var reply struct {
		Data struct {
			Items []grafanaImportResult `json:"items"`
		} `json:"data"`
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &reply))
	require.Empty(t, reply.Error)
	return reply.Data.Items
}

func TestGrafanaImport_PersistsConnectionClusterAndAuthenticatedAuthor(t *testing.T) {
	rt := grafanaImportRouter(t)
	form := models.Datasource{
		Name: "metrics", PluginType: models.PROMETHEUS, CreatedBy: "forged", UpdatedBy: "forged",
		HTTPJson: models.HTTP{
			Url: "https://metrics.example.invalid", Headers: map[string]string{"X-Tenant": "production"},
			Timeout: 3000, DialTimeout: 2000, MaxIdleConnsPerHost: 20,
		},
		AuthJson:     models.Auth{BasicAuth: true, BasicAuthUser: "reader", BasicAuthPassword: "password"},
		SettingsJson: map[string]interface{}{"prometheus.cluster_name": "edge-a", "scrape_interval": float64(15)},
	}

	got := grafanaImportReply(t, rt, []models.Datasource{form})

	require.Equal(t, []grafanaImportResult{{Name: "metrics", Status: "imported"}}, got)
	var stored models.Datasource
	require.NoError(t, rt.Ctx.DB.First(&stored).Error)
	require.NoError(t, stored.DB2FE())
	require.Equal(t, "enabled", stored.Status)
	require.Equal(t, "alice", stored.CreatedBy)
	require.Equal(t, "alice", stored.UpdatedBy)
	require.Equal(t, "edge-a", stored.ClusterName)
	require.Equal(t, form.HTTPJson, stored.HTTPJson)
	require.Equal(t, form.AuthJson, stored.AuthJson)
	require.Equal(t, form.SettingsJson, stored.SettingsJson)
}

func TestGrafanaImport_PreservesDisabledStatusForMissingCredentials(t *testing.T) {
	rt := grafanaImportRouter(t)
	form := models.Datasource{
		Name: "needs-auth", PluginType: models.PROMETHEUS, Status: "disabled",
		SettingsJson: map[string]interface{}{"prometheus.cluster_name": float64(123)},
	}

	got := grafanaImportReply(t, rt, []models.Datasource{form})

	require.Equal(t, []grafanaImportResult{{Name: "needs-auth", Status: "pending_auth"}}, got)
	var stored models.Datasource
	require.NoError(t, rt.Ctx.DB.First(&stored).Error)
	require.Equal(t, "disabled", stored.Status)
	// Non-string settings are not valid cluster names.
	require.Empty(t, stored.ClusterName)
}

func TestGrafanaImport_SkipsExistingNameWithoutChangingConnection(t *testing.T) {
	rt := grafanaImportRouter(t)
	original := models.Datasource{Id: 1, Name: "metrics", PluginType: models.PROMETHEUS, HTTP: `{"url":"original"}`}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	form := models.Datasource{Name: "metrics", PluginType: models.PROMETHEUS, HTTPJson: models.HTTP{Url: "replacement"}}

	got := grafanaImportReply(t, rt, []models.Datasource{form})

	require.Equal(t, []grafanaImportResult{{Name: "metrics", Status: "skipped", Reason: "name already exists"}}, got)
	var stored []models.Datasource
	require.NoError(t, rt.Ctx.DB.Find(&stored).Error)
	require.Equal(t, []models.Datasource{original}, stored)
}

func TestGrafanaImport_RejectsCaseInsensitiveDuplicatesWithinBatch(t *testing.T) {
	rt := grafanaImportRouter(t)
	forms := []models.Datasource{
		{Name: "Metrics", PluginType: models.PROMETHEUS},
		{Name: "metrics", PluginType: models.PROMETHEUS},
	}

	got := grafanaImportReply(t, rt, forms)

	require.Equal(t, []grafanaImportResult{
		{Name: "Metrics", Status: "imported"},
		{Name: "metrics", Status: "skipped", Reason: "name already exists"},
	}, got)
	var stored []models.Datasource
	require.NoError(t, rt.Ctx.DB.Find(&stored).Error)
	require.Len(t, stored, 1)
	require.Equal(t, "Metrics", stored[0].Name)
}

func TestGrafanaImport_RejectsDisabledPluginWithoutWriting(t *testing.T) {
	rt := grafanaImportRouter(t)

	got := grafanaImportReply(t, rt, []models.Datasource{{Name: "logs", PluginType: models.LOKI}})

	require.Equal(t, []grafanaImportResult{{Name: "logs", Status: "failed", Reason: "unsupported type"}}, got)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Datasource{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestGrafanaImport_ContinuesAfterInvalidDatasource(t *testing.T) {
	rt := grafanaImportRouter(t)
	forms := []models.Datasource{
		{Name: "invalid'name", PluginType: models.PROMETHEUS},
		{Name: "valid", PluginType: models.PROMETHEUS},
	}

	got := grafanaImportReply(t, rt, forms)

	require.Equal(t, []grafanaImportResult{
		{Name: "invalid'name", Status: "failed", Reason: "Name has invalid characters"},
		{Name: "valid", Status: "imported"},
	}, got)
	var stored []models.Datasource
	require.NoError(t, rt.Ctx.DB.Find(&stored).Error)
	require.Len(t, stored, 1)
	require.Equal(t, "valid", stored[0].Name)
}

func TestGrafanaImport_ReadOnlyDatasourceHookPreventsWrites(t *testing.T) {
	rt := grafanaImportRouter(t)
	rt.DatasourceCache.DatasourceCheckHook = func(*gin.Context) bool { return true }

	w := identityRequest(t, rt, rt.datasourceGrafanaImport,
		[]models.Datasource{{Name: "metrics", PluginType: models.PROMETHEUS}}, "0", &models.User{Username: "alice"})

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"data":[],"error":""}`, w.Body.String())
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Datasource{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestGrafanaImport_RejectsEmptySelection(t *testing.T) {
	rt := grafanaImportRouter(t)

	w := identityRequest(t, rt, rt.datasourceGrafanaImport, []models.Datasource{}, "0", nil)

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"error":{"message":"input json is empty"}}`, w.Body.String())
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Datasource{}).Count(&count).Error)
	require.Zero(t, count)
}
