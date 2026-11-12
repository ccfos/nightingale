package router

import (
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/chat"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAILLMConfigAdd_StoresConfigurationWithCreator(t *testing.T) {
	cases := []struct {
		name    string
		service bool
		creator string
	}{
		{name: "user", creator: "alice"},
		{name: "service", service: true, creator: "system"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.AILLMConfig{})
			form := models.AILLMConfig{
				Name:    " local ",
				APIType: "openai",
				APIURL:  "https://example.invalid/v1",
				APIKey:  "test-secret-key",
				Model:   "test-model",
			}
			handler := rt.aiLLMConfigAdd
			if tc.service {
				handler = rt.aiLLMConfigAddByService
			}

			w := identityRequest(t, rt, handler, form, "1", &models.User{Username: "alice"})

			var id int64
			require.Empty(t, identityReply(t, w, &id))
			require.Positive(t, id)
			var got models.AILLMConfig
			require.NoError(t, rt.Ctx.DB.First(&got, id).Error)
			require.Equal(t, "local", got.Name)
			require.Equal(t, form.APIURL, got.APIURL)
			require.Equal(t, form.APIKey, got.APIKey)
			require.Equal(t, tc.creator, got.CreatedBy)
			require.Equal(t, tc.creator, got.UpdatedBy)
		})
	}
}

func TestAILLMConfigReads_MasksKeysForUIAndPreservesServiceValues(t *testing.T) {
	rt := identityRouter(t, &models.AILLMConfig{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AILLMConfig{Id: 1, Name: "local", APIKey: "0123456789abcdef"}).Error)
	cases := []struct {
		name     string
		list     bool
		service  bool
		expected string
	}{
		{name: "UI detail", expected: "0123****cdef"},
		{name: "UI list", list: true, expected: "0123****cdef"},
		{name: "service detail", service: true, expected: "0123456789abcdef"},
		{name: "service list", list: true, service: true, expected: "0123456789abcdef"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := func(c *gin.Context) {
				if tc.service {
					c.Request.URL.Path = "/v1/n9e/ai-configs"
				}
				if tc.list {
					rt.aiLLMConfigGets(c)
				} else {
					rt.aiLLMConfigGet(c)
				}
			}

			w := identityRequest(t, rt, handler, nil, "1", nil)

			var got models.AILLMConfig
			if tc.list {
				var list []models.AILLMConfig
				require.Empty(t, identityReply(t, w, &list))
				require.Len(t, list, 1)
				got = list[0]
			} else {
				require.Empty(t, identityReply(t, w, &got))
			}
			require.Equal(t, tc.expected, got.APIKey)
			var saved models.AILLMConfig
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			require.Equal(t, "0123456789abcdef", saved.APIKey)
		})
	}
}

func TestAILLMConfigPut_PreservesMaskedOrOmittedKeysAndAcceptsReplacement(t *testing.T) {
	cases := []struct {
		name     string
		key      string
		service  bool
		expected string
	}{
		{name: "omitted key", expected: "0123456789abcdef"},
		{name: "masked key", key: "0123****cdef", expected: "0123456789abcdef"},
		{name: "new key", key: "replacement-key", expected: "replacement-key"},
		{name: "service omitted key", service: true, expected: "0123456789abcdef"},
		{name: "service masked key", service: true, key: "0123****cdef", expected: "0123456789abcdef"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.AILLMConfig{})
			require.NoError(t, rt.Ctx.DB.Create(&models.AILLMConfig{
				Id: 1, Name: "local", APIKey: "0123456789abcdef", CreatedBy: "creator", CreatedAt: 123,
			}).Error)
			form := models.AILLMConfig{
				Name: "updated", APIType: "openai", APIURL: "https://example.invalid/v1", APIKey: tc.key, Model: "test-model",
			}
			handler := rt.aiLLMConfigPut
			if tc.service {
				handler = rt.aiLLMConfigPutByService
			}

			w := identityRequest(t, rt, handler, form, "1", &models.User{Username: "alice"})

			require.Empty(t, identityReply(t, w, nil))
			var got models.AILLMConfig
			require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
			require.Equal(t, tc.expected, got.APIKey)
			require.Equal(t, "updated", got.Name)
			require.Equal(t, "creator", got.CreatedBy)
			require.Equal(t, int64(123), got.CreatedAt)
			expectedUser := "alice"
			if tc.service {
				expectedUser = "system"
			}
			require.Equal(t, expectedUser, got.UpdatedBy)
		})
	}
}

func TestAILLMConfigPut_InvalidConfigurationDoesNotWrite(t *testing.T) {
	rt := identityRouter(t, &models.AILLMConfig{})
	original := models.AILLMConfig{Id: 1, Name: "original", APIKey: "key"}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)

	w := identityRequest(t, rt, rt.aiLLMConfigPut, models.AILLMConfig{Name: "updated"}, "1", nil)

	require.Contains(t, identityReply(t, w, nil), "api_type is required")
	var got models.AILLMConfig
	require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
	require.Equal(t, original, got)
}

func TestAILLMConfigDel_RemovesOnlySelectedConfiguration(t *testing.T) {
	rt := identityRouter(t, &models.AILLMConfig{}, &models.AIAgent{}, &models.Configs{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AILLMConfig{{Id: 1, Name: "one"}, {Id: 2, Name: "two"}}).Error)

	w := identityRequest(t, rt, rt.aiLLMConfigDel, nil, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var got []models.AILLMConfig
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, int64(2), got[0].Id)
}

func TestAILLMConfigHandlers_MissingConfigurationReturnsNotFound(t *testing.T) {
	rt := identityRouter(t, &models.AILLMConfig{})
	cases := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "get", handler: rt.aiLLMConfigGet},
		{name: "update", handler: rt.aiLLMConfigPut},
		{name: "service update", handler: rt.aiLLMConfigPutByService},
		{name: "delete", handler: rt.aiLLMConfigDel},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := identityRequest(t, rt, tc.handler, nil, "404", nil)

			require.Equal(t, http.StatusNotFound, w.Code)
			require.Contains(t, w.Body.String(), "ai llm config not found")
		})
	}
}

func TestAIAgentGets_ResolvesLLMConfigurationNames(t *testing.T) {
	rt := identityRouter(t, &models.AIAgent{}, &models.AILLMConfig{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AILLMConfig{Id: 7, Name: "local model"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AIAgent{
		{Id: 1, Name: "configured", LLMConfigId: 7}, {Id: 2, Name: "deleted config", LLMConfigId: 404},
	}).Error)

	w := identityRequest(t, rt, rt.aiAgentGets, nil, "1", nil)

	var got []models.AIAgent
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	require.Equal(t, "local model", got[0].LLMConfigName)
	require.Empty(t, got[1].LLMConfigName)
}

func TestAIAgentGet_ReturnsAgentWithConfigurationName(t *testing.T) {
	rt := identityRouter(t, &models.AIAgent{}, &models.AILLMConfig{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AILLMConfig{Id: 7, Name: "local model"}).Error)
	require.NoError(t,
		rt.Ctx.DB.Create(&models.AIAgent{Id: 1, Name: "configured", LLMConfigId: 7, SkillIds: []int64{3}}).Error)

	w := identityRequest(t, rt, rt.aiAgentGet, nil, "1", nil)

	var got models.AIAgent
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "configured", got.Name)
	require.Equal(t, "local model", got.LLMConfigName)
	require.Equal(t, []int64{3}, got.SkillIds)
}

func TestAIAgentAdd_AssignsCreatorAndReturnsID(t *testing.T) {
	rt := identityRouter(t, &models.AIAgent{})
	form := models.AIAgent{Name: " agent ", LLMConfigId: 7, SkillIds: []int64{3}, CreatedBy: "forged"}

	w := identityRequest(t, rt, rt.aiAgentAdd, form, "1", &models.User{Username: "alice"})

	var id int64
	require.Empty(t, identityReply(t, w, &id))
	require.Positive(t, id)
	var got models.AIAgent
	require.NoError(t, rt.Ctx.DB.First(&got, id).Error)
	require.Equal(t, "agent", got.Name)
	require.Equal(t, form.SkillIds, got.SkillIds)
	require.Equal(t, "alice", got.CreatedBy)
}

func TestAIAgentPut_UpdatesCapabilitiesAndPreservesCreation(t *testing.T) {
	rt := identityRouter(t, &models.AIAgent{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AIAgent{Id: 1, Name: "old", CreatedBy: "creator", CreatedAt: 123}).Error)
	form := models.AIAgent{
		Name: "updated", LLMConfigId: 7, SkillIds: []int64{3}, MCPServerIds: []int64{9}, Enabled: true, UseCase: "chat",
	}

	w := identityRequest(t, rt, rt.aiAgentPut, form, "1", &models.User{Username: "alice"})

	require.Empty(t, identityReply(t, w, nil))
	var got models.AIAgent
	require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
	require.Equal(t, "updated", got.Name)
	require.Equal(t, form.SkillIds, got.SkillIds)
	require.Equal(t, form.MCPServerIds, got.MCPServerIds)
	require.True(t, got.Enabled)
	require.Equal(t, "creator", got.CreatedBy)
	require.Equal(t, int64(123), got.CreatedAt)
	require.Equal(t, "alice", got.UpdatedBy)
}

func TestAIAgentDel_PreservesOtherAgents(t *testing.T) {
	rt := identityRouter(t, &models.AIAgent{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AIAgent{{Id: 1, Name: "one"}, {Id: 2, Name: "two"}}).Error)

	w := identityRequest(t, rt, rt.aiAgentDel, nil, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var got []models.AIAgent
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, "two", got[0].Name)
}

func TestBuildSkillConfigForAgent_PreloadsOnlyBoundEnabledSkills(t *testing.T) {
	rt := identityRouter(t, &models.AISkill{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AISkill{
		{Id: 1, Name: "enabled", Enabled: true}, {Id: 2, Name: "disabled", Enabled: false},
		{Id: 3, Name: "unbound", Enabled: true},
	}).Error)

	cfg := rt.buildSkillConfigForAgent(&models.AIAgent{SkillIds: []int64{1, 2, 404}})

	require.Equal(t, []string{"enabled"}, cfg.SkillNames)
	empty := rt.buildSkillConfigForAgent(&models.AIAgent{})
	require.Empty(t, empty.SkillNames)
}

func TestResolveSkillConfig_ActionRequirementsOverrideAgentBindings(t *testing.T) {
	rt := identityRouter(t, &models.AISkill{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AISkill{Id: 1, Name: "bound", Enabled: true}).Error)
	cases := []struct {
		name     string
		handler  *chat.ActionHandler
		expected []string
	}{
		{name: "no action override", expected: []string{"bound"}},
		{
			name: "explicit requirements", expected: []string{"required"},
			handler: &chat.ActionHandler{RequiredSkills: func(*chat.AIChatRequest) []string { return []string{"required"} }},
		},
		{
			name:    "explicit empty requirements",
			handler: &chat.ActionHandler{RequiredSkills: func(*chat.AIChatRequest) []string { return nil }},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rt.resolveSkillConfig(tc.handler, &chat.AIChatRequest{}, &models.AIAgent{SkillIds: []int64{1}})

			require.Equal(t, tc.expected, got.SkillNames)
		})
	}
}

func TestHiddenSkillNamesForUser_RespectsTeamAccessAndFailsClosedForDeletedUsers(t *testing.T) {
	cases := []struct {
		name     string
		userID   int64
		expected []string
	}{
		{name: "member", userID: 1, expected: []string{"foreign"}},
		{name: "admin", userID: 2},
		{name: "deleted user", userID: 404, expected: []string{"team", "foreign"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.AISkill{}, &models.User{}, &models.UserGroupMember{})
			require.NoError(t, rt.Ctx.DB.Create(&[]models.User{
				{Id: 1, Username: "member"}, {Id: 2, Username: "admin", Roles: "Admin"},
			}).Error)
			require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{UserId: 1, GroupId: 10}).Error)
			require.NoError(t, rt.Ctx.DB.Create(&[]models.AISkill{
				{Id: 1, Name: "public"},
				{Id: 2, Name: "team", Private: 1, UserGroupIds: []int64{10}},
				{Id: 3, Name: "foreign", Private: 1, UserGroupIds: []int64{20}},
			}).Error)

			hidden, denyAll := rt.hiddenSkillNamesForUser(tc.userID)

			require.False(t, denyAll)
			require.ElementsMatch(t, tc.expected, hidden)
		})
	}
}

func TestHiddenSkillNamesForUser_DeniesAllWhenPrivateSkillEnumerationFails(t *testing.T) {
	rt := identityRouter(t, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "member"}).Error)

	hidden, denyAll := rt.hiddenSkillNamesForUser(1)

	require.True(t, denyAll)
	require.Nil(t, hidden)
}
