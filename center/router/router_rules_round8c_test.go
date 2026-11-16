package router

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestR8cAlertRuleTryRunLegacyNotificationValidation(t *testing.T) {
	for _, tc := range []struct {
		name             string
		channels, groups []string
		version          int
		want             string
	}{
		{"no channels", nil, nil, 0, "no notify channels selected"},
		{"no groups", []string{"email"}, nil, 0, "no notify groups selected"},
		{"unknown group", []string{"email"}, []string{"bad", "404"}, 0, "notification test ok"},
		{"new notification", nil, nil, 1, "notification test ok"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := r8cRouter(t, &models.AlertHisEvent{}, &models.UserGroup{}, &models.UserGroupMember{}, &models.User{})
			rt.UserCache = &memsto.UserCacheType{}
			rt.UserGroupCache = &memsto.UserGroupCacheType{}
			require.NoError(t, rt.Ctx.DB.Create(&models.AlertHisEvent{Id: 1, Tags: "env=prod"}).Error)
			form := AlertRuleTryRunForm{EventId: 1, AlertRuleConfig: models.AlertRule{NotifyVersion: tc.version, NotifyChannelsJSON: tc.channels, NotifyGroupsJSON: tc.groups}}
			w := r8cRequest(t, rt.alertRuleNotifyTryRun, form, "/1")
			require.Equal(t, 200, w.Code)
			require.Contains(t, w.Body.String(), tc.want)
		})
	}
}
func TestR8cTimezonesAndEventTimeRange(t *testing.T) {
	rt := &Router{}
	var zones []string
	r8cData(t, r8cRequest(t, rt.timezonesGet, nil, "/1"), &zones)
	require.Contains(t, zones, "UTC")
	require.Contains(t, zones, "Asia/Shanghai")
	require.Contains(t, zones, "Asia/Kolkata")
	require.NotContains(t, zones, "Asia/Singapore")
	c := queryRound6Context()
	c.Request.URL.RawQuery = "stime=30&etime=20"
	s, e := GetAlertCueEventTimeRange(c)
	require.Equal(t, int64(20), e)
	require.Equal(t, e-30*24*3600, s)
}
func TestR8cRuleDatasourceResolution(t *testing.T) {
	rt := r8cRouter(t, &models.AlertRule{}, &models.UserGroup{}, &models.User{}, &models.AlertCurEvent{})
	rule := seedBehaviorRule(t, rt, "one", 1)
	require.NoError(t, rt.Ctx.DB.Model(&rule).Update("datasource_queries", `[{"match_type":0,"op":"in","values":[7]}]`).Error)
	rt.DatasourceCache = &memsto.DatasourceCacheType{}
	rt.DatasourceCache.Set(map[int64]*models.Datasource{7: {Id: 7, PluginType: "prometheus", Name: "metrics"}}, 1, 1)
	for _, h := range []gin.HandlerFunc{rt.alertRuleGetsByGids, rt.alertRulesGetByService} {
		var rules []models.AlertRule
		r8cData(t, r8cRequest(t, h, nil, "/1"), &rules)
		require.Len(t, rules, 1)
		require.Equal(t, []int64{7}, rules[0].DatasourceIdsJson)
	}
}

func TestR8cRuleCloneReportsDatabaseRejectionAndMissingIDs(t *testing.T) {
	rt := r8cRouter(t, &models.AlertRule{}, &models.BusiGroup{})
	require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroup{Id: 2, Name: "destination"}).Error)
	rule := seedBehaviorRule(t, rt, "source", 1)
	require.NoError(t, rt.Ctx.DB.Callback().Create().Before("gorm:create").Register("r8c_clone_fail", func(db *gorm.DB) { db.AddError(errors.New("clone storage rejected")) }))
	var result map[string]string
	r8cData(t, r8cRequest(t, rt.batchAlertRuleClone, alertBatchCloneForm{RuleIds: []int64{rule.Id, 404}, Bgids: []int64{2}}, "/1"), &result)
	require.Contains(t, result[fmt.Sprintf("%d-2", rule.Id)], "clone storage rejected")
	require.Equal(t, "alert rule not found", result["404-2"])
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.AlertRule{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}
func TestR8cHostRuleTargetsDatabaseFallback(t *testing.T) {
	rt := r8cRouter(t, &models.AlertRule{}, &models.Target{}, &models.TargetBusiGroup{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertRule{Id: 1, Name: "host", Prod: "host", RuleConfig: `{"queries":[]}`}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Target{{Ident: "local", EngineName: "one"}, {Ident: "foreign", EngineName: "two"}}).Error)
	var result map[string]map[int64][]string
	r8cData(t, r8cRequest(t, rt.targetsOfAlertRule, nil, "/1?engine_name=one"), &result)
	require.Equal(t, map[string]map[int64][]string{"one": {1: {"local"}}}, result)
}

func TestR8cUpdateTriggersPreservesQueryAndValidatesSeverity(t *testing.T) {
	rt := r8cRouter(t, &models.AlertRule{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertRule{Id: 1, Name: "host", Prod: "host", Cate: "host", RuleConfig: `{"queries":[{"key":"hosts","op":"==","values":["node"]}],"triggers":[{"type":"target_miss","severity":2}]}`}).Error)
	for _, severity := range []int{1, 4} {
		form := alertRuleFieldForm{Ids: []int64{1}, Action: "update_triggers", Fields: map[string]interface{}{"triggers": []map[string]interface{}{{"type": "target_miss", "severity": severity}}}}
		w := r8cRequest(t, rt.alertRulePutFields, form, "/1")
		if severity == 1 {
			r8cData(t, w, nil)
		} else {
			require.Equal(t, 400, w.Code)
			require.Contains(t, w.Body.String(), "severity")
		}
		var saved models.AlertRule
		require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
		var config models.HostRuleConfig
		require.NoError(t, json.Unmarshal([]byte(saved.RuleConfig), &config))
		require.Len(t, config.Queries, 1)
		require.Equal(t, "hosts", config.Queries[0].Key)
		require.Equal(t, 1, config.Triggers[0].Severity)
	}
}
