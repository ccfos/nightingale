package router

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestUserBusiGroupsGets_ResolvesUserIDAndUsername(t *testing.T) {
	rt := identityRouter(t, &models.User{}, &models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserGroupMember{})
	seedBoardOwnerMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice"}).Error)
	for _, query := range []string{"?userid=1", "?username=alice"} {
		t.Run(query, func(t *testing.T) {
			w := identityRequest(t, rt, rt.userBusiGroupsGets, nil, "1"+query, nil)

			var got []models.BusiGroup
			require.Empty(t, identityReply(t, w, &got))
			require.Len(t, got, 1)
			require.Equal(t, int64(1), got[0].Id)
			require.Equal(t, "production", got[0].Name)
		})
	}
	rejected := identityRequest(t, rt, rt.userBusiGroupsGets, nil, "1", nil)
	require.Equal(t, http.StatusBadRequest, rejected.Code)
	require.Contains(t, rejected.Body.String(), "userid or username required")
}

func TestUserProfileGet_ExcludesPasswordAndReturnsProfile(t *testing.T) {
	rt := identityRouter(t, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&models.User{
		Id: 1, Username: "alice", Nickname: "Alice", Password: "private-hash", Roles: "Viewer",
	}).Error)

	w := identityRequest(t, rt, rt.userProfileGet, nil, "1", nil)

	var got models.User
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "alice", got.Username)
	require.Equal(t, "Alice", got.Nickname)
	require.Equal(t, []string{"Viewer"}, got.RolesLst)
	require.NotContains(t, w.Body.String(), "private-hash")
}

func TestSelfProfileGet_MarksAdminsAndExcludesPasswords(t *testing.T) {
	rt := &Router{}
	user := &models.User{Username: "admin", Password: "private-hash", RolesLst: []string{"Admin"}}

	w := identityRequest(t, rt, rt.selfProfileGet, nil, "1", user)

	var got models.User
	require.Empty(t, identityReply(t, w, &got))
	require.True(t, got.Admin)
	require.Equal(t, "admin", got.Username)
	require.NotContains(t, w.Body.String(), "private-hash")
}

func TestInstallDateGet_ReturnsRootCreationTimeOrZero(t *testing.T) {
	rt := identityRouter(t, &models.User{})

	empty := identityRequest(t, rt, rt.installDateGet, nil, "1", nil)

	var got int64
	require.Empty(t, identityReply(t, empty, &got))
	require.Zero(t, got)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "root", CreateAt: 123}).Error)

	installed := identityRequest(t, rt, rt.installDateGet, nil, "1", nil)

	require.Empty(t, identityReply(t, installed, &got))
	require.Equal(t, int64(123), got)
}

func TestBusiGroupGet_ReturnsTeamPermissionDetails(t *testing.T) {
	rt := identityRouter(t, &models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserGroup{})
	require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroup{Id: 1, Name: "production"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroup{Id: 10, Name: "oncall"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroupMember{BusiGroupId: 1, UserGroupId: 10, PermFlag: "rw"}).Error)

	w := identityRequest(t, rt, rt.busiGroupGet, nil, "1", nil)

	var got models.BusiGroup
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "production", got.Name)
	require.Len(t, got.UserGroups, 1)
	require.Equal(t, int64(10), got.UserGroups[0].UserGroup.Id)
	require.Equal(t, "oncall", got.UserGroups[0].UserGroup.Name)
	require.Equal(t, "rw", got.UserGroups[0].PermFlag)
}

func TestChartShareAdd_StoresConfigurationsAndReturnsRetrievableIDs(t *testing.T) {
	rt := identityRouter(t, &models.ChartShare{})
	form := []chartShareForm{
		{DatasourceId: 7, Configs: `{"metric":"cpu"}`},
		{DatasourceId: 8, Configs: `{"metric":"memory"}`},
	}

	w := identityRequest(t, rt, rt.chartShareAdd, form, "1", nil)

	var ids []int64
	require.Empty(t, identityReply(t, w, &ids))
	require.Len(t, ids, 2)
	require.NotEqual(t, ids[0], ids[1])
	read := identityRequest(t, rt, rt.chartShareGets, nil, "1?ids="+fmt.Sprint(ids[0]), nil)
	var got []models.ChartShare
	require.Empty(t, identityReply(t, read, &got))
	require.Len(t, got, 1)
	require.Equal(t, form[0].Configs, got[0].Configs)
	require.Equal(t, form[0].DatasourceId, got[0].DatasourceId)
	require.Equal(t, "operator", got[0].CreateBy)
}
