package aiagent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func nestedAgentSettings(t *testing.T) (map[string]interface{}, <-chan map[string]interface{}) {
	t.Helper()
	previous := defaultClientCache
	defaultClientCache = &llm.ClientCache{}
	t.Cleanup(func() { defaultClientCache = previous })
	received := make(chan map[string]interface{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		received <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"healthy"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(server.Close)
	return map[string]interface{}{
		"llm":          map[string]interface{}{"provider": llm.ProviderOpenAI, "base_url": server.URL, "model": "fixture"},
		"output_field": "original", "user_prompt_template": "{{.Inputs.question}}",
	}, received
}

func TestExecuteProcessorTool_MergesArgumentsAndReportsEventChanges(t *testing.T) {
	settings, received := nestedAgentSettings(t)
	tool := &AgentTool{ProcessorType: "ai.agent", ProcessorConfig: settings}
	wf := &models.WorkflowContext{
		Event:  &models.AlertCurEvent{},
		Inputs: map[string]string{"question": "inspect CPU"},
	}

	got, err := executeProcessorTool(nil, tool, map[string]interface{}{"output_field": "diagnosis"}, wf)

	require.NoError(t, err)
	require.Contains(t, got, "Message: AI Agent completed: 1 iterations, success=true\n")
	require.Contains(t, got, `Updated annotations: {"diagnosis":"healthy"}`)
	require.Empty(t, wf.Event.Annotations)
	require.Nil(t, wf.Event.AnnotationsJSON)
	require.Equal(t, "original", settings["output_field"])
	request := <-received
	messages := request["messages"].([]interface{})
	require.Equal(t, "inspect CPU", messages[len(messages)-1].(map[string]interface{})["content"])
}

func TestExecuteProcessorTool_RejectsUnavailableProcessor(t *testing.T) {
	cases := []struct {
		name string
		tool AgentTool
		want string
	}{
		{name: "missing type", want: "processor_type not configured"},
		{
			name: "unknown type", tool: AgentTool{ProcessorType: "round7.missing"},
			want: "failed to get processor 'round7.missing'",
		},
		{
			name: "invalid settings",
			tool: AgentTool{ProcessorType: "ai.agent", ProcessorConfig: map[string]interface{}{"timeout": "invalid"}},
			want: "failed to get processor 'ai.agent'",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := executeProcessorTool(nil, &tc.tool, nil, &models.WorkflowContext{})

			require.Empty(t, got)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestExecuteProcessorTool_PropagatesExecutionFailure(t *testing.T) {
	tool := &AgentTool{ProcessorType: "ai.agent", ProcessorConfig: map[string]interface{}{
		"llm": map[string]interface{}{"provider": "unsupported"},
	}}
	previous := defaultClientCache
	defaultClientCache = &llm.ClientCache{}
	t.Cleanup(func() { defaultClientCache = previous })
	wf := &models.WorkflowContext{Event: &models.AlertCurEvent{Annotations: "original"}}

	got, err := executeProcessorTool(nil, tool, nil, wf)

	require.Empty(t, got)
	require.ErrorContains(t, err, "processor execution failed: agent error:")
	require.Equal(t, "original", wf.Event.Annotations)
}

func TestExecuteSkillTool_MergesArgumentsWithoutChangingCachedConfig(t *testing.T) {
	settings, received := nestedAgentSettings(t)
	registry := NewSkillRegistry(t.TempDir())
	registry.toolsCache["triage"] = map[string]*SkillTool{
		"diagnose": {Name: "diagnose", Type: "ai.agent", Config: settings},
	}
	tool := &AgentTool{Name: "diagnose", SkillName: "triage"}
	wf := &models.WorkflowContext{Event: &models.AlertCurEvent{}, Inputs: map[string]string{"question": "inspect memory"}}

	got, err := executeSkillTool(nil, tool, map[string]interface{}{"output_field": "diagnosis"}, wf, registry)

	require.NoError(t, err)
	require.Contains(t, got, "Message: AI Agent completed: 1 iterations, success=true\n")
	require.Contains(t, got, `Updated annotations: {"diagnosis":"healthy"}`)
	require.Empty(t, wf.Event.Annotations)
	require.Nil(t, wf.Event.AnnotationsJSON)
	require.Equal(t, "original", settings["output_field"])
	request := <-received
	messages := request["messages"].([]interface{})
	require.Equal(t, "inspect memory", messages[len(messages)-1].(map[string]interface{})["content"])
}

func TestExecuteSkillTool_RejectsUnavailableTool(t *testing.T) {
	registry := NewSkillRegistry(t.TempDir())
	registry.toolsCache["triage"] = map[string]*SkillTool{"unknown": {Type: "round7.missing"}}
	cases := []struct {
		name     string
		tool     AgentTool
		registry *SkillRegistry
		want     string
	}{
		{name: "no registry", want: "skill registry not initialized"},
		{name: "no skill name", registry: registry, want: "skill_name not specified"},
		{
			name: "missing tool", tool: AgentTool{Name: "absent", SkillName: "triage"}, registry: registry,
			want: "failed to load skill tool 'absent'",
		},
		{
			name: "unknown processor", tool: AgentTool{Name: "unknown", SkillName: "triage"}, registry: registry,
			want: "failed to get processor 'round7.missing'",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := executeSkillTool(nil, &tc.tool, nil, &models.WorkflowContext{}, tc.registry)

			require.Empty(t, got)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestExecuteSkillTool_PropagatesExecutionFailure(t *testing.T) {
	previous := defaultClientCache
	defaultClientCache = &llm.ClientCache{}
	t.Cleanup(func() { defaultClientCache = previous })
	registry := NewSkillRegistry(t.TempDir())
	registry.toolsCache["triage"] = map[string]*SkillTool{
		"diagnose": {Type: "ai.agent", Config: map[string]interface{}{
			"llm": map[string]interface{}{"provider": "unsupported"},
		}},
	}
	wf := &models.WorkflowContext{Event: &models.AlertCurEvent{Tags: "host=web-1"}}

	got, err := executeSkillTool(nil, &AgentTool{Name: "diagnose", SkillName: "triage"}, nil, wf, registry)

	require.Empty(t, got)
	require.ErrorContains(t, err, "skill tool execution failed: agent error:")
	require.Equal(t, "host=web-1", wf.Event.Tags)
}
