package tools

import (
	"context"
	"encoding/json"
	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestR8eMuteDurationConfirmationAndDetail(t *testing.T) {
	isolatedResourceProposals(t)
	deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.AlertMute{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.BusiGroup{Id: 1, Name: "Ops"}).Error)
	start := time.Now().Add(24 * time.Hour).Unix()
	mute := &models.AlertMute{Id: 7, GroupId: 1, Btime: start, Etime: start + 60, Cause: "release", CreateBy: "original"}
	require.NoError(t, mute.Add(deps.DBCtx))
	params := map[string]string{"user_id": "1", "chat_id": "r8e-mute", "seq_id": "1", "lang": "en_US"}
	_, err := updateAlertMute(context.Background(), deps, map[string]interface{}{"id": "7", "duration": "2h"}, params)
	var interrupt *aiagent.ToolInterrupt
	require.ErrorAs(t, err, &interrupt)
	require.Contains(t, interrupt.Prompt, "release")
	replay := r8eReplay(t, interrupt)
	require.Equal(t, "2h", replay["duration"])
	params["seq_id"] = "2"
	out, err := updateAlertMute(context.Background(), deps, replay, params)
	require.NoError(t, err)
	require.Contains(t, out, `"applied":true`)
	saved, err := models.AlertMuteGetById(deps.DBCtx, 7)
	require.NoError(t, err)
	require.Equal(t, start+7200, saved.Etime)
	out, err = getAlertMuteDetail(context.Background(), deps, map[string]interface{}{"id": "7"}, params)
	require.NoError(t, err)
	var detail alertMuteDetailResult
	require.NoError(t, json.Unmarshal([]byte(out), &detail))
	require.Equal(t, "release", detail.Cause)
	require.Equal(t, "original", detail.CreateBy)
	require.Equal(t, formatUnixTime(start+7200), detail.Etime)
	for _, config := range []string{"{", `{"disabled":"bad"}`, `{"tags":[{"op":"=","value":"x"}]}`, `{"etime":1}`} {
		_, err := updateAlertMute(context.Background(), deps, map[string]interface{}{"id": "7", "config": config}, params)
		require.Error(t, err)
	}
}

func TestR8eDashboardMalformedUpdatesPreservePayload(t *testing.T) {
	for _, tc := range []struct {
		payload string
		args    map[string]interface{}
		want    string
	}{
		{"", nil, "has no config payload"}, {"{", nil, "invalid dashboard config"},
		{`{"panels":[],"var":[]}`, map[string]interface{}{"variables": "{"}, "invalid variables JSON"},
		{`{"panels":[],"var":[]}`, map[string]interface{}{"panels": "{"}, "invalid panels JSON"},
		{`{"panels":[],"var":[]}`, nil, "nothing to change"},
	} {
		deps := newDashboardTestDeps(t)
		require.NoError(t, models.DB(deps.DBCtx).Create(&models.Board{Id: 7, GroupId: 1}).Error)
		require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 7, tc.payload))
		args := map[string]interface{}{"id": "7"}
		for k, v := range tc.args {
			args[k] = v
		}
		out, err := updateDashboard(context.Background(), deps, args, map[string]string{"user_id": "1"})
		require.Empty(t, out)
		require.ErrorContains(t, err, tc.want)
		payload, err := models.BoardPayloadGet(deps.DBCtx, 7)
		require.NoError(t, err)
		require.Equal(t, tc.payload, payload)
	}
}

func TestR8eDashboardPatchDecodingAndEdits(t *testing.T) {
	for _, raw := range []string{`{"multi":[]}`, `{"delete":[]}`, `{"name":[]}`} {
		var p variablePatch
		require.Error(t, json.Unmarshal([]byte(raw), &p))
	}
	for _, raw := range []string{`{"delete":[]}`, `{"id":[]}`} {
		var p panelPatch
		require.Error(t, json.Unmarshal([]byte(raw), &p))
	}
	var patches []variablePatch
	require.NoError(t, json.Unmarshal([]byte(`[{"name":"host","type":"query","definition":"label_values(up,host)","defaultValue":"a","multi":true}]`), &patches))
	configs := map[string]interface{}{"var": []interface{}{nil}}
	changes, err := applyVariablePatches("en_US", configs, patches)
	require.NoError(t, err)
	require.NotEmpty(t, changes)
	require.NoError(t, json.Unmarshal([]byte(`[{"name":"host","type":"textbox","definition":"updated"}]`), &patches))
	_, err = applyVariablePatches("en_US", configs, patches)
	require.NoError(t, err)
	vars := configs["var"].([]interface{})
	require.Equal(t, "updated", vars[1].(map[string]interface{})["definition"])
	require.Equal(t, "textbox", vars[1].(map[string]interface{})["type"])
	require.NoError(t, json.Unmarshal([]byte(`[{"name":"host","type":"invalid"}]`), &patches))
	_, err = applyVariablePatches("en_US", configs, patches)
	require.ErrorContains(t, err, "only query")
	var panels []panelPatch
	require.NoError(t, json.Unmarshal([]byte(`[{"id":"missing","delete":true}]`), &panels))
	_, err = applyPanelPatches("en_US", map[string]interface{}{"panels": []interface{}{nil}}, panels)
	require.ErrorContains(t, err, "not found")
}

func TestR8eListEmptyMembershipAndLimits(t *testing.T) {
	deps := newResourceTestDeps(t, &models.RoleOperation{}, &models.UserGroupMember{}, &models.BusiGroupMember{}, &models.AlertRule{}, &models.AlertMute{}, &models.Board{})
	for _, op := range []string{PermAlertRules, PermAlertMutes, PermDashboards} {
		require.NoError(t, models.DB(deps.DBCtx).Create(&models.RoleOperation{RoleName: "Standard", Operation: op}).Error)
	}
	for _, fn := range []aiagent.BuiltinToolFunc{listAlertRules, listLegacyNotifyAlertRules, listAlertMutes, listDashboards} {
		out, err := fn(context.Background(), deps, map[string]interface{}{"limit": float64(9999)}, map[string]string{"user_id": "2"})
		require.NoError(t, err)
		require.JSONEq(t, `{"total":0,"items":[]}`, out)
	}
}

func TestR8eAlertRuleShortcutErrorsPreserveRule(t *testing.T) {
	for _, tc := range []struct {
		cate, config, prom string
		args               map[string]interface{}
		want               string
	}{
		{"mysql", `{}`, "", map[string]interface{}{"threshold": "2"}, "only apply to cate=prometheus"},
		{"prometheus", `{}`, "up > 1", map[string]interface{}{"operator": "bad"}, "invalid operator"},
		{"prometheus", `{}`, "up > 1", map[string]interface{}{"threshold": "2"}, "not a simple prometheus"},
		{"prometheus", `{}`, "up", map[string]interface{}{"prom_ql": "rate(up[5m])"}, "threshold is required"},
	} {
		deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{})
		db := models.DB(deps.DBCtx)
		require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Ops"}).Error)
		rule := &models.AlertRule{Id: 7, GroupId: 1, Name: "Original", Cate: tc.cate, PromQl: tc.prom, RuleConfig: tc.config}
		require.NoError(t, db.Create(rule).Error)
		args := map[string]interface{}{"id": "7"}
		for k, v := range tc.args {
			args[k] = v
		}
		out, err := updateAlertRule(context.Background(), deps, args, map[string]string{"user_id": "1"})
		require.Empty(t, out)
		require.ErrorContains(t, err, tc.want)
		var saved models.AlertRule
		require.NoError(t, db.First(&saved, 7).Error)
		require.Equal(t, "Original", saved.Name)
		require.Equal(t, tc.config, saved.RuleConfig)
	}
}

func TestR8eRuleImportDatasourceValidation(t *testing.T) {
	for _, fn := range []aiagent.BuiltinToolFunc{importPromRuleYAML, importAlertRuleTemplate} {
		for _, migrate := range []bool{false, true} {
			deps := newResourceTestDeps(t, &models.BusiGroup{})
			db := models.DB(deps.DBCtx)
			require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Ops"}).Error)
			if migrate {
				require.NoError(t, db.AutoMigrate(&models.Datasource{}))
			}
			out, err := fn(context.Background(), deps, map[string]interface{}{"group_id": "1", "datasource_id": "3", "datasource_ids": "[3]", "component": "Linux", "file": "test.json"}, map[string]string{"user_id": "1"})
			require.Empty(t, out)
			if migrate {
				require.ErrorContains(t, err, "record not found")
			} else {
				require.ErrorContains(t, err, "failed to get datasource")
			}
		}
	}
	deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.Datasource{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.BusiGroup{Id: 1}).Error)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Datasource{Id: 3}).Error)
	_, err := importPromRuleYAML(context.Background(), deps, map[string]interface{}{"group_id": "1", "datasource_ids": "[3]", "disabled": "bad"}, map[string]string{"user_id": "1"})
	require.ErrorContains(t, err, "disabled must be")
	_, err = importPromRuleYAML(context.Background(), deps, nil, map[string]string{"user_id": "1"})
	require.ErrorContains(t, err, "group_id is required")
}

func TestR8eAlertRuleListFiltersEnabledNames(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.AlertRule{
		{Id: 1, Name: "CPU disabled", Disabled: 1}, {Id: 2, Name: "Memory", Disabled: 0}, {Id: 3, Name: "CPU active", Disabled: 0},
	}).Error)
	out, err := listAlertRules(context.Background(), deps, map[string]interface{}{"query": "cpu", "disabled": float64(0)}, map[string]string{"user_id": "1"})
	require.NoError(t, err)
	var result struct {
		Total int               `json:"total"`
		Items []alertRuleResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Len(t, result.Items, 1)
	require.Equal(t, "CPU active", result.Items[0].Name)
}
