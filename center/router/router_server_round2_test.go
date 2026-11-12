package router

import (
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestServersActive_FiltersDatasourceOrClusterAndExcludesStaleInstances(t *testing.T) {
	rt := identityRouter(t, &models.AlertingEngines{})
	now := time.Now().Unix()
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertingEngines{
		{Instance: "node-a", EngineCluster: "production", DatasourceId: 7, Clock: now},
		{Instance: "node-b", EngineCluster: "production", DatasourceId: 8, Clock: now},
		{Instance: "stale", EngineCluster: "production", DatasourceId: 7, Clock: 1},
		{Instance: "other", EngineCluster: "other", DatasourceId: 9, Clock: now},
	}).Error)
	cases := []struct {
		name     string
		query    string
		expected []string
	}{
		{name: "datasource", query: "1?dsid=7", expected: []string{"node-a"}},
		{name: "cluster", query: "1?engine_name=production", expected: []string{"node-a", "node-b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := identityRequest(t, rt, rt.serversActive, nil, tc.query, nil)

			var got []string
			require.Empty(t, identityReply(t, w, &got))
			require.Equal(t, tc.expected, got)
		})
	}
	missing := identityRequest(t, rt, rt.serversActive, nil, "1", nil)
	require.Equal(t, "dsid is required", identityReply(t, missing, nil))
}

func TestServerHeartbeat_RefreshesExistingInstanceWithoutDuplicatingIt(t *testing.T) {
	rt := identityRouter(t, &models.AlertingEngines{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertingEngines{
		Id: 1, Instance: "node-a", EngineCluster: "production", DatasourceId: 7, Clock: 1,
	}).Error)
	form := models.HeartbeatInfo{Instance: "node-a", EngineCluster: "production", DatasourceId: 7}

	w := identityRequest(t, rt, rt.serverHeartbeat, form, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var got []models.AlertingEngines
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].Id)
	require.Equal(t, "node-a", got[0].Instance)
	require.Equal(t, int64(7), got[0].DatasourceId)
	require.Greater(t, got[0].Clock, int64(1))
}
