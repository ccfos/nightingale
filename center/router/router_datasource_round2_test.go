package router

import (
	"net/http"
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
