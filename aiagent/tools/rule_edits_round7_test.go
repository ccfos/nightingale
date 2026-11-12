package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func confirmRuleEdit(t *testing.T, args map[string]interface{}) *models.AlertRule {
	t.Helper()
	isolatedResourceProposals(t)
	deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.AlertRule{
		Id: 7, GroupId: 1, Name: "CPU high", Cate: "prometheus", Severity: 2, NotifyVersion: 1,
		RuleConfig: `{"queries":[{"prom_ql":"cpu > 80","severity":2}],"old_field":true}`,
	}).Error)
	args["id"] = "7"
	params := map[string]string{"user_id": "1", "chat_id": "rule-round7", "seq_id": "1", "lang": "en_US"}
	out, err := updateAlertRule(context.Background(), deps, args, params)
	require.Empty(t, out)
	var interrupt *aiagent.ToolInterrupt
	require.ErrorAs(t, err, &interrupt)
	var replay map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(interrupt.ResumeArgs), &replay))
	before, err := models.AlertRuleGetById(deps.DBCtx, 7)
	require.NoError(t, err)
	require.Equal(t, "CPU high", before.Name)
	require.Equal(t, "cpu > 80", currentBakedPromQL(before.RuleConfigJson))
	params["seq_id"] = "2"

	out, err = updateAlertRule(context.Background(), deps, replay, params)

	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, true, result["applied"])
	saved, err := models.AlertRuleGetById(deps.DBCtx, 7)
	require.NoError(t, err)
	require.NotNil(t, saved)
	return saved
}

func TestUpdateAlertRule_ConfirmsMetadataEdits(t *testing.T) {
	args := map[string]interface{}{
		"name": "CPU saturation", "note": "Inspect the busy host", "runbook_url": "/runbooks/cpu",
		"append_tags": " env=prod   team=ops ", "notify_rule_ids": "[3,5]",
	}

	saved := confirmRuleEdit(t, args)

	require.Equal(t, "CPU saturation", saved.Name)
	require.Equal(t, "Inspect the busy host", saved.Note)
	require.Equal(t, "/runbooks/cpu", saved.RunbookUrl)
	require.Equal(t, []string{"env=prod", "team=ops"}, saved.AppendTagsJSON)
	require.Equal(t, []int64{3, 5}, saved.NotifyRuleIds)
	require.Equal(t, "cpu > 80", currentBakedPromQL(saved.RuleConfigJson))
}

func TestUpdateAlertRule_ReplacesRawConfigAndConvertsIntervalToSeconds(t *testing.T) {
	args := map[string]interface{}{
		"rule_config_json": `{"queries":[{"prom_ql":"memory > 90","severity":2,"interval":2,"interval_unit":"min"}]}`,
	}

	saved := confirmRuleEdit(t, args)

	var config map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(saved.RuleConfig), &config))
	require.NotContains(t, config, "old_field")
	queries, ok := promQueries(config)
	require.True(t, ok)
	require.Equal(t, "memory > 90", queries[0]["prom_ql"])
	require.Equal(t, float64(120), queries[0]["interval"])
	require.NotContains(t, queries[0], "interval_unit")
}

func TestUpdateAlertRule_ConfirmsExpressionAndOperatorEdit(t *testing.T) {
	args := map[string]interface{}{"prom_ql": "sum(cpu)", "operator": "<="}

	saved := confirmRuleEdit(t, args)

	// Parentheses preserve the grouping of the replacement expression before the comparison.
	require.Equal(t, "(sum(cpu)) <= 80", currentBakedPromQL(saved.RuleConfigJson))
}
