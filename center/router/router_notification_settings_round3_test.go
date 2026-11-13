package router

import (
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func subscriptionSettingsRouter(t *testing.T) *Router {
	t.Helper()
	rt := muteSubscribeRouter(t, &models.AlertHisEvent{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertHisEvent{
		Id: 1, RuleId: 5, DatasourceId: 3, Severity: 2, Tags: "env=prod", GroupName: "production",
	}).Error)
	rt.UserCache = &memsto.UserCacheType{}
	rt.UserCache.Set(map[int64]*models.User{1: {Id: 1, Username: "alice", Email: "alice@example.invalid"}}, 1, 1, 0, 0)
	rt.UserGroupCache = &memsto.UserGroupCacheType{}
	rt.UserGroupCache.Set(map[int64]*models.UserGroup{10: {Id: 10, UserIds: []int64{1}}}, 1, 1)
	return rt
}

func TestAlertSubscribeTryRun_ValidatesLegacyRecipientsWithoutSending(t *testing.T) {
	rt := subscriptionSettingsRouter(t)
	sub := models.AlertSubscribe{
		RuleIds: []int64{5}, DatasourceIdsJson: []int64{3}, SeveritiesJson: []int{2},
		RedefineChannels: 1, NewChannels: models.Email, UserGroupIds: "10",
	}

	w := muteSubscribeRequest(t, rt.alertSubscribeTryRun,
		SubscribeTryRunForm{EventId: 1, SubscribeConfig: sub}, "/groups/1/objects/0/0", nil)

	var result string
	require.Empty(t, muteSubscribeReply(t, w, &result))
	require.Equal(t, "event match subscribe and notify settings ok", result)
	var stored models.AlertHisEvent
	require.NoError(t, rt.Ctx.DB.First(&stored, 1).Error)
	require.Empty(t, stored.NotifyChannels)
}

func TestAlertSubscribeTryRun_ReportsMissingLegacyRecipientToken(t *testing.T) {
	rt := subscriptionSettingsRouter(t)
	sub := models.AlertSubscribe{
		RedefineChannels: 1, NewChannels: models.Wecom, UserGroupIds: "10", SeveritiesJson: []int{2},
	}

	w := muteSubscribeRequest(t, rt.alertSubscribeTryRun,
		SubscribeTryRunForm{EventId: 1, SubscribeConfig: sub}, "/groups/1/objects/0/0", nil)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "all users missing notify channel configurations")
	require.Contains(t, w.Body.String(), models.Wecom)
}

func TestAlertSubscribeTryRun_RequiresNotificationRuleSelection(t *testing.T) {
	rt := subscriptionSettingsRouter(t)

	w := muteSubscribeRequest(t, rt.alertSubscribeTryRun,
		SubscribeTryRunForm{EventId: 1, SubscribeConfig: models.AlertSubscribe{NotifyVersion: 1, SeveritiesJson: []int{2}}},
		"/groups/1/objects/0/0", nil)

	require.Contains(t, identityReply(t, w, nil), "no notify rules selected")
}

func TestLegacyNotifyMissingTokens_OnlyReportsUnconfiguredBuiltinChannels(t *testing.T) {
	rt := subscriptionSettingsRouter(t)
	cfg := &models.AlertRule{
		NotifyGroupsJSON: []string{"invalid", "10"}, NotifyChannelsJSON: []string{models.Email, "custom", models.Wecom},
	}

	missing := rt.legacyNotifyMissingTokens(cfg)

	require.Equal(t, []string{models.Wecom}, missing)
}

func TestMessageTemplateGets_FiltersAndResolvesUpdater(t *testing.T) {
	rt := identityRouter(t, &models.MessageTemplate{}, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "alice", Nickname: "Alice"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.MessageTemplate{
		{ID: 1, Name: "Mail", Ident: "mail", UpdateBy: "alice", Content: map[string]string{"body": "Alert"}},
		{ID: 2, Name: "Other", Ident: "hook"},
	}).Error)

	w := identityRequest(t, rt, rt.messageTemplateGets, nil, "0?id=1&name=Mail&ident=mail", nil)

	var got []models.MessageTemplate
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].ID)
	require.Equal(t, map[string]string{"body": "Alert"}, got[0].Content)
	require.Equal(t, "Alice", got[0].UpdateByNickname)
}

func TestNotifyChannelsGet_ResolvesUpdaterWithoutSending(t *testing.T) {
	rt := identityRouter(t, &models.NotifyChannelConfig{}, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "alice", Nickname: "Alice"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.NotifyChannelConfig{ID: 1, Name: "Mail", UpdateBy: "alice"}).Error)

	w := identityRequest(t, rt, rt.notifyChannelsGet, nil, "0", nil)

	var got []models.NotifyChannelConfig
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "Mail", got[0].Name)
	require.Equal(t, "Alice", got[0].UpdateByNickname)
}
