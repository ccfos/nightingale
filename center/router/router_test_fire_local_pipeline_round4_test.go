package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRunTestFirePipelines_DropStopsLaterPipelinesAndReportsWarning(t *testing.T) {
	rt := identityRouter(t, &models.EventPipelineExecution{})
	event := &models.AlertCurEvent{Id: 7, Hash: "event", Severity: 2, TagsMap: map[string]string{"env": "prod"}}
	pipeline := &models.EventPipeline{
		ID: 1, Name: "drop", Nodes: []models.WorkflowNode{
			{ID: "drop-node", Name: "Drop", Type: "event_drop", Config: map[string]interface{}{"content": "true"}},
		},
	}
	config := &models.AlertRule{PipelineConfigs: []models.PipelineConfig{
		{PipelineId: 1, Enable: true}, {PipelineId: 2, Enable: true},
	}}
	pipelines := map[int64]*models.EventPipeline{1: pipeline, 2: {ID: 2, Name: "must not run"}}

	stage, _, dropped := rt.runTestFirePipelines(config, event, "alice", false, pipelines)

	require.True(t, dropped)
	require.Equal(t, testFireStageWarn, stage.Status)
	data := stage.Data
	require.Equal(t, false, data["skip_send"])
	items := data["pipelines"].([]gin.H)
	require.Len(t, items, 1)
	require.Equal(t, int64(1), items[0]["id"])
	require.Equal(t, true, items[0]["dropped"])
	var executions []models.EventPipelineExecution
	require.NoError(t, rt.Ctx.DB.Find(&executions).Error)
	require.Len(t, executions, 1)
	require.Equal(t, int64(1), executions[0].PipelineID)
	require.Equal(t, "alice", executions[0].TriggerBy)
	require.Equal(t, 2, event.Severity)
}

func TestRunTestFirePipelines_NonmatchingDropReturnsOriginalEvent(t *testing.T) {
	rt := identityRouter(t, &models.EventPipelineExecution{})
	event := &models.AlertCurEvent{Id: 7, Hash: "event", Severity: 2}
	pipeline := &models.EventPipeline{
		ID: 1, Name: "keep", Nodes: []models.WorkflowNode{
			{ID: "keep-node", Name: "Keep", Type: "event_drop", Config: map[string]interface{}{"content": "false"}},
		},
	}
	config := &models.AlertRule{PipelineConfigs: []models.PipelineConfig{{PipelineId: 1, Enable: true}}}

	stage, result, dropped := rt.runTestFirePipelines(config, event, "alice", false,
		map[int64]*models.EventPipeline{1: pipeline})

	require.False(t, dropped)
	require.Same(t, event, result)
	require.Equal(t, testFireStagePass, stage.Status)
	items := stage.Data["pipelines"].([]gin.H)
	require.Len(t, items, 1)
	require.Equal(t, models.ExecutionStatusSuccess, items[0]["status"])
	nodes := items[0]["node_results"].([]gin.H)
	require.Len(t, nodes, 1)
	require.Equal(t, "keep-node", nodes[0]["node_id"])
	require.Equal(t, "Keep", nodes[0]["node_name"])
	require.Equal(t, "success", nodes[0]["status"])
}

func TestRunTestFirePipelines_ProcessorFailureMarksStageFailed(t *testing.T) {
	rt := identityRouter(t, &models.EventPipelineExecution{})
	event := &models.AlertCurEvent{Hash: "event"}
	pipeline := &models.EventPipeline{
		ID: 1, Name: "broken", Nodes: []models.WorkflowNode{{ID: "unknown", Type: "round4-unknown-processor"}},
	}
	config := &models.AlertRule{PipelineConfigs: []models.PipelineConfig{{PipelineId: 1, Enable: true}}}

	stage, result, dropped := rt.runTestFirePipelines(config, event, "alice", false,
		map[int64]*models.EventPipeline{1: pipeline})

	require.Equal(t, testFireStageFail, stage.Status)
	require.False(t, dropped)
	require.Same(t, event, result)
	items := stage.Data["pipelines"].([]gin.H)
	nodes := items[0]["node_results"].([]gin.H)
	require.Equal(t, "failed", items[0]["status"])
	require.Equal(t, "failed", nodes[0]["status"])
	require.Contains(t, nodes[0]["error"], "round4-unknown-processor")
}

func TestRunNotifyOwnPipelines_SkipsUnavailablePipelinesAndStopsAfterDrop(t *testing.T) {
	rt := identityRouter(t, &models.EventPipeline{}, &models.EventPipelineExecution{})
	rows := []models.EventPipeline{
		{ID: 1, Name: "disabled", Disabled: true},
		{
			ID: 2, Name: "other environment", FilterEnable: true,
			LabelFilters: []models.TagFilter{{Key: "env", Func: "==", Value: "stage"}},
		},
		{ID: 3, Name: "keep"},
		{
			ID: 4, Name: "drop", Nodes: []models.WorkflowNode{
				{ID: "drop-node", Type: "event_drop", Config: map[string]interface{}{"content": "true"}},
			},
		},
	}
	require.NoError(t, rt.Ctx.DB.Create(&rows).Error)
	configs := []models.PipelineConfig{
		{PipelineId: 404, Enable: true}, {PipelineId: 1, Enable: true}, {PipelineId: 2, Enable: true},
		{PipelineId: 3, Enable: false}, {PipelineId: 3, Enable: true}, {PipelineId: 4, Enable: true},
		{PipelineId: 3, Enable: true},
	}
	event := &models.AlertCurEvent{Id: 7, Hash: "event", Severity: 2, TagsMap: map[string]string{"env": "prod"}}
	original := event.DeepCopy()

	result, dropped := rt.runNotifyOwnPipelines(configs, event, "alice")

	require.True(t, dropped)
	require.Nil(t, result)
	require.Equal(t, original, event)
	var executions []models.EventPipelineExecution
	require.NoError(t, rt.Ctx.DB.Find(&executions).Error)
	require.Len(t, executions, 1)
	require.Equal(t, int64(4), executions[0].PipelineID)
	require.Equal(t, "alice", executions[0].TriggerBy)
}

func TestRunNotifyOwnPipelines_NoEnabledPipelinesPreservesEvent(t *testing.T) {
	rt := &Router{}
	event := &models.AlertCurEvent{Id: 7}

	result, dropped := rt.runNotifyOwnPipelines([]models.PipelineConfig{{PipelineId: 1, Enable: false}}, event, "alice")

	require.False(t, dropped)
	require.Same(t, event, result)
}

func TestSanitizeNodeResults_RedactsCredentialsAndOmitsInternalMessages(t *testing.T) {
	nodes := []*models.NodeExecutionResult{
		nil,
		{NodeID: "safe", NodeName: "Safe", NodeType: "local", Status: "success", DurationMs: 12, Message: "internal data"},
		{NodeID: "failed", NodeType: "local", Status: "failed", Error: "authorization: secret-value"},
	}

	got := sanitizeNodeResults(nodes)

	require.Equal(t, []gin.H{
		{"node_id": "safe", "node_name": "Safe", "node_type": "local", "status": "success", "duration_ms": int64(12)},
		{
			"node_id": "failed", "node_name": "", "node_type": "local", "status": "failed", "duration_ms": int64(0),
			"error": "(details hidden for security)",
		},
	}, got)
}
