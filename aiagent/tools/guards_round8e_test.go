package tools

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

// All resource tools bind identity from the session before inspecting arguments.
func TestR8eResourceIdentityGuards(t *testing.T) {
	handlers := map[string]aiagent.BuiltinToolFunc{
		"list rules": listAlertRules, "legacy rules": listLegacyNotifyAlertRules, "rule detail": getAlertRuleDetail, "create rule": createAlertRule, "update rule": updateAlertRule, "import YAML": importPromRuleYAML, "import rule template": importAlertRuleTemplate,
		"list dashboards": listDashboards, "dashboard detail": getDashboardDetail, "create dashboard": createDashboard, "import dashboard": importDashboardTemplate, "update dashboard": updateDashboard, "analyze dashboard": getDashboardDataTool,
		"create mute": createAlertMute, "update mute": updateAlertMute, "list mutes": listAlertMutes, "mute detail": getAlertMuteDetail,
		"create notification": createNotifyRule, "update notification": updateNotifyRule, "list notifications": listNotifyRules, "notification detail": getNotifyRuleDetail,
		"get skill": getSkill, "create skill": createSkill, "update skill": updateSkill,
		"task status": getTaskStatus, "task records": listTaskRecords, "host status": getTargetRealtimeStatus, "host metrics": queryHostMetricsWindow, "neighbors": listNeighborTargets,
	}
	deps := newResourceTestDeps(t, &models.RoleOperation{})
	for name, handler := range handlers {
		t.Run(name, func(t *testing.T) {
			for _, id := range []string{"", "bogus", "999"} {
				out, err := handler(context.Background(), deps, nil, map[string]string{"user_id": id})
				require.Empty(t, out)
				if id == "999" {
					require.ErrorContains(t, err, "user not found")
				} else {
					require.ErrorContains(t, err, "user_id not found")
				}
			}
		})
	}
}

func TestR8eResourceOperationGuards(t *testing.T) {
	deps := newResourceTestDeps(t, &models.RoleOperation{})
	handlers := []aiagent.BuiltinToolFunc{listAlertRules, listLegacyNotifyAlertRules, getAlertRuleDetail, importPromRuleYAML, importAlertRuleTemplate, listDashboards, getDashboardDetail, importDashboardTemplate, createNotifyRule, updateNotifyRule, listNotifyRules, getNotifyRuleDetail, getTaskStatus, listTaskRecords}
	for i, handler := range handlers {
		out, err := handler(context.Background(), deps, nil, map[string]string{"user_id": "2"})
		require.Empty(t, out, "handler %d", i)
		require.ErrorContains(t, err, "forbidden: no permission", "handler %d", i)
	}
}

func TestR8eResourceReadFailures(t *testing.T) {
	cases := []struct {
		name string
		fn   aiagent.BuiltinToolFunc
		args map[string]interface{}
		want string
	}{
		{"rules", listAlertRules, nil, "failed to query alert rules"}, {"legacy", listLegacyNotifyAlertRules, nil, "failed to query legacy alert rules"},
		{"rule", getAlertRuleDetail, map[string]interface{}{"id": "1"}, "failed to get alert rule"}, {"edit rule", updateAlertRule, map[string]interface{}{"id": "1"}, "failed to get alert rule"},
		{"dashboards", listDashboards, nil, "failed to query dashboards"}, {"dashboard", getDashboardDetail, map[string]interface{}{"id": "1"}, "failed to get dashboard"}, {"analyze", getDashboardDataTool, map[string]interface{}{"id": "1"}, "failed to get dashboard"}, {"edit dashboard", updateDashboard, map[string]interface{}{"id": "1"}, "failed to get dashboard"},
		{"mutes", listAlertMutes, nil, "failed to query alert mutes"}, {"mute", getAlertMuteDetail, map[string]interface{}{"id": "1"}, "failed to get alert mute"}, {"edit mute", updateAlertMute, map[string]interface{}{"id": "1"}, "failed to get alert mute"},
		{"notifications", listNotifyRules, nil, "failed to query notify rules"}, {"notification", getNotifyRuleDetail, map[string]interface{}{"id": "1"}, "failed to get notify rule"}, {"edit notification", updateNotifyRule, map[string]interface{}{"id": "1"}, "failed to get notify rule"},
		{"skill", getSkill, map[string]interface{}{"name": "triage"}, "failed to load skill"}, {"edit skill", updateSkill, map[string]interface{}{"name": "triage"}, "failed to load skill"},
		{"task", getTaskStatus, map[string]interface{}{"task_id": "1"}, "failed to get task record"}, {"tasks", listTaskRecords, nil, "failed to count task records"},
		{"host", getTargetRealtimeStatus, map[string]interface{}{"ident": "web"}, "failed to get target"}, {"neighbors", listNeighborTargets, map[string]interface{}{"ident": "web"}, "failed to get target groups"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t)
			out, err := tc.fn(context.Background(), deps, tc.args, map[string]string{"user_id": "1"})
			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

// A failed membership query must fail closed even when the resource exists.
func TestR8eMembershipReadFailures(t *testing.T) {
	deps := newResourceTestDeps(t, &models.RoleOperation{}, &models.AlertRule{}, &models.Board{}, &models.AlertMute{}, &models.NotifyRule{}, &models.TaskRecord{}, &models.Target{}, &models.TargetBusiGroup{}, &models.AISkill{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.AlertRule{Id: 7, GroupId: 1}).Error)
	require.NoError(t, db.Create(&models.Board{Id: 7, GroupId: 1}).Error)
	require.NoError(t, (&models.AlertMute{Id: 7, GroupId: 1, Btime: 1, Etime: 2}).Add(deps.DBCtx))
	require.NoError(t, db.Create(&models.NotifyRule{ID: 7}).Error)
	require.NoError(t, db.Create(&models.TaskRecord{Id: 7, GroupId: 1}).Error)
	require.NoError(t, db.Create(&models.Target{Ident: "web"}).Error)
	require.NoError(t, db.Create(&models.TargetBusiGroup{TargetIdent: "web", GroupId: 1}).Error)
	require.NoError(t, db.Create(&models.AISkill{Name: "triage", CreatedBy: "alice"}).Error)
	for _, op := range []string{PermAlertRules, PermDashboards, PermAlertMutes, PermNotificationRules, PermNotificationRulesPut, PermNotificationRulesAdd, PermJobTasks, PermAISkills} {
		require.NoError(t, db.Create(&models.RoleOperation{RoleName: "Standard", Operation: op}).Error)
	}
	handlers := []aiagent.BuiltinToolFunc{listAlertRules, listLegacyNotifyAlertRules, listDashboards, listAlertMutes, getAlertRuleDetail, getDashboardDetail, getDashboardDataTool, getAlertMuteDetail, listNotifyRules, getNotifyRuleDetail, updateNotifyRule, getTaskStatus, listTaskRecords, getTargetRealtimeStatus, queryHostMetricsWindow, listNeighborTargets, updateSkill, createNotifyRule}
	for i, fn := range handlers {
		out, err := fn(context.Background(), deps, map[string]interface{}{"id": "7", "task_id": "7", "ident": "web", "name": "triage", "config": `{"name":"notify","user_group_ids":[1]}`}, map[string]string{"user_id": "2"})
		require.Empty(t, out, "handler %d", i)
		require.ErrorContains(t, err, "group", "handler %d", i)
	}
}

func TestR8eMissingResourceResults(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{}, &models.AlertMute{}, &models.Board{}, &models.NotifyRule{})
	for _, fn := range []aiagent.BuiltinToolFunc{getAlertRuleDetail, updateAlertRule, getAlertMuteDetail, updateAlertMute, getDashboardDetail, updateDashboard, updateNotifyRule} {
		out, err := fn(context.Background(), deps, map[string]interface{}{"id": "777"}, map[string]string{"user_id": "1"})
		require.NoError(t, err)
		require.Contains(t, out, "not found")
		require.Contains(t, out, "777")
	}
	for _, fn := range []aiagent.BuiltinToolFunc{getAlertRuleDetail, updateAlertRule, getAlertMuteDetail, updateAlertMute, updateDashboard, getNotifyRuleDetail, updateNotifyRule, getTaskStatus, getTargetRealtimeStatus, queryHostMetricsWindow, listNeighborTargets, getSkill, updateSkill} {
		out, err := fn(context.Background(), deps, nil, map[string]string{"user_id": "1"})
		require.Empty(t, out)
		require.ErrorContains(t, err, "is required")
	}
}
