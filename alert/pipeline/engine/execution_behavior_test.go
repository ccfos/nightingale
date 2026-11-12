package engine

import (
	"context"
	"testing"

	_ "github.com/ccfos/nightingale/v6/alert/pipeline/processor/eventdrop"
	_ "github.com/ccfos/nightingale/v6/alert/pipeline/processor/logic"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestExecute_EmptyPipelinePreservesEvent(t *testing.T) {
	engine := NewWorkflowEngine(nil)
	event := &models.AlertCurEvent{Hash: "event-1"}

	got, result, err := engine.Execute(&models.EventPipeline{}, event, nil)

	require.NoError(t, err)
	require.Same(t, event, got)
	require.Same(t, event, result.Event)
	require.Equal(t, models.ExecutionStatusSuccess, result.Status)
	require.Equal(t, "no nodes to execute", result.Message)
}

func TestWorkflowContext_MergesOverridesAndTriggerMetadata(t *testing.T) {
	engine := NewWorkflowEngine(nil)
	pipeline := &models.EventPipeline{ID: 12, Inputs: []models.InputVariable{
		{Key: "env", Value: "dev"}, {Key: "team", Value: "ops"},
	}}
	trigger := &models.WorkflowTriggerContext{
		RequestID: "request-1", Mode: "api", TriggerBy: "alice", Stream: true,
		InputsOverrides: map[string]string{"env": "prod", "region": "west"},
	}
	event := &models.AlertCurEvent{}

	workflow := engine.initWorkflowContext(pipeline, event, trigger)

	require.Same(t, event, workflow.Event)
	require.Equal(t, map[string]string{"env": "prod", "team": "ops", "region": "west"}, workflow.Inputs)
	require.Equal(t, "12", workflow.Metadata["pipeline_id"])
	require.Equal(t, "request-1", workflow.Metadata["request_id"])
	require.Equal(t, "api", workflow.Metadata["trigger_mode"])
	require.Equal(t, "alice", workflow.Metadata["trigger_by"])
	require.True(t, workflow.Stream)
	require.NotNil(t, workflow.Vars)
	require.Equal(t, "dev", pipeline.Inputs[0].Value)
}

func TestExecute_ProcessesLinearNodesInDependencyOrder(t *testing.T) {
	engine := NewWorkflowEngine(nil)
	event := &models.AlertCurEvent{Hash: "event-1"}
	pipeline := &models.EventPipeline{Nodes: []models.WorkflowNode{
		{ID: "keep", Name: "Keep", Type: "event_drop", Config: map[string]interface{}{"content": "false"}},
		{ID: "drop", Name: "Drop", Type: "event_drop", Config: map[string]interface{}{"content": "true"}},
		{ID: "unreached", Name: "Unreached", Type: "missing"},
	}}

	got, result, err := engine.Execute(pipeline, event, nil)

	require.NoError(t, err)
	require.Nil(t, got)
	require.Nil(t, result.Event)
	require.Equal(t, models.ExecutionStatusSuccess, result.Status)
	require.Len(t, result.NodeResults, 2)
	require.Equal(t, "keep", result.NodeResults[0].NodeID)
	require.Equal(t, "success", result.NodeResults[0].Status)
	require.Equal(t, "drop", result.NodeResults[1].NodeID)
	require.Equal(t, "terminated", result.NodeResults[1].Status)
	require.Contains(t, result.NodeResults[1].Message, "drop event success")
	require.Equal(t, "workflow terminated at node Drop", result.Message)
}

func TestExecute_DisabledNodeSkipsProcessorDispatch(t *testing.T) {
	engine := NewWorkflowEngine(nil)
	pipeline := &models.EventPipeline{Nodes: []models.WorkflowNode{
		{ID: "disabled", Type: "unregistered", Disabled: true},
		{ID: "keep", Type: "event_drop", Config: map[string]interface{}{"content": "false"}},
	}}
	event := &models.AlertCurEvent{Hash: "event-1"}

	got, result, err := engine.Execute(pipeline, event, nil)

	require.NoError(t, err)
	require.Same(t, event, got)
	require.Equal(t, models.ExecutionStatusSuccess, result.Status)
	require.Len(t, result.NodeResults, 2)
	require.Equal(t, "skipped", result.NodeResults[0].Status)
	require.Equal(t, "node is disabled", result.NodeResults[0].Message)
	require.Equal(t, "success", result.NodeResults[1].Status)
}

func TestExecute_UnknownProcessorReportsFailure(t *testing.T) {
	engine := NewWorkflowEngine(nil)
	pipeline := &models.EventPipeline{Nodes: []models.WorkflowNode{{ID: "unknown", Name: "Unknown", Type: "unregistered"}}}

	_, result, err := engine.Execute(pipeline, &models.AlertCurEvent{}, nil)

	require.NoError(t, err)
	require.Equal(t, models.ExecutionStatusFailed, result.Status)
	require.Equal(t, "unknown", result.ErrorNode)
	require.Contains(t, result.Message, "processor type unregistered not found")
}

func TestExecute_ContinueOnFailKeepsWorkflowSuccessful(t *testing.T) {
	engine := NewWorkflowEngine(nil)
	pipeline := &models.EventPipeline{Nodes: []models.WorkflowNode{
		{ID: "invalid", Type: "event_drop", Config: map[string]interface{}{"content": "{{"}, ContinueOnFail: true},
		{ID: "keep", Type: "event_drop", Config: map[string]interface{}{"content": "false"}},
	}}

	_, result, err := engine.Execute(pipeline, &models.AlertCurEvent{Hash: "event-1"}, nil)

	require.NoError(t, err)
	require.Equal(t, models.ExecutionStatusSuccess, result.Status)
	require.Len(t, result.NodeResults, 2)
	require.Equal(t, "failed", result.NodeResults[0].Status)
	require.Contains(t, result.NodeResults[0].Error, "failed to parse template")
	require.Equal(t, "success", result.NodeResults[1].Status)
}

func TestExecute_ConditionalNodeDispatchesOnlySelectedBranch(t *testing.T) {
	engine := NewWorkflowEngine(nil)
	pipeline := &models.EventPipeline{
		Nodes: []models.WorkflowNode{
			{ID: "condition", Type: "logic.if", Config: map[string]interface{}{"condition": "false"}},
			{ID: "true", Type: "unregistered"},
			{ID: "false", Type: "event_drop", Config: map[string]interface{}{"content": "false"}},
		},
		Connections: models.Connections{"condition": {Main: [][]models.ConnectionTarget{
			{{Node: "true", Type: "main"}}, {{Node: "false", Type: "main"}},
		}}},
	}

	_, result, err := engine.Execute(pipeline, &models.AlertCurEvent{Hash: "event-1"}, nil)

	require.NoError(t, err)
	require.Equal(t, models.ExecutionStatusSuccess, result.Status)
	require.Len(t, result.NodeResults, 2)
	require.Equal(t, "condition", result.NodeResults[0].NodeID)
	require.NotNil(t, result.NodeResults[0].BranchIndex)
	require.Equal(t, 1, *result.NodeResults[0].BranchIndex)
	require.Equal(t, "false", result.NodeResults[1].NodeID)
}

func TestExecute_RejectsCircularWorkflow(t *testing.T) {
	engine := NewWorkflowEngine(nil)
	pipeline := &models.EventPipeline{
		Nodes: []models.WorkflowNode{{ID: "a"}, {ID: "b"}},
		Connections: models.Connections{
			"a": {Main: [][]models.ConnectionTarget{{{Node: "b"}}}},
			"b": {Main: [][]models.ConnectionTarget{{{Node: "a"}}}},
		},
	}

	_, result, err := engine.Execute(pipeline, &models.AlertCurEvent{}, nil)

	require.NoError(t, err)
	require.Equal(t, models.ExecutionStatusFailed, result.Status)
	require.Equal(t, "workflow has circular dependency", result.Message)
	require.Empty(t, result.NodeResults)
}

func TestExecute_PersistsTriggeredExecutionAndInputSnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.EventPipelineExecution{}))
	engine := NewWorkflowEngine(ctx.NewContext(context.Background(), db, true))
	pipeline := &models.EventPipeline{ID: 12, Name: "Example", Nodes: []models.WorkflowNode{{ID: "skip", Disabled: true}}}
	trigger := &models.WorkflowTriggerContext{
		RequestID: "request-1", Mode: "api", TriggerBy: "alice", InputsOverrides: map[string]string{"env": "prod"},
	}

	_, result, err := engine.Execute(pipeline, &models.AlertCurEvent{Id: 7}, trigger)

	require.NoError(t, err)
	var stored models.EventPipelineExecution
	require.NoError(t, db.First(&stored, "id = ?", "request-1").Error)
	require.Equal(t, pipeline.ID, stored.PipelineID)
	require.Equal(t, pipeline.Name, stored.PipelineName)
	require.Equal(t, int64(7), stored.EventID)
	require.Equal(t, "alice", stored.TriggerBy)
	require.Equal(t, "api", stored.Mode)
	require.Equal(t, result.Status, stored.Status)
	nodes, err := stored.GetNodeResults()
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.Equal(t, "skipped", nodes[0].Status)
	require.Contains(t, stored.InputsSnapshot, "prod")
}
