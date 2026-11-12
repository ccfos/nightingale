package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestListDatasources_AppliesQueryAndVisibilityBeforeLimit(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Datasource{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.Datasource{
		{Id: 1, Name: "prod metrics", PluginType: "prometheus", Category: "timeseries", Status: "enabled"},
		{Id: 2, Name: "prod SQL", PluginType: "mysql"},
		{Id: 3, Name: "staging metrics", PluginType: "prometheus"},
		{Id: 4, Name: "prod private", PluginType: "prometheus"},
	}).Error)
	var seenIDs []int64
	deps.FilterDatasources = func(rows []*models.Datasource, user *models.User) []*models.Datasource {
		require.Equal(t, int64(2), user.Id)
		var visible []*models.Datasource
		for _, row := range rows {
			seenIDs = append(seenIDs, row.Id)
			if row.Id != 4 {
				visible = append(visible, row)
			}
		}
		return visible
	}

	out, err := listDatasourcesBuiltin(context.Background(), deps, map[string]interface{}{
		"plugin_type": "prometheus", "query": "prod", "limit": float64(1),
	}, map[string]string{"user_id": "2"})

	require.NoError(t, err)
	require.Equal(t, []int64{4, 1}, seenIDs)
	var result struct {
		Total int                `json:"total"`
		Items []datasourceResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Equal(t, []datasourceResult{{
		Id: 1, Name: "prod metrics", PluginType: "prometheus", Category: "timeseries", Status: "enabled",
	}}, result.Items)
}

func TestGetDatasourceDetail_ReturnsMetadataWithoutCredentials(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Datasource{})
	ds := &models.Datasource{
		Id: 7, Name: "metrics", Description: "production", PluginType: "prometheus", PluginTypeName: "Prometheus",
		Category: "timeseries", ClusterName: "default", Status: "enabled", IsDefault: true, CreatedBy: "root", UpdatedBy: "alice",
		Auth: `{"basic_auth":true,"basic_auth_password":"private-password"}`, Settings: `{"token":"private-token"}`,
	}
	require.NoError(t, models.DB(deps.DBCtx).Create(ds).Error)
	deps.FilterDatasources = func(rows []*models.Datasource, user *models.User) []*models.Datasource { return rows }

	out, err := getDatasourceDetail(context.Background(), deps, map[string]interface{}{"id": "7"}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	require.NotContains(t, out, "private-password")
	require.NotContains(t, out, "private-token")
	var result datasourceDetailResult
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, datasourceDetailResult{
		Id: 7, Name: "metrics", Description: "production", PluginType: "prometheus", PluginTypeName: "Prometheus",
		Category: "timeseries", ClusterName: "default", Status: "enabled", IsDefault: true, CreatedBy: "root", UpdatedBy: "alice",
	}, result)
}

func TestGetDatasourceDetail_RejectsInvisibleDatasource(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Datasource{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Datasource{Id: 7, Name: "private"}).Error)
	deps.FilterDatasources = func(rows []*models.Datasource, user *models.User) []*models.Datasource { return nil }

	out, err := getDatasourceDetail(context.Background(), deps, map[string]interface{}{"id": "7"}, map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "no access to this datasource")
}

func TestGetDatasourceDetail_RequiresID(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Datasource{})

	out, err := getDatasourceDetail(context.Background(), deps, nil, map[string]string{"user_id": "1"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "id is required")

}

func TestListUsers_ReturnsFilteredContactAndRoles(t *testing.T) {
	deps := newResourceTestDeps(t, &models.UserGroupMember{}, &models.UserGroup{}, &models.BusiGroup{}, &models.BusiGroupMember{})
	require.NoError(t, models.DB(deps.DBCtx).Model(&models.User{}).Where("id = ?", 2).Updates(map[string]interface{}{
		"nickname": "Alice", "phone": "12345", "email": "alice@example.invalid", "roles": "Standard Viewer", "password": "hidden-password",
	}).Error)

	out, err := listUsers(context.Background(), deps, map[string]interface{}{"query": "alice", "limit": float64(1)},
		map[string]string{"user_id": "1"})

	require.NoError(t, err)
	require.NotContains(t, out, "hidden-password")
	var result struct {
		Total int          `json:"total"`
		Items []userResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Equal(t, []userResult{{
		Id: 2, Username: "alice", Nickname: "Alice", Phone: "12345", Email: "alice@example.invalid", Roles: []string{"Standard", "Viewer"},
	}}, result.Items)
}

func TestListUsers_RequiresUsersPermission(t *testing.T) {
	deps := newResourceTestDeps(t, &models.RoleOperation{})

	out, err := listUsers(context.Background(), deps, nil, map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "forbidden: no permission")
}

func TestListTeams_IncludesMembershipAndCreatedTeamsOnly(t *testing.T) {
	deps := newResourceTestDeps(t, &models.UserGroup{}, &models.UserGroupMember{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.UserGroupMember{GroupId: 10, UserId: 2}).Error)
	require.NoError(t, db.Create(&[]models.UserGroup{
		{Id: 10, Name: "A member", Note: "operations", CreateBy: "root"},
		{Id: 20, Name: "B created", CreateBy: "alice"},
		{Id: 30, Name: "C private", CreateBy: "root"},
	}).Error)

	out, err := listTeams(context.Background(), deps, nil, map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		Total int          `json:"total"`
		Items []teamResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 2, result.Total)
	require.Equal(t, []teamResult{
		{Id: 10, Name: "A member", Note: "operations", CreateBy: "root"}, {Id: 20, Name: "B created", CreateBy: "alice"},
	}, result.Items)
}

func TestListDashboards_FiltersGroupAndQuery(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Board{})
	resourceMembership(t, deps, "ro", PermDashboards)
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.Board{
		{Id: 1, GroupId: 1, Name: "CPU overview", Ident: "cpu", Tags: "prod", Public: 1},
		{Id: 2, GroupId: 2, Name: "CPU private"}, {Id: 3, GroupId: 1, Name: "Network"},
	}).Error)

	out, err := listDashboards(context.Background(), deps, map[string]interface{}{"query": "CPU", "limit": float64(1)},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		Total int               `json:"total"`
		Items []dashboardResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Equal(t, []dashboardResult{{
		Id: 1, GroupId: 1, Name: "CPU overview", Ident: "cpu", Tags: "prod", Public: 1,
	}}, result.Items)
}
