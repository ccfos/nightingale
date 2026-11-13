package router

import (
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConfigPutByKey_UpsertsValueWithOperatorAudit(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	for _, value := range []string{"first", "updated"} {
		w := identityRequest(t, rt, rt.configPutByKey, models.Configs{Ckey: "site_info", Cval: value}, "1", nil)

		require.Empty(t, identityReply(t, w, nil))
		var saved []models.Configs
		require.NoError(t, rt.Ctx.DB.Find(&saved).Error)
		require.Len(t, saved, 1)
		require.Equal(t, value, saved[0].Cval)
		require.Equal(t, "operator", saved[0].CreateBy)
		require.Equal(t, "operator", saved[0].UpdateBy)
	}
}

func TestConfigsGet_FiltersPrefixAndPaginates(t *testing.T) {
	rt := identityRouter(t, &models.Configs{}, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Configs{
		{Id: 1, Ckey: "site_one", Cval: "first"},
		{Id: 2, Ckey: "site_two", Cval: "second"},
		{Id: 3, Ckey: "other", Cval: "excluded"},
	}).Error)

	w := identityRequest(t, rt, rt.configsGet, nil, "1?prefix=site_&limit=1&p=2", nil)

	var got []models.Configs
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].Id)
	require.Equal(t, "first", got[0].Cval)
}

func TestConfigReads_ReturnRequestedBuiltinValues(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Configs{
		{Id: 1, Ckey: "site_info", Cval: `{"title":"monitoring"}`},
		{Id: 2, Ckey: EMBEDDEDDASHBOARD, Cval: "[]"},
		{Id: 3, Ckey: "external", Cval: "hidden", External: 1},
	}).Error)
	cases := []struct {
		name     string
		handler  gin.HandlerFunc
		id       string
		expected string
	}{
		{name: "site info", handler: rt.siteInfo, id: "1", expected: `{"title":"monitoring"}`},
		{name: "by key", handler: rt.configGetByKey, id: "1?key=site_info", expected: `{"title":"monitoring"}`},
		{name: "embedded dashboards", handler: rt.embeddedDashboardsGet, id: "1", expected: "[]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := identityRequest(t, rt, tc.handler, nil, tc.id, nil)

			var got string
			require.Empty(t, identityReply(t, w, &got))
			require.Equal(t, tc.expected, got)
		})
	}
	w := identityRequest(t, rt, rt.configGet, nil, "1", nil)
	var got models.Configs
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "site_info", got.Ckey)
	require.Equal(t, `{"title":"monitoring"}`, got.Cval)
}

func TestConfigGetAll_ExcludesExternalAndEmptyKeys(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Configs{
		{Ckey: "builtin", Cval: "value"}, {Ckey: "external", External: 1}, {Ckey: ""},
	}).Error)

	w := identityRequest(t, rt, rt.configGetAll, nil, "1", nil)

	var got []models.Configs
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "builtin", got[0].Ckey)
	require.Equal(t, "value", got[0].Cval)
}

func TestEmbeddedDashboardsPut_UsesFixedConfigurationKey(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})

	w := identityRequest(t, rt, rt.embeddedDashboardsPut,
		models.Configs{Ckey: "untrusted", Cval: `[{"name":"overview"}]`}, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var saved models.Configs
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, EMBEDDEDDASHBOARD, saved.Ckey)
	require.Equal(t, `[{"name":"overview"}]`, saved.Cval)
	require.Equal(t, "operator", saved.UpdateBy)
}

func TestConfigsPost_RecordsServiceIdentityOrDefault(t *testing.T) {
	for _, username := range []string{"", "service-user"} {
		t.Run("user="+username, func(t *testing.T) {
			rt := identityRouter(t, &models.Configs{})
			handler := func(c *gin.Context) { c.Set("user", username); rt.configsPost(c) }
			body := []models.Configs{{Ckey: "custom", Cval: "value", CreateBy: "forged"}}

			w := identityRequest(t, rt, handler, body, "1", nil)

			require.Empty(t, identityReply(t, w, nil))
			var saved models.Configs
			require.NoError(t, rt.Ctx.DB.First(&saved).Error)
			expected := username
			if expected == "" {
				expected = "default"
			}
			require.Equal(t, expected, saved.CreateBy)
			require.Equal(t, expected, saved.UpdateBy)
			require.Equal(t, "value", saved.Cval)
		})
	}
}

func TestConfigsPut_UpdatesValueAndServiceAudit(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	require.NoError(t, rt.Ctx.DB.Create(&models.Configs{Id: 1, Ckey: "custom", Cval: "old", CreateBy: "alice"}).Error)
	handler := func(c *gin.Context) { c.Set("user", "service-user"); rt.configsPut(c) }

	w := identityRequest(t, rt, handler, []models.Configs{{Id: 1, Ckey: "custom", Cval: "updated"}}, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var saved models.Configs
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "updated", saved.Cval)
	require.Equal(t, "service-user", saved.UpdateBy)
	require.Equal(t, "alice", saved.CreateBy)
}

func TestConfigsDel_DeletesOnlyRequestedIDs(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Configs{{Id: 1, Ckey: "one"}, {Id: 2, Ckey: "two"}}).Error)

	w := identityRequest(t, rt, rt.configsDel, idsForm{Ids: []int64{1, 404}}, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var saved []models.Configs
	require.NoError(t, rt.Ctx.DB.Find(&saved).Error)
	require.Len(t, saved, 1)
	require.Equal(t, "two", saved[0].Ckey)
}

func TestConfigPutByKey_AdminMiddlewareRejectsNonAdminWithoutWriting(t *testing.T) {
	cases := []struct {
		name   string
		roles  string
		status int
	}{
		{name: "admin", roles: "Admin", status: http.StatusOK},
		{name: "viewer", roles: "Viewer", status: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.Configs{}, &models.User{})
			user := &models.User{Id: 9, Username: "operator", Roles: tc.roles}
			require.NoError(t, rt.Ctx.DB.Create(user).Error)
			require.NoError(t, rt.Ctx.DB.Create(&models.Configs{Ckey: "site_info", Cval: "old"}).Error)
			handler := func(c *gin.Context) {
				c.Set("userid", user.Id)
				rt.admin()(c)
				rt.configPutByKey(c)
			}

			w := identityRequest(t, rt, handler, models.Configs{Ckey: "site_info", Cval: "new"}, "9", user)

			require.Equal(t, tc.status, w.Code)
			value, err := models.ConfigsGet(rt.Ctx, "site_info")
			require.NoError(t, err)
			if tc.status == http.StatusOK {
				require.Empty(t, identityReply(t, w, nil))
				require.Equal(t, "new", value)
			} else {
				require.Equal(t, "old", value)
			}
		})
	}
}

func TestConfigContactOptions_HideDisabledEntries(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		handler func(*Router) gin.HandlerFunc
	}{
		{
			name:    "notify channels",
			key:     models.NOTIFYCHANNEL,
			handler: func(rt *Router) gin.HandlerFunc { return rt.notifyChannelsGets },
		},
		{
			name:    "contact keys",
			key:     models.NOTIFYCONTACT,
			handler: func(rt *Router) gin.HandlerFunc { return rt.contactKeysGets },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.Configs{})
			require.NoError(t, rt.Ctx.DB.Create(&models.Configs{
				Ckey: tc.key, Cval: `[{"name":"Visible","ident":"shown"},{"name":"Secret","ident":"hidden","hide":true}]`,
			}).Error)

			w := identityRequest(t, rt, tc.handler(rt), nil, "1", nil)

			var got []models.LabelAndKey
			require.Empty(t, identityReply(t, w, &got))
			require.Equal(t, []models.LabelAndKey{{Label: "Visible", Key: "shown"}}, got)
		})
	}
}
