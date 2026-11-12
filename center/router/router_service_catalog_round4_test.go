package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDatasourceGetsByService_FiltersTypeAndPreservesConnectionSettings(t *testing.T) {
	rt := identityRouter(t, &models.Datasource{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Datasource{
		{
			Id: 1, Name: "metrics", PluginType: models.PROMETHEUS,
			HTTP: `{"url":"https://metrics.example.invalid"}`, Auth: `{"basic_auth":true,"basic_auth_password":"service-secret"}`,
		},
		{Id: 2, Name: "logs", PluginType: models.LOKI},
	}).Error)

	w := identityRequest(t, rt, rt.datasourceGetsByService, nil, "0?typ=prometheus", nil)

	var got []models.Datasource
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].Id)
	require.Equal(t, "https://metrics.example.invalid", got[0].HTTPJson.Url)
	require.True(t, got[0].AuthJson.BasicAuth)
	require.Equal(t, "service-secret", got[0].AuthJson.BasicAuthPassword)
}

func TestGetDatasourceIds_FiltersByEngineCluster(t *testing.T) {
	rt := identityRouter(t, &models.Datasource{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Datasource{
		{Id: 1, Name: "metrics-a", ClusterName: "edge-a"},
		{Id: 2, Name: "metrics-b", ClusterName: "edge-b"},
		{Id: 3, Name: "logs-a", ClusterName: "edge-a"},
	}).Error)

	w := identityRequest(t, rt, rt.getDatasourceIds, nil, "0?name=edge-a", nil)

	var got []int64
	require.Empty(t, identityReply(t, w, &got))
	require.ElementsMatch(t, []int64{1, 3}, got)
}

func TestAlertSubscribeGetsByService_DecodesMatchingArrays(t *testing.T) {
	rt := identityRouter(t, &models.AlertSubscribe{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertSubscribe{
		Id: 1, Name: "critical", GroupId: 7, Severities: "[1,2]", DatasourceIds: "[3,4]", Webhooks: `["local-hook"]`,
	}).Error)

	w := identityRequest(t, rt, rt.alertSubscribeGetsByService, nil, "0", nil)

	var got []models.AlertSubscribe
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "critical", got[0].Name)
	require.Equal(t, int64(7), got[0].GroupId)
	require.Equal(t, []int{1, 2}, got[0].SeveritiesJson)
	require.Equal(t, []int64{3, 4}, got[0].DatasourceIdsJson)
	require.Equal(t, []string{"local-hook"}, got[0].WebhooksJson)
}

func TestBoardGetsByBids_ReturnsOnlySelectedDashboards(t *testing.T) {
	rt := identityRouter(t, &models.Board{}, &models.BusiGroup{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Board{
		{Id: 1, GroupId: 1, Name: "CPU"}, {Id: 2, GroupId: 1, Name: "Memory"}, {Id: 3, GroupId: 1, Name: "Disk"},
	}).Error)

	require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroup{Id: 1, Name: "production"}).Error)

	w := identityRequest(t, rt, rt.boardGetsByBids, nil, "0?bids=1,3,404", nil)

	var got []struct {
		ID        int64  `json:"board_id"`
		Name      string `json:"board_name"`
		GroupName string `json:"busi_group_name"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	require.ElementsMatch(t, []int64{1, 3}, []int64{got[0].ID, got[1].ID})
	require.ElementsMatch(t, []string{"CPU", "Disk"}, []string{got[0].Name, got[1].Name})
	require.Equal(t, "production", got[0].GroupName)
	require.Equal(t, "production", got[1].GroupName)
}

func TestMetricFilterAdd_RejectsBlankRequiredFieldsWithoutWriting(t *testing.T) {
	cases := []struct {
		name    string
		form    models.MetricFilter
		message string
	}{
		{name: "name", form: models.MetricFilter{Name: " ", Configs: "{}"}, message: "name is blank"},
		{name: "configs", form: models.MetricFilter{Name: "CPU", Configs: " \t"}, message: "configs is blank"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.MetricFilter{})

			w := identityRequest(t, rt, rt.metricFilterAdd, tc.form, "0", &models.User{Username: "alice"})

			require.Contains(t, identityReply(t, w, nil), tc.message)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.MetricFilter{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestBusiGroupsGetTags_ReturnsSelectedGroupsUserTags(t *testing.T) {
	rt := identityRouter(t, &models.Target{}, &models.TargetBusiGroup{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Target{
		{Ident: "host-a", Tags: "env=prod role=api", HostTags: []string{"agent=linux"}},
		{Ident: "host-b", Tags: "env=stage"},
	}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.TargetBusiGroup{
		{TargetIdent: "host-a", GroupId: 1}, {TargetIdent: "host-b", GroupId: 2},
	}).Error)

	w := identityRequest(t, rt, rt.busiGroupsGetTags, nil, "0?gids=1", nil)

	var got []string
	require.Empty(t, identityReply(t, w, &got))
	require.ElementsMatch(t, []string{"env=prod", "role=api"}, got)
}

func TestBusiGroupsGetTags_EmptyGroupReturnsNoTags(t *testing.T) {
	rt := identityRouter(t, &models.Target{}, &models.TargetBusiGroup{})
	require.NoError(t, rt.Ctx.DB.Create(&models.Target{Ident: "host-a", Tags: "env=secret"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.TargetBusiGroup{TargetIdent: "host-a", GroupId: 1}).Error)

	w := identityRequest(t, rt, rt.busiGroupsGetTags, nil, "0?gids=999", nil)

	// Group 999 has no hosts, so no other group's tags may appear.
	var got []string
	require.Empty(t, identityReply(t, w, &got))
	require.Empty(t, got)
}

func TestAlertAggrViewGets_ExcludesAnotherUsersPrivateViews(t *testing.T) {
	rt := identityRouter(t, &models.AlertAggrView{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertAggrView{
		{Id: 1, Name: "public", Cate: 0, CreateBy: 2},
		{Id: 2, Name: "own", Cate: 1, CreateBy: 1},
		{Id: 3, Name: "foreign", Cate: 1, CreateBy: 2},
	}).Error)
	handler := func(c *gin.Context) { c.Set("userid", int64(1)); rt.alertAggrViewGets(c) }

	w := identityRequest(t, rt, handler, nil, "0", nil)

	var got []models.AlertAggrView
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	require.ElementsMatch(t, []int64{1, 2}, []int64{got[0].Id, got[1].Id})
}

func TestNotifyRuleGet_ResolvesRecipientDisplayNamesWithoutPersistingThem(t *testing.T) {
	rt := identityRouter(t, &models.NotifyRule{}, &models.NotifyChannelConfig{}, &models.UserGroupMember{})
	rt.UserCache = &memsto.UserCacheType{}
	rt.UserCache.Set(map[int64]*models.User{
		1: {Id: 1, Username: "alice", Nickname: "Alice"},
		2: {Id: 2, Username: "bob"},
	}, 2, 1, 0, 0)
	rt.UserGroupCache = &memsto.UserGroupCacheType{}
	rt.UserGroupCache.Set(map[int64]*models.UserGroup{10: {Id: 10, Name: "On call"}}, 1, 1)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{UserId: 1, GroupId: 10}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.NotifyChannelConfig{ID: 5, Ident: "email"}).Error)
	original := models.NotifyRule{
		ID: 1, Name: "critical", UserGroupIds: []int64{10}, NotifyConfigs: []models.NotifyConfig{
			{
				ChannelID: 5,
				Params: map[string]interface{}{
					"user_ids": []int64{1, 2, 404, 1}, "user_group_ids": []int64{10, 404, 10},
				},
			},
		},
	}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)

	w := identityRequest(t, rt, rt.notifyRuleGet, nil, "1", &models.User{Id: 1, Username: "alice"})

	var got models.NotifyRule
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got.NotifyConfigs, 1)
	require.Equal(t, "email", got.NotifyConfigs[0].ChannelIdent)
	// Use the login name when a recipient has no nickname, and omit missing IDs.
	require.Equal(t, []string{"Alice", "bob"}, got.NotifyConfigs[0].UserNames)
	require.Equal(t, []string{"On call"}, got.NotifyConfigs[0].UserGroupNames)
	var stored models.NotifyRule
	require.NoError(t, rt.Ctx.DB.First(&stored, 1).Error)
	require.Empty(t, stored.NotifyConfigs[0].UserNames)
	require.Empty(t, stored.NotifyConfigs[0].UserGroupNames)
	require.Empty(t, stored.NotifyConfigs[0].ChannelIdent)
}
