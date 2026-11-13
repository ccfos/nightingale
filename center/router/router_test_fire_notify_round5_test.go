package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRunTestFireNotify_DryRunReportsMatchedChannelsWithoutSending(t *testing.T) {
	rt := identityRouter(t, &models.NotifyChannelConfig{}, &models.EventPipelineExecution{})
	require.NoError(t, rt.Ctx.DB.Create(&models.NotifyChannelConfig{ID: 3, Name: "Primary"}).Error)
	config := &models.AlertRule{NotifyVersion: 1, NotifyRuleIds: []int64{1}}
	event := &models.AlertCurEvent{Severity: 2, NotifyRuleIds: []int64{1, 1}, Hash: "dry-run"}
	notifyRules := map[int64]*models.NotifyRule{1: {
		ID: 1, Name: "oncall", Enable: true,
		PipelineConfigs: []models.PipelineConfig{{PipelineId: 99, Enable: true}},
		NotifyConfigs: []models.NotifyConfig{
			{ChannelID: 3, Severities: []int{2}},
			{ChannelID: 3, Severities: []int{2}},
		},
	}}

	stage := rt.runTestFireNotify(config, event, true, "alice", notifyRules)

	require.Equal(t, "notify", stage.Stage)
	require.Equal(t, testFireStagePass, stage.Status)
	require.Equal(t, true, stage.Data["skip_send"])
	items := stage.Data["results"].([]gin.H)
	// Duplicate rule IDs are evaluated once, with one result per channel configuration.
	require.Len(t, items, 2)
	for _, item := range items {
		require.Equal(t, int64(1), item["notify_rule_id"])
		require.Equal(t, "oncall", item["notify_rule_name"])
		require.Equal(t, "Primary", item["channel_name"])
		require.Equal(t, true, item["matched"])
		require.Equal(t, false, item["sent"])
		require.Equal(t, true, item["skipped"])
	}
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.EventPipelineExecution{}).Count(&count).Error)
	require.Zero(t, count)
	require.Equal(t, []int64{1, 1}, event.NotifyRuleIds)
}

func TestRunTestFireNotify_DryRunReportsSeverityMismatch(t *testing.T) {
	rt := identityRouter(t, &models.NotifyChannelConfig{})
	config := &models.AlertRule{NotifyVersion: 1, NotifyRuleIds: []int64{1}}
	event := &models.AlertCurEvent{Severity: 2, NotifyRuleIds: []int64{1}}
	notifyRules := map[int64]*models.NotifyRule{1: {
		ID: 1, Name: "critical only", Enable: true,
		NotifyConfigs: []models.NotifyConfig{{ChannelID: 404, Severities: []int{1}}},
	}}

	stage := rt.runTestFireNotify(config, event, true, "alice", notifyRules)

	require.Equal(t, testFireStagePass, stage.Status)
	items := stage.Data["results"].([]gin.H)
	require.Len(t, items, 1)
	require.Equal(t, "channel-404", items[0]["channel_name"])
	require.Equal(t, false, items[0]["matched"])
	require.Equal(t, "event severity not match severity filter", items[0]["match_error"])
	require.NotContains(t, items[0], "sent")
}

func TestRunTestFireNotify_ReportsMissingAndDisabledRules(t *testing.T) {
	config := &models.AlertRule{NotifyVersion: 1, NotifyRuleIds: []int64{1, 2}}
	event := &models.AlertCurEvent{NotifyRuleIds: []int64{1, 2}}
	notifyRules := map[int64]*models.NotifyRule{2: {ID: 2, Name: "disabled", Enable: false}}

	stage := (&Router{}).runTestFireNotify(config, event, true, "alice", notifyRules)

	require.Equal(t, testFireStageFail, stage.Status)
	items := stage.Data["results"].([]gin.H)
	require.Equal(t, []gin.H{
		{"notify_rule_id": int64(1), "matched": false, "match_error": "notify rule not found"},
		{"notify_rule_id": int64(2), "notify_rule_name": "disabled", "matched": false, "match_error": "notify rule disabled"},
	}, items)
}

func TestRunTestFireNotify_NotifyPipelineDropPreventsChannelDispatch(t *testing.T) {
	rt := identityRouter(t, &models.EventPipeline{}, &models.EventPipelineExecution{})
	require.NoError(t, rt.Ctx.DB.Create(&models.EventPipeline{
		ID: 9, Name: "drop",
		Nodes: []models.WorkflowNode{{ID: "drop", Type: "event_drop", Config: map[string]interface{}{"content": "true"}}},
	}).Error)
	config := &models.AlertRule{NotifyVersion: 1, NotifyRuleIds: []int64{1}}
	event := &models.AlertCurEvent{Severity: 2, NotifyRuleIds: []int64{1}}
	notifyRules := map[int64]*models.NotifyRule{1: {
		ID: 1, Name: "suppressed", Enable: true,
		PipelineConfigs: []models.PipelineConfig{{PipelineId: 9, Enable: true}},
		NotifyConfigs:   []models.NotifyConfig{{ChannelID: 404, Severities: []int{2}}},
	}}

	stage := rt.runTestFireNotify(config, event, false, "alice", notifyRules)

	require.Equal(t, testFireStagePass, stage.Status)
	items := stage.Data["results"].([]gin.H)
	require.Equal(t, []gin.H{{
		"notify_rule_id": int64(1), "notify_rule_name": "suppressed", "matched": false, "dropped_by_notify_pipeline": true,
	}}, items)
	var executions []models.EventPipelineExecution
	require.NoError(t, rt.Ctx.DB.Find(&executions).Error)
	require.Len(t, executions, 1)
	require.Equal(t, int64(9), executions[0].PipelineID)
	require.Equal(t, "alice", executions[0].TriggerBy)
	require.Equal(t, 2, event.Severity)
}
