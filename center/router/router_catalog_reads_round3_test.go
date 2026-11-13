package router

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ccfos/nightingale/v6/center/sso"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSsoConfigGets_DecodesStoredSettingsWithoutContactingProviders(t *testing.T) {
	rt := identityRouter(t, &models.SsoConfig{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.SsoConfig{
		{Id: 1, Name: "dingtalk", Content: `{"enable":false,"display_name":"Company login"}`, UpdateAt: 100},
		{Id: 2, Name: "feishu", Content: `{"enable":false,"display_name":"Team login"}`, UpdateAt: 200},
		{Id: 3, Name: "LDAP", Content: "Enable = false", UpdateAt: 300},
	}).Error)

	w := identityRequest(t, rt, rt.ssoConfigGets, nil, "0", nil)

	var got []models.SsoConfig
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 3)
	byName := make(map[string]models.SsoConfig)
	for _, config := range got {
		byName[config.Name] = config
	}
	require.Equal(t, map[string]interface{}{"enable": false, "display_name": "Company login"}, byName["dingtalk"].SettingJson)
	require.Equal(t, map[string]interface{}{"enable": false, "display_name": "Team login"}, byName["feishu"].SettingJson)
	require.Equal(t, int64(200), byName["feishu"].UpdateAt)
	require.Empty(t, byName["dingtalk"].Content)
	require.Equal(t, "Enable = false", byName["LDAP"].Content)
}

func TestSsoConfigGets_AddsCompatibilityEntriesForMissingProviders(t *testing.T) {
	rt := identityRouter(t, &models.SsoConfig{})
	require.NoError(t, rt.Ctx.DB.Create(&models.SsoConfig{Name: "LDAP", Content: "Enable = false"}).Error)

	w := identityRequest(t, rt, rt.ssoConfigGets, nil, "0", nil)

	var got []models.SsoConfig
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 3)
	require.Equal(t, "LDAP", got[0].Name)
	require.Equal(t, "dingtalk", got[1].Name)
	require.Equal(t, "feishu", got[2].Name)
	require.Empty(t, got[1].Content)
	require.Nil(t, got[2].SettingJson)
}

func TestBuiltinMarkdown_ReturnsComponentGuide(t *testing.T) {
	rt := &Router{}
	dir, err := os.MkdirTemp("/tmp", "router-markdown-round3-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	rt.Center.BuiltinIntegrationsDir = dir
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "linux", "markdown"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "linux", "markdown", "README.md"), []byte("# Linux guide\n"), 0600))
	handler := func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "cate", Value: "linux"})
		rt.builtinMarkdown(c)
	}

	w := identityRequest(t, rt, handler, nil, "0", nil)

	var got string
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "# Linux guide\n", got)
}

func TestUserGroupMemberGetsByService_ReturnsAllMemberships(t *testing.T) {
	rt := identityRouter(t, &models.UserGroupMember{})
	members := []models.UserGroupMember{{GroupId: 10, UserId: 1}, {GroupId: 20, UserId: 2}}
	require.NoError(t, rt.Ctx.DB.Create(&members).Error)

	w := identityRequest(t, rt, rt.userGroupMemberGetsByService, nil, "0", nil)

	var got []models.UserGroupMember
	require.Empty(t, identityReply(t, w, &got))
	require.ElementsMatch(t, members, got)
}

func TestBusiGroupAlertingsGets_CountsOnlySelectedGroups(t *testing.T) {
	rt := identityRouter(t, &models.AlertCurEvent{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertCurEvent{
		{GroupId: 1}, {GroupId: 1}, {GroupId: 2},
	}).Error)

	w := identityRequest(t, rt, rt.busiGroupAlertingsGets, nil, "0?ids=1", nil)

	var got map[int64]int64
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, map[int64]int64{1: 2}, got)
}

func TestSsoConfigUpdate_UpsertsDisabledProviderSettingsLocally(t *testing.T) {
	for _, provider := range []string{"dingtalk", "feishu"} {
		t.Run(provider, func(t *testing.T) {
			rt := identityRouter(t, &models.SsoConfig{})
			rt.Sso = &sso.SsoClient{}
			for _, label := range []string{"Original login", "Updated login"} {
				request := models.SsoConfig{
					Name: provider, SettingJson: map[string]interface{}{"enable": false, "display_name": label},
				}

				w := identityRequest(t, rt, rt.ssoConfigUpdate, request, "0", nil)

				require.Empty(t, identityReply(t, w, nil))
				var stored []models.SsoConfig
				require.NoError(t, rt.Ctx.DB.Find(&stored).Error)
				require.Len(t, stored, 1)
				require.Equal(t, provider, stored[0].Name)
				expected, err := json.Marshal(request.SettingJson)
				require.NoError(t, err)
				require.JSONEq(t, string(expected), stored[0].Content)
				if provider == "dingtalk" {
					require.False(t, rt.Sso.DingTalk.Enable)
					require.Equal(t, label, rt.Sso.DingTalk.DingTalkConfig.DisplayName)
				} else {
					require.False(t, rt.Sso.FeiShu.Enable)
					require.Equal(t, label, rt.Sso.FeiShu.FeiShuConfig.DisplayName)
				}
			}
		})
	}
}
