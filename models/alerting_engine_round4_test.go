package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAlertingEngineAdd_RejectsDuplicateInstanceDatasourcePairs(t *testing.T) {
	c := pipelineContext(t, &AlertingEngines{})
	require.NoError(t, AlertingEngineAdd(c, "engine-a", 7))

	err := AlertingEngineAdd(c, "engine-a", 7)

	require.ErrorContains(t, err, "already exists")
	require.NoError(t, AlertingEngineAdd(c, "engine-a", 8))
	require.NoError(t, AlertingEngineAdd(c, "engine-b", 7))
	ids, err := AlertingEngineGetDatasourceIds(c, "engine-a")
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{7, 8}, ids)
	missing, err := AlertingEngineGetDatasourceIds(c, "missing")
	require.NoError(t, err)
	require.Equal(t, []int64{}, missing)
}

func TestAlertingEngineUpdateDatasource_ExcludesSelfAndRejectsConflicts(t *testing.T) {
	c := pipelineContext(t, &AlertingEngines{})
	row := AlertingEngines{Id: 1, Instance: "engine", DatasourceId: 7, EngineCluster: "prod", Clock: 100}
	require.NoError(t, c.DB.Create(&row).Error)
	require.NoError(t, c.DB.Create(&AlertingEngines{Id: 2, Instance: "engine", DatasourceId: 8}).Error)
	require.NoError(t, row.UpdateDatasourceId(c, 7))

	err := row.UpdateDatasourceId(c, 8)

	require.ErrorContains(t, err, "already exists")
	require.NoError(t, row.UpdateDatasourceId(c, 9))
	got, err := AlertingEngineGet(c, "id = ?", 1)
	require.NoError(t, err)
	require.Equal(t, int64(9), got.DatasourceId)
	require.Equal(t, "prod", got.EngineCluster)
	require.Equal(t, int64(100), got.Clock)
}

func TestAlertingEngineGets_FiltersAndOrdersInstances(t *testing.T) {
	c := pipelineContext(t, &AlertingEngines{})
	require.NoError(t, c.DB.Create(&[]AlertingEngines{
		{Id: 1, Instance: "zeta", DatasourceId: 7},
		{Id: 2, Instance: "alpha", DatasourceId: 7},
		{Id: 3, Instance: "beta", DatasourceId: 8},
	}).Error)

	rows, err := AlertingEngineGets(c, "datasource_id = ?", 7)

	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "alpha", rows[0].Instance)
	require.Equal(t, "zeta", rows[1].Instance)
	all, err := AlertingEngineGets(c, "")
	require.NoError(t, err)
	require.Len(t, all, 3)
	missing, err := AlertingEngineGet(c, "id = ?", 99)
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestAlertingEngineClusterQueries_ExcludeEmptyAndDeduplicateClusters(t *testing.T) {
	c := pipelineContext(t, &AlertingEngines{})
	require.NoError(t, c.DB.Create(&[]AlertingEngines{
		{Instance: "zeta", DatasourceId: 7, EngineCluster: "prod"},
		{Instance: "alpha", DatasourceId: 7, EngineCluster: "prod"},
		{Instance: "beta", DatasourceId: 8, EngineCluster: "dev"},
		{Instance: "empty", DatasourceId: 7},
	}).Error)

	clusters, err := AlertingEngineGetsClusters(c, "")

	require.NoError(t, err)
	require.Equal(t, []string{"dev", "prod"}, clusters)
	filtered, err := AlertingEngineGetsClusters(c, "datasource_id = ?", 7)
	require.NoError(t, err)
	require.Equal(t, []string{"prod"}, filtered)
	instances, err := AlertingEngineGetsInstances(c, "engine_cluster = ?", "prod")
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "zeta"}, instances)
	all, err := AlertingEngineGetsInstances(c, "")
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "beta", "empty", "zeta"}, all)
}

func TestAlertingEngineGetsAlive_UsesStrictClockBoundary(t *testing.T) {
	c := pipelineContext(t, &AlertingEngines{})
	require.NoError(t, c.DB.Create(&[]AlertingEngines{
		{Instance: "stale", Clock: 99},
		{Instance: "boundary", Clock: 100},
		{Instance: "live", Clock: 101, DatasourceId: 7, EngineCluster: "prod"},
	}).Error)

	rows, err := AlertingEngineGetsAlive(c, 100)

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "live", rows[0].Instance)
	require.Equal(t, int64(7), rows[0].DatasourceId)
	require.Equal(t, "prod", rows[0].EngineCluster)
}

func TestAlertingEngineHeartbeat_RefreshesOnlyMatchingTuple(t *testing.T) {
	c := pipelineContext(t, &AlertingEngines{})
	require.NoError(t, c.DB.Create(&AlertingEngines{
		Id: 1, Instance: "engine", EngineCluster: "prod", DatasourceId: 7, Clock: 1,
	}).Error)
	require.NoError(t, c.DB.Create(&AlertingEngines{
		Id: 2, Instance: "engine", EngineCluster: "other", DatasourceId: 7, Clock: 2,
	}).Error)

	require.NoError(t, AlertingEngineHeartbeatWithCluster(c, "engine", "prod", 7))
	require.NoError(t, AlertingEngineHeartbeatWithCluster(c, "new", "dev", 8))
	rows, err := AlertingEngineGets(c, "")

	require.NoError(t, err)
	require.Len(t, rows, 3)
	refreshed, err := AlertingEngineGet(c, "id = ?", 1)
	require.NoError(t, err)
	require.Greater(t, refreshed.Clock, int64(1))
	untouched, err := AlertingEngineGet(c, "id = ?", 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), untouched.Clock)
	inserted, err := AlertingEngineGet(c, "instance = ?", "new")
	require.NoError(t, err)
	require.Equal(t, "dev", inserted.EngineCluster)
	require.Equal(t, int64(8), inserted.DatasourceId)
	require.NotZero(t, inserted.Clock)
}

func TestAlertingEngineDel_DeletesOnlySelectedRows(t *testing.T) {
	c := pipelineContext(t, &AlertingEngines{})
	require.NoError(t, c.DB.Create(&[]AlertingEngines{{Id: 1, Instance: "one"}, {Id: 2, Instance: "two"}}).Error)
	require.NoError(t, AlertingEngineDel(c, nil))

	require.NoError(t, AlertingEngineDel(c, []int64{1}))
	rows, err := AlertingEngineGets(c, "")

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(2), rows[0].Id)
}
