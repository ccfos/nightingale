package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

// Binding notify rules to a legacy (notify_version=0) rule used to report applied
// while Verify silently dropped the IDs.
func TestUpdateAlertRule_NotifyRuleIdsSwitchLegacyRuleToNewMode(t *testing.T) {
	isolatedResourceProposals(t)
	deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.AlertRule{
		Id: 7, GroupId: 1, Name: "CPU high", Cate: "prometheus", Severity: 2, PromEvalInterval: 30,
		RuleConfig: `{"queries":[{"prom_ql":"cpu > 80","severity":2}]}`, NotifyGroups: "3",
	}).Error)
	params := map[string]string{"user_id": "1", "chat_id": "notify-switch", "seq_id": "1", "lang": "en_US"}

	_, err := updateAlertRule(context.Background(), deps, map[string]interface{}{"id": "7", "notify_rule_ids": "[3,5]"}, params)
	var interrupt *aiagent.ToolInterrupt
	require.ErrorAs(t, err, &interrupt)
	var replay map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(interrupt.ResumeArgs), &replay))
	params["seq_id"] = "2"
	_, err = updateAlertRule(context.Background(), deps, replay, params)

	require.NoError(t, err)
	saved, err := models.AlertRuleGetById(deps.DBCtx, 7)
	require.NoError(t, err)
	require.Equal(t, []int64{3, 5}, saved.NotifyRuleIds)
	require.Equal(t, 1, saved.NotifyVersion)
}
