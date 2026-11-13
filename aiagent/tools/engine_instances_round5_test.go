package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestListAlertEngineInstances_FiltersDatasourceAndCluster(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertingEngines{})
	// Stay well outside the 30-second health boundary without waiting for a heartbeat to expire.
	heartbeat := time.Now().Add(-time.Hour).Unix()
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.AlertingEngines{
		{Instance: "engine-a", DatasourceId: 7, EngineCluster: "prod", Clock: heartbeat},
		{Instance: "engine-b", DatasourceId: 8, EngineCluster: "prod", Clock: heartbeat},
		{Instance: "engine-c", DatasourceId: 7, EngineCluster: "test", Clock: heartbeat},
	}).Error)

	out, err := listAlertEngineInstances(context.Background(), deps, map[string]interface{}{
		"datasource_id": float64(7), "engine_cluster": "prod",
	}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var result struct {
		Count     int `json:"count"`
		Instances []struct {
			Instance      string `json:"instance"`
			EngineCluster string `json:"engine_cluster"`
			DatasourceID  int64  `json:"datasource_id"`
			LastHeartbeat string `json:"last_heartbeat"`
			StaleSeconds  int64  `json:"stale_seconds"`
			Healthy       bool   `json:"healthy"`
		} `json:"instances"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Count)
	require.Len(t, result.Instances, 1)
	instance := result.Instances[0]
	require.Equal(t, "engine-a", instance.Instance)
	require.Equal(t, "prod", instance.EngineCluster)
	require.Equal(t, int64(7), instance.DatasourceID)
	require.Equal(t, time.Unix(heartbeat, 0).Format("2006-01-02 15:04:05"), instance.LastHeartbeat)
	require.GreaterOrEqual(t, instance.StaleSeconds, int64(3600))
	require.False(t, instance.Healthy)
}

func TestListAlertEngineInstances_ReportsDatabaseFailure(t *testing.T) {
	deps := newResourceTestDeps(t)

	out, err := listAlertEngineInstances(context.Background(), deps, nil, map[string]string{"user_id": "1"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "failed to list alert engine instances")
	require.ErrorContains(t, err, "no such table")
}
