package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestMetricFilterGets_IncludesOwnedAndSharedFiltersOnly(t *testing.T) {
	rt := identityRouter(t, &models.MetricFilter{}, &models.UserGroupMember{}, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{GroupId: 10, UserId: 1}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.MetricFilter{
		{ID: 1, Name: "owned", CreateBy: "alice"},
		{ID: 2, Name: "shared", CreateBy: "bob", GroupsPerm: []models.GroupPerm{{Gid: 10}}},
		{ID: 3, Name: "private", CreateBy: "bob", GroupsPerm: []models.GroupPerm{{Gid: 20, Write: true}}},
	}).Error)

	w := identityRequest(t, rt, rt.metricFilterGets, nil, "1", &models.User{Id: 1, Username: "alice"})

	var got []models.MetricFilter
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	require.ElementsMatch(t, []int64{1, 2}, []int64{got[0].ID, got[1].ID})
}

func TestMetricFilterPut_RequiresOwnershipOrWritableTeam(t *testing.T) {
	cases := []struct {
		name     string
		user     models.User
		writable bool
		allowed  bool
	}{
		{name: "owner", user: models.User{Id: 2, Username: "owner"}, allowed: true},
		{name: "writer", user: models.User{Id: 1, Username: "writer"}, writable: true, allowed: true},
		{name: "reader", user: models.User{Id: 1, Username: "reader"}},
		{name: "outsider", user: models.User{Id: 3, Username: "outsider"}, writable: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.MetricFilter{}, &models.UserGroupMember{})
			require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{GroupId: 10, UserId: 1}).Error)
			original := models.MetricFilter{
				ID: 1, Name: "original", Configs: "{}", CreateBy: "owner", UpdateBy: "owner",
				GroupsPerm: []models.GroupPerm{{Gid: 10, Write: tc.writable}},
			}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)
			form := models.MetricFilter{ID: 1, Name: "updated", Configs: `{"query":"up"}`, CreateBy: "forged"}

			w := identityRequest(t, rt, rt.metricFilterPut, form, "1", &tc.user)

			var got models.MetricFilter
			require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
			require.Equal(t, original.CreateBy, got.CreateBy)
			if tc.allowed {
				require.Empty(t, identityReply(t, w, nil))
				require.Equal(t, form.Name, got.Name)
				require.Equal(t, form.Configs, got.Configs)
				require.Equal(t, tc.user.Username, got.UpdateBy)
			} else {
				require.Equal(t, "forbidden", identityReply(t, w, nil))
				require.Equal(t, original, got)
			}
		})
	}
}

func TestMetricFilterDel_ChecksEveryFilterBeforeDeleting(t *testing.T) {
	rt := identityRouter(t, &models.MetricFilter{}, &models.UserGroupMember{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.MetricFilter{
		{ID: 1, Name: "owned", CreateBy: "alice"}, {ID: 2, Name: "foreign", CreateBy: "bob"},
	}).Error)

	w := identityRequest(t, rt, rt.metricFilterDel, idsForm{Ids: []int64{1, 2}},
		"1", &models.User{Id: 1, Username: "alice"})

	require.Equal(t, "forbidden", identityReply(t, w, nil))
	var got []models.MetricFilter
	require.NoError(t, rt.Ctx.DB.Order("id").Find(&got).Error)
	require.Len(t, got, 2)
	require.Equal(t, "owned", got[0].Name)
	require.Equal(t, "foreign", got[1].Name)
}

func TestMetricFilterDel_AllowsWritableTeamAndOwner(t *testing.T) {
	rt := identityRouter(t, &models.MetricFilter{}, &models.UserGroupMember{})
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{GroupId: 10, UserId: 1}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.MetricFilter{
		{ID: 1, CreateBy: "alice"},
		{ID: 2, CreateBy: "bob", GroupsPerm: []models.GroupPerm{{Gid: 10, Write: true}}},
		{ID: 3, CreateBy: "bob"},
	}).Error)

	w := identityRequest(t, rt, rt.metricFilterDel, idsForm{Ids: []int64{1, 2}},
		"1", &models.User{Id: 1, Username: "alice"})

	require.Empty(t, identityReply(t, w, nil))
	var got []models.MetricFilter
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, int64(3), got[0].ID)
}

func TestGetMetricPromql_AddsMatchersToEachSelector(t *testing.T) {
	rt := &Router{}
	form := metricPromqlReq{LabelFilter: `{env="prod"}`, Promql: `sum(rate(requests_total{job="api"}[5m])) / sum(up)`}

	w := identityRequest(t, rt, rt.getMetricPromql, form, "1", nil)

	var got string
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, `sum(rate(requests_total{env="prod",job="api"}[5m])) / sum(up{env="prod"})`, got)
}
