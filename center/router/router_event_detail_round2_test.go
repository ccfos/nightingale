package router

import (
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestAlertCurEventGet_AnonymousDetailIncludesNotificationRules(t *testing.T) {
	rt := identityRouter(t, &models.AlertCurEvent{}, &models.NotifyRule{}, &models.AlertRule{})
	rt.Center.AnonymousAccess.AlertDetail = true
	require.NoError(t, rt.Ctx.DB.Create(&models.NotifyRule{ID: 9, Name: "oncall"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertCurEvent{
		Id: 1, RuleId: 7, RuleName: "CPU high", TriggerTime: 123, NotifyRuleIds: []int64{9},
	}).Error)

	w := eventListingRequest(t, rt.alertCurEventGet, nil, "", nil)

	var got models.AlertCurEvent
	require.Empty(t, eventListingReply(t, w, &got))
	require.Equal(t, int64(1), got.Id)
	require.Equal(t, "CPU high", got.RuleName)
	require.Equal(t, int64(123), got.LastEvalTime)
	require.Equal(t, 1, got.NotifyVersion)
	require.Len(t, got.NotifyRules, 1)
	require.Equal(t, "oncall", got.NotifyRules[0].Name)
}

func TestAlertHisEventGet_ValidShareTokenReturnsConvertedEvent(t *testing.T) {
	rt := identityRouter(t, &models.AlertHisEvent{}, &models.NotifyRule{}, &models.AlertRule{}, &models.SourceToken{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertHisEvent{
		Id: 1, RuleId: 7, RuleName: "CPU high", TriggerTime: 123, LastEvalTime: 150, RuleConfig: "{}", Annotations: "{}",
	}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.SourceToken{
		SourceType: "event", SourceId: "1", Token: "share",
	}).Error)

	w := eventListingRequest(t, rt.alertHisEventGet, nil, "?__token=share", nil)

	var got models.AlertCurEvent
	require.Empty(t, eventListingReply(t, w, &got))
	require.Equal(t, int64(1), got.Id)
	require.Equal(t, "CPU high", got.RuleName)
	require.Equal(t, int64(123), got.TriggerTime)
	require.Equal(t, int64(150), got.LastEvalTime)
}

func TestAlertHisEventGet_RejectsInvalidTokenAndMissingEvent(t *testing.T) {
	rt := identityRouter(t, &models.AlertHisEvent{}, &models.SourceToken{})

	missing := eventListingRequest(t, rt.alertHisEventGet, nil, "", nil)

	require.Equal(t, http.StatusNotFound, missing.Code)
	require.Contains(t, missing.Body.String(), "No such alert event")
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertHisEvent{Id: 1, GroupId: 1}).Error)

	denied := eventListingRequest(t, rt.alertHisEventGet, nil, "?__token=invalid", nil)

	require.Equal(t, http.StatusUnauthorized, denied.Code)
	require.NotContains(t, denied.Body.String(), "rule_config")
}

func TestAlertCurEventsGetByRid_FiltersRuleAndDatasource(t *testing.T) {
	rt := identityRouter(t, &models.AlertCurEvent{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertCurEvent{
		{Id: 1, RuleId: 7, DatasourceId: 9, RuleConfig: "{}", Annotations: "{}"},
		{Id: 2, RuleId: 7, DatasourceId: 8, RuleConfig: "{}", Annotations: "{}"},
		{Id: 3, RuleId: 8, DatasourceId: 9, RuleConfig: "{}", Annotations: "{}"},
	}).Error)

	w := identityRequest(t, rt, rt.alertCurEventsGetByRid, nil, "1?rid=7&dsid=9", nil)

	var got []models.AlertCurEvent
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].Id)
}

func TestAlertCurEventsCardDetails_FillsNotificationGroupNames(t *testing.T) {
	rt := identityRouter(t, &models.AlertCurEvent{}, &models.UserGroup{})
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroup{Id: 9, Name: "oncall"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertCurEvent{
		{Id: 1, NotifyGroups: "9", RuleConfig: "{}", Annotations: "{}"},
		{Id: 2, RuleConfig: "{}", Annotations: "{}"},
	}).Error)

	w := identityRequest(t, rt, rt.alertCurEventsCardDetails, idsForm{Ids: []int64{1, 404}}, "1", nil)

	var got []models.AlertCurEvent
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].Id)
	require.Len(t, got[0].NotifyGroupsJSON, 1)
	require.Equal(t, "oncall", got[0].NotifyGroupsObj[0].Name)
}
