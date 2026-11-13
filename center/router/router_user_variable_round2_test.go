package router

import (
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserVariableConfigAdd_TrimsKeyAndAssignsOwnership(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	form := models.Configs{Id: 999, Ckey: " custom_key ", Cval: "value", CreateBy: "forged", External: 0}

	w := identityRequest(t, rt, rt.userVariableConfigAdd, form, "1", &models.User{Username: "operator"})

	require.Empty(t, identityReply(t, w, nil))
	var got models.Configs
	require.NoError(t, rt.Ctx.DB.First(&got).Error)
	require.NotEqual(t, int64(999), got.Id)
	require.Equal(t, "custom_key", got.Ckey)
	require.Equal(t, "value", got.Cval)
	require.Equal(t, models.ConfigExternal, got.External)
	require.Equal(t, "operator", got.CreateBy)
	require.Equal(t, "operator", got.UpdateBy)
}

func TestUserVariableConfigAdd_RejectsReservedAndDuplicateKeys(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		message string
	}{
		{name: "reserved", key: "event", message: "reserved words"},
		{name: "invalid identifier", key: "bad-key", message: "invalid key"},
		{name: "duplicate", key: "taken", message: "duplicate ckey"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.Configs{})
			original := models.Configs{Id: 1, Ckey: "taken", Cval: "original", External: 1}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)

			w := identityRequest(t, rt, rt.userVariableConfigAdd, models.Configs{Ckey: tc.key, Cval: "bad"}, "1", nil)

			require.Contains(t, identityReply(t, w, nil), tc.message)
			var got []models.Configs
			require.NoError(t, rt.Ctx.DB.Find(&got).Error)
			require.Equal(t, []models.Configs{original}, got)
		})
	}
}

func TestUserVariableConfigPut_RequiresCreatorOrAdmin(t *testing.T) {
	cases := []struct {
		name    string
		user    models.User
		allowed bool
	}{
		{name: "creator", user: models.User{Username: "operator"}, allowed: true},
		{name: "admin", user: models.User{Username: "admin", RolesLst: []string{"Admin"}}, allowed: true},
		{name: "outsider", user: models.User{Username: "outsider"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.Configs{})
			original := models.Configs{Id: 1, Ckey: "custom", Cval: "old", External: 1, CreateBy: "operator", CreateAt: 100}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)
			form := models.Configs{Id: 999, Ckey: " renamed ", Cval: "new", Note: "updated", CreateBy: "forged"}

			w := identityRequest(t, rt, rt.userVariableConfigPut, form, "1", &tc.user)

			var got models.Configs
			require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
			if tc.allowed {
				require.Empty(t, identityReply(t, w, nil))
				require.Equal(t, "renamed", got.Ckey)
				require.Equal(t, "new", got.Cval)
				require.Equal(t, "updated", got.Note)
				require.Equal(t, original.CreateBy, got.CreateBy)
				require.Equal(t, original.CreateAt, got.CreateAt)
				require.Equal(t, original.External, got.External)
				require.Equal(t, "operator", got.UpdateBy)
			} else {
				require.Equal(t, http.StatusForbidden, w.Code)
				require.Equal(t, original, got)
			}
		})
	}
}

func TestUserVariableConfigDel_RequiresCreatorOrAdminAndPreservesBuiltins(t *testing.T) {
	cases := []struct {
		name     string
		user     models.User
		external int
		deleted  bool
		status   int
	}{
		{name: "creator", user: models.User{Username: "owner"}, external: 1, deleted: true, status: 200},
		{name: "admin", user: models.User{RolesLst: []string{"Admin"}}, external: 1, deleted: true, status: 200},
		{name: "outsider", user: models.User{Username: "other"}, external: 1, status: 403},
		{name: "builtin", user: models.User{RolesLst: []string{"Admin"}}, status: 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.Configs{})
			original := models.Configs{Id: 1, Ckey: "custom", Cval: "original", External: tc.external, CreateBy: "owner"}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)

			w := identityRequest(t, rt, rt.userVariableConfigDel, nil, "1", &tc.user)

			require.Equal(t, tc.status, w.Code)
			var got []models.Configs
			require.NoError(t, rt.Ctx.DB.Find(&got).Error)
			if tc.deleted {
				require.Empty(t, got)
			} else {
				require.Equal(t, []models.Configs{original}, got)
			}
		})
	}
}

func TestUserVariableConfigGets_ListsOnlyExternalVariables(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Configs{
		{Id: 1, Ckey: "external_one", External: 1}, {Id: 2, Ckey: "builtin"}, {Id: 3, Ckey: "external_two", External: 1},
	}).Error)

	w := identityRequest(t, rt, rt.userVariableConfigGets, nil, "1", nil)

	var got []models.Configs
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	require.Equal(t, "external_two", got[0].Ckey)
	require.Equal(t, "external_one", got[1].Ckey)
}

func TestUserVariableConfigMutation_AdminMiddlewareRejectsViewer(t *testing.T) {
	rt := identityRouter(t, &models.Configs{}, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "viewer", Roles: "Viewer"}).Error)
	handler := func(c *gin.Context) { c.Set("userid", int64(1)); rt.admin()(c); rt.userVariableConfigAdd(c) }

	w := identityRequest(t, rt, handler, models.Configs{Ckey: "custom", Cval: "value"}, "1", nil)

	require.Equal(t, http.StatusForbidden, w.Code)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Configs{}).Count(&count).Error)
	require.Zero(t, count)
}
