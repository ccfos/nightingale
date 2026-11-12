package router

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDatasourceUpsert_RejectsClickhouseHTTPAddressEvenWhenForced(t *testing.T) {
	rt := identityRouter(t, &models.Datasource{})
	rt.DatasourceCache = &memsto.DatasourceCacheType{DatasourceCheckHook: func(*gin.Context) bool { return false }}
	form := models.Datasource{
		Name: "invalid", PluginType: models.CLICKHOUSE, ForceSave: true,
		SettingsJson: map[string]interface{}{"ck.nodes": []string{"http://127.0.0.1:1"}},
	}

	w := identityRequest(t, rt, rt.datasourceUpsert, form, "1", &models.User{Username: "alice"})

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "node address should not start with http:// or https://")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Datasource{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestDatasourceUpdataStatus_ChangesOnlyStatusAndUpdateAudit(t *testing.T) {
	rt := identityRouter(t, &models.Datasource{})
	rt.DatasourceCache = &memsto.DatasourceCacheType{DatasourceCheckHook: func(*gin.Context) bool { return false }}
	original := models.Datasource{
		Id:         1,
		Name:       "metrics",
		PluginType: models.PROMETHEUS,
		Status:     "enabled",
		CreatedBy:  "creator",
	}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	form := models.Datasource{Id: 1, Name: "ignored", PluginType: models.PROMETHEUS, Status: "disabled"}

	w := identityRequest(t, rt, rt.datasourceUpdataStatus, form, "1", &models.User{Username: "alice"})

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"error":""`)
	var got models.Datasource
	require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
	require.Equal(t, "disabled", got.Status)
	require.Equal(t, original.Name, got.Name)
	require.Equal(t, original.CreatedBy, got.CreatedBy)
	require.Equal(t, "alice", got.UpdatedBy)
}

func TestDatasourceDel_RemovesOnlyRequestedDatasources(t *testing.T) {
	rt := identityRouter(t, &models.Datasource{})
	rt.DatasourceCache = &memsto.DatasourceCacheType{DatasourceCheckHook: func(*gin.Context) bool { return false }}
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Datasource{{Id: 1, Name: "one"}, {Id: 2, Name: "two"}}).Error)

	w := identityRequest(t, rt, rt.datasourceDel, []int64{1, 404}, "1", nil)

	require.JSONEq(t, `{"data":{},"error":""}`, w.Body.String())
	var got []models.Datasource
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, int64(2), got[0].Id)
}

func TestDatasourceUpsert_ForceSaveRejectsDuplicateWithoutProbing(t *testing.T) {
	cases := []struct {
		name     string
		plugin   string
		settings map[string]interface{}
	}{
		{name: "prometheus", plugin: models.PROMETHEUS},
		{name: "loki", plugin: models.LOKI},
		{
			name: "elasticsearch with supplied version", plugin: models.ELASTICSEARCH,
			settings: map[string]interface{}{"version": "8.0"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.Datasource{})
			rt.DatasourceCache = &memsto.DatasourceCacheType{DatasourceCheckHook: func(*gin.Context) bool { return false }}
			require.NoError(t, rt.Ctx.DB.Create(&models.Datasource{Id: 1, Name: "existing", PluginType: tc.plugin}).Error)
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(server.Close)
			form := models.Datasource{
				Name: "existing", PluginType: tc.plugin, ForceSave: true, SettingsJson: tc.settings,
				HTTPJson: models.HTTP{Url: server.URL},
			}

			w := identityRequest(t, rt, rt.datasourceUpsert, form, "1", &models.User{Username: "alice"})

			require.JSONEq(t, `{"error":{"message":"name already exists"}}`, w.Body.String())
			require.Zero(t, calls.Load())
			var got []models.Datasource
			require.NoError(t, rt.Ctx.DB.Find(&got).Error)
			require.Len(t, got, 1)
			require.Equal(t, "existing", got[0].Name)
			require.Empty(t, got[0].UpdatedBy)
		})
	}
}
