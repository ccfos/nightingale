package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestCreateDashboard_RejectsInvalidInputWithoutWriting(t *testing.T) {
	cases := []struct {
		name  string
		patch map[string]interface{}
		want  string
	}{
		{name: "missing name", patch: map[string]interface{}{"name": ""}, want: "name is required"},
		{name: "unknown group", patch: map[string]interface{}{"group_id": "9"}, want: "busi group not found"},
		{name: "missing datasource", patch: map[string]interface{}{"datasource_id": ""}, want: "datasource_id is required"},
		{name: "missing panels", patch: map[string]interface{}{"panels": ""}, want: "panels is required"},
		{name: "malformed panels", patch: map[string]interface{}{"panels": "{"}, want: "invalid panels JSON"},
		{name: "malformed variables", patch: map[string]interface{}{"variables": "{"}, want: "invalid variables JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.Board{}, &models.BoardPayload{})
			db := models.DB(deps.DBCtx)
			require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
			args := map[string]interface{}{
				"group_id": "1", "name": "CPU", "datasource_id": "7", "panels": `[{"name":"Runbook","type":"text"}]`,
			}
			for key, value := range tc.patch {
				args[key] = value
			}

			got, err := createDashboard(context.Background(), deps, args, map[string]string{"user_id": "1"})

			require.Empty(t, got)
			require.ErrorContains(t, err, tc.want)
			var count int64
			require.NoError(t, db.Model(&models.Board{}).Count(&count).Error)
			require.Zero(t, count)
			require.NoError(t, db.Model(&models.BoardPayload{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestCreateDashboard_UsesPageDatasourceAndPersistsVariables(t *testing.T) {
	deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.Board{}, &models.BoardPayload{}, &models.Datasource{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.Datasource{Id: 7, Name: "metrics", PluginType: "prometheus"}).Error)

	got, err := createDashboard(context.Background(), deps, map[string]interface{}{
		"group_id": "1", "name": "CPU", "tags": "production",
		"panels":    `[{"name":"CPU","type":"timeseries","queries":[{"promql":"up{host=~\"$host\"}"}]}]`,
		"variables": `[{"name":"host","definition":"label_values(up,host)","multi":false}]`,
	}, map[string]string{"user_id": "1", "datasource_id": "7"})

	require.NoError(t, err)
	var result struct {
		ID             int64  `json:"id"`
		DatasourceID   int64  `json:"datasource_id"`
		DatasourceName string `json:"datasource_name"`
		Variables      int    `json:"variables_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(got), &result))
	require.Equal(t, int64(7), result.DatasourceID)
	require.Equal(t, "metrics", result.DatasourceName)
	require.Equal(t, 1, result.Variables)
	board, err := models.BoardGetByID(deps.DBCtx, result.ID)
	require.NoError(t, err)
	require.Equal(t, "production", board.Tags)
	payload, err := models.BoardPayloadGet(deps.DBCtx, result.ID)
	require.NoError(t, err)
	var config map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(payload), &config))
	variables := config["var"].([]interface{})
	require.Len(t, variables, 2)
	require.Equal(t, float64(7), variables[0].(map[string]interface{})["defaultValue"])
	require.Equal(t, "host", variables[1].(map[string]interface{})["name"])
	require.Equal(t, false, variables[1].(map[string]interface{})["multi"])
}

func TestCreateDashboard_ReportsDuplicateNameWithoutOverwriting(t *testing.T) {
	deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.Board{}, &models.BoardPayload{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	board := &models.Board{GroupId: 1, Name: "CPU", CreateBy: "original"}
	require.NoError(t, board.AtomicAdd(deps.DBCtx, `{"panels":[]}`))

	got, err := createDashboard(context.Background(), deps, map[string]interface{}{
		"group_id": "1", "name": "CPU", "datasource_id": "7", "panels": `[{"name":"Runbook","type":"text"}]`,
	}, map[string]string{"user_id": "1"})

	require.Empty(t, got)
	require.ErrorContains(t, err, `dashboard name "CPU" already exists in busi_group 1`)
	require.ErrorContains(t, err, `"CPU-v2"`)
	var count int64
	require.NoError(t, db.Model(&models.Board{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
	payload, err := models.BoardPayloadGet(deps.DBCtx, board.Id)
	require.NoError(t, err)
	require.JSONEq(t, `{"panels":[]}`, payload)
}
