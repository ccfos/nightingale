package router

import (
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAlertRuleGets_ReturnsOnlySelectedGroupAndResolvesUpdater(t *testing.T) {
	rt := ruleBehaviorRouter(t, &models.AlertRule{}, &models.User{}, &models.UserGroup{})
	selected := seedBehaviorRule(t, rt, "selected", 1)
	seedBehaviorRule(t, rt, "foreign", 2)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "creator", Nickname: "Rule Owner"}).Error)

	w := identityRequest(t, rt, rt.alertRuleGets, nil, "1", nil)

	var got []models.AlertRule
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, selected.Id, got[0].Id)
	require.Equal(t, "selected", got[0].Name)
	require.Equal(t, "Rule Owner", got[0].UpdateByNickname)
}

func TestRecordingRuleGets_ReturnsSelectedGroupWithParsedConfiguration(t *testing.T) {
	rt := identityRouter(t, &models.RecordingRule{}, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.RecordingRule{
		{Id: 1, GroupId: 1, Name: "selected", AppendTags: "env=prod", DatasourceIds: "[7]", UpdateBy: "alice"},
		{Id: 2, GroupId: 2, Name: "foreign"},
	}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "alice", Nickname: "Alice"}).Error)

	w := identityRequest(t, rt, rt.recordingRuleGets, nil, "1", nil)

	var got []models.RecordingRule
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "selected", got[0].Name)
	require.Equal(t, []string{"env=prod"}, got[0].AppendTagsJSON)
	require.Equal(t, []int64{7}, got[0].DatasourceIdsJson)
	require.Equal(t, "Alice", got[0].UpdateByNickname)
}

func TestRecordingRuleGetsByService_ExcludesDisabledRules(t *testing.T) {
	rt := identityRouter(t, &models.RecordingRule{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.RecordingRule{
		{Id: 1, Name: "enabled", Disabled: 0}, {Id: 2, Name: "disabled", Disabled: 1},
	}).Error)

	w := identityRequest(t, rt, rt.recordingRuleGetsByService, nil, "0", nil)

	var got []models.RecordingRule
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "enabled", got[0].Name)
}

func TestRecordingRuleGet_ReturnsRuleOrNotFound(t *testing.T) {
	rt := identityRouter(t, &models.RecordingRule{})
	require.NoError(t, rt.Ctx.DB.Create(&models.RecordingRule{Id: 1, Name: "latency", PromQl: "up"}).Error)
	handler := func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "rrid", Value: c.Param("id")})
		rt.recordingRuleGet(c)
	}

	w := identityRequest(t, rt, handler, nil, "1", nil)

	var got models.RecordingRule
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "latency", got.Name)
	require.Equal(t, "up", got.PromQl)
	missing := identityRequest(t, rt, handler, nil, "404", nil)
	require.Equal(t, http.StatusNotFound, missing.Code)
	require.Contains(t, missing.Body.String(), "No such recording rule")
}

func TestTaskTplGet_RejectsUnknownTemplate(t *testing.T) {
	rt := identityRouter(t, &models.TaskTpl{})
	handler := func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "tid", Value: c.Param("id")})
		rt.taskTplGet(c)
	}

	missing := identityRequest(t, rt, handler, nil, "404", nil)

	require.Equal(t, http.StatusNotFound, missing.Code)
	require.Contains(t, missing.Body.String(), "no such task template")
}

func TestTaskTplGetsByService_ReturnsAllTemplates(t *testing.T) {
	rt := identityRouter(t, &models.TaskTpl{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.TaskTpl{
		{Id: 1, Title: "first", UpdateAt: 100}, {Id: 2, Title: "second", UpdateAt: 200},
	}).Error)

	w := identityRequest(t, rt, rt.taskTplGetsByService, nil, "0", nil)

	var got []models.TaskTpl
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	require.ElementsMatch(t, []string{"first", "second"}, []string{got[0].Title, got[1].Title})
}

func TestTaskTplStatistics_ReturnsCountAndLatestUpdate(t *testing.T) {
	rt := identityRouter(t, &models.TaskTpl{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.TaskTpl{
		{Id: 1, Title: "first", UpdateAt: 100}, {Id: 2, Title: "second", UpdateAt: 200},
	}).Error)

	w := identityRequest(t, rt, rt.taskTplStatistics, nil, "0", nil)

	var got models.Statistics
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, int64(2), got.Total)
	require.Equal(t, int64(200), got.LastUpdated)
}

func TestNotifyTplGets_MarksBuiltinChannelsAndResolvesUpdater(t *testing.T) {
	rt := identityRouter(t, &models.NotifyTpl{}, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "alice", Nickname: "Alice"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.NotifyTpl{
		{Id: 1, Name: "Mail", Channel: models.Email, UpdateBy: "alice"},
		{Id: 2, Name: "Custom", Channel: "custom"},
	}).Error)

	w := identityRequest(t, rt, rt.notifyTplGets, nil, "0", nil)

	var got []models.NotifyTpl
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	byChannel := make(map[string]models.NotifyTpl)
	for _, tpl := range got {
		byChannel[tpl.Channel] = tpl
	}
	require.True(t, byChannel[models.Email].BuiltIn)
	require.Equal(t, "Alice", byChannel[models.Email].UpdateByNickname)
	require.False(t, byChannel["custom"].BuiltIn)
}

func TestNotifyChannelConfigGets_AppliesAllFilters(t *testing.T) {
	rt := identityRouter(t, &models.NotifyChannelConfig{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.NotifyChannelConfig{
		{ID: 1, Name: "Production mail", Ident: "mail", Enable: true},
		{ID: 2, Name: "Production mail disabled", Ident: "mail", Enable: false},
		{ID: 3, Name: "Production hook", Ident: "hook", Enable: true},
	}).Error)

	w := identityRequest(t, rt, rt.notifyChannelConfigGets, nil, "0?name=Production+mail&ident=mail&enabled=1", nil)

	var got []models.NotifyChannelConfig
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].ID)
	require.Equal(t, "Production mail", got[0].Name)
}

func TestNotifyRulesGetByService_OnlyReturnsEnabledRules(t *testing.T) {
	rt := identityRouter(t, &models.NotifyRule{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.NotifyRule{
		{ID: 1, Name: "enabled", Enable: true}, {ID: 2, Name: "disabled", Enable: false},
	}).Error)

	w := identityRequest(t, rt, rt.notifyRulesGetByService, nil, "0", nil)

	var got []models.NotifyRule
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "enabled", got[0].Name)
}

func TestServersGet_ReturnsAllInstancesInNameOrder(t *testing.T) {
	rt := identityRouter(t, &models.AlertingEngines{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertingEngines{
		{Instance: "engine-b", EngineCluster: "prod"}, {Instance: "engine-a", EngineCluster: "prod"},
	}).Error)

	w := identityRequest(t, rt, rt.serversGet, nil, "0", nil)

	var got []models.AlertingEngines
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	require.Equal(t, "engine-a", got[0].Instance)
	require.Equal(t, "engine-b", got[1].Instance)
}

func TestServerClustersGet_ReturnsDistinctNonemptyClusters(t *testing.T) {
	rt := identityRouter(t, &models.AlertingEngines{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertingEngines{
		{Instance: "engine-b", EngineCluster: "prod"}, {Instance: "engine-a", EngineCluster: "prod"},
		{Instance: "engine-c", EngineCluster: "stage"}, {Instance: "unassigned"},
	}).Error)

	w := identityRequest(t, rt, rt.serverClustersGet, nil, "0", nil)

	var got []string
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, []string{"prod", "stage"}, got)
}
