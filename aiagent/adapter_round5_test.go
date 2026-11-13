package aiagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestWorkflowContextToRequest_FlattensEventWithoutChangingInputs(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	wf := &models.WorkflowContext{
		Event: &models.AlertCurEvent{
			RuleName: "CPU high", Severity: 2, TriggerValue: "91", GroupName: "Operations",
			TagsMap:         map[string]string{"host": "web-1", "env": "prod"},
			AnnotationsJSON: map[string]string{"runbook": "inspect CPU"},
		},
		Inputs: map[string]string{"alert_name": "original", "scope": "prod"},
		Vars:   map[string]interface{}{"query": "up"}, Metadata: map[string]string{"trace": "one"}, ParentCtx: parent,
	}

	req := workflowContextToRequest(wf)

	require.Equal(t, "CPU high", req.Params["alert_name"])
	require.Equal(t, "2", req.Params["severity"])
	require.Equal(t, "91", req.Params["trigger_value"])
	require.Equal(t, "Operations", req.Params["group_name"])
	require.Equal(t, "inspect CPU", req.Params["annotation_runbook"])
	require.Equal(t, "prod", req.Params["scope"])
	// Tag order is intentionally unspecified because the source is a map.
	require.Contains(t, req.Params["tags"], "host=web-1")
	require.Contains(t, req.Params["tags"], "env=prod")
	require.Equal(t, wf.Vars, req.Vars)
	require.Equal(t, wf.Metadata, req.Metadata)
	require.Same(t, parent, req.ParentCtx)
	require.Same(t, wf.Event, req.TemplateExtra["Event"])
	require.Same(t, wf.Event, req.TemplateExtra["event"])
	require.Equal(t, wf.Inputs, req.TemplateExtra["Inputs"])
	require.Equal(t, wf.Inputs, req.TemplateExtra["inputs"])
	req.Params["scope"] = "changed"
	require.Equal(t, map[string]string{"alert_name": "original", "scope": "prod"}, wf.Inputs)
}

func TestWorkflowContextToRequest_HandlesAbsentEvent(t *testing.T) {
	wf := &models.WorkflowContext{Inputs: map[string]string{"question": "inspect"}}

	req := workflowContextToRequest(wf)

	require.Equal(t, map[string]string{"question": "inspect"}, req.Params)
	require.Nil(t, req.ParentCtx)
	require.Nil(t, req.TemplateExtra["event"])
}

func TestWriteResponseToWorkflowContext_WritesOutputWithoutEvent(t *testing.T) {
	wf := &models.WorkflowContext{}
	steps := []ToolStep{{Action: "lookup", Observation: "healthy"}}

	writeResponseToWorkflowContext(wf, &AgentResponse{Content: "CPU healthy", Steps: steps}, "analysis")

	require.Equal(t, map[string]interface{}{"analysis": "CPU healthy", "analysis_steps": steps}, wf.Output)
}

func TestWriteResponseToWorkflowContext_PreservesExistingAnnotations(t *testing.T) {
	wf := &models.WorkflowContext{Event: &models.AlertCurEvent{AnnotationsJSON: map[string]string{"runbook": "inspect"}}}
	steps := []ToolStep{{Action: "lookup", Observation: "healthy"}}

	writeResponseToWorkflowContext(wf, &AgentResponse{Content: "CPU healthy", Steps: steps}, "analysis")

	require.Equal(t, "inspect", wf.Event.AnnotationsJSON["runbook"])
	require.Equal(t, "CPU healthy", wf.Event.AnnotationsJSON["analysis"])
	var savedSteps []ToolStep
	require.NoError(t, json.Unmarshal([]byte(wf.Event.AnnotationsJSON["analysis_steps"]), &savedSteps))
	require.Equal(t, steps, savedSteps)
	var persisted map[string]string
	require.NoError(t, json.Unmarshal([]byte(wf.Event.Annotations), &persisted))
	require.Equal(t, wf.Event.AnnotationsJSON, persisted)
}

func TestWriteResponseToWorkflowContext_InitializesAnnotationsWithoutSteps(t *testing.T) {
	wf := &models.WorkflowContext{Event: &models.AlertCurEvent{}}

	writeResponseToWorkflowContext(wf, &AgentResponse{Content: "healthy"}, "analysis")

	require.Equal(t, map[string]string{"analysis": "healthy"}, wf.Event.AnnotationsJSON)
	require.JSONEq(t, `{"analysis":"healthy"}`, wf.Event.Annotations)
}

func TestWriteResponseToWorkflowContext_IgnoresNilResponse(t *testing.T) {
	wf := &models.WorkflowContext{Output: map[string]interface{}{"existing": "value"}}

	writeResponseToWorkflowContext(wf, nil, "analysis")

	require.Equal(t, map[string]interface{}{"existing": "value"}, wf.Output)
}

func TestProcessorAdapter_ProcessWritesRenderedAnswer(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{{{Content: "CPU is healthy"}}}}
	adapter := &ProcessorAdapter{agent: NewAgent(&AgentConfig{
		OutputField: "diagnosis", UserPromptTemplate: "{{.Event.RuleName}}: {{.Inputs.question}}",
	}, WithLLMClient(fake))}
	wf := &models.WorkflowContext{
		Inputs: map[string]string{"question": "why?"}, Event: &models.AlertCurEvent{RuleName: "CPU high"},
	}

	result, message, err := adapter.Process(nil, wf)

	require.NoError(t, err)
	require.Same(t, wf, result)
	require.Equal(t, "AI Agent completed: 1 iterations, success=true", message)
	require.Equal(t, "CPU is healthy", result.Event.AnnotationsJSON["diagnosis"])
	require.Len(t, fake.reqs, 1)
	messages := fake.reqs[0].Messages
	require.Equal(t, "CPU high: why?", messages[len(messages)-1].Content)
}

func TestProcessorAdapter_ProcessReportsAgentFailure(t *testing.T) {
	adapter := &ProcessorAdapter{agent: NewAgent(&AgentConfig{OutputField: "diagnosis"})}
	wf := &models.WorkflowContext{}

	result, message, err := adapter.Process(nil, wf)

	require.Same(t, wf, result)
	require.ErrorContains(t, err, "agent error:")
	require.Contains(t, err.Error(), "LLM")
	require.Contains(t, message, "success=false")
}

func TestProcessorAdapter_InitRejectsInvalidSettings(t *testing.T) {
	cases := []struct {
		name     string
		settings interface{}
	}{
		{name: "not serializable", settings: make(chan int)},
		{name: "wrong shape", settings: "not an object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			processor, err := (&ProcessorAdapter{}).Init(tc.settings)

			require.Nil(t, processor)
			require.Error(t, err)
		})
	}
}
