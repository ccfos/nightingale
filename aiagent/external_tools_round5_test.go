package aiagent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/stretchr/testify/require"
)

type round5ToolSource struct {
	tools       []AgentTool
	discoverErr error
	result      string
	callErr     error
	called      string
	args        map[string]interface{}
}

func (s *round5ToolSource) DiscoverTools(context.Context) ([]AgentTool, error) {
	return s.tools, s.discoverErr
}

func (s *round5ToolSource) CallTool(_ context.Context, tool *AgentTool, args map[string]interface{}) (string, error) {
	s.called = tool.Name
	s.args = args
	return s.result, s.callErr
}

func TestAppendSourceTools_KeepsFirstDefinitionAndBindsSource(t *testing.T) {
	failed := &round5ToolSource{discoverErr: errors.New("source unavailable")}
	first := &round5ToolSource{
		tools: []AgentTool{{Name: "configured"}, {Name: "remote", Description: "first definition"}}, result: "first result",
	}
	second := &round5ToolSource{tools: []AgentTool{{Name: "remote"}, {Name: "another"}}, result: "second result"}
	base := []AgentTool{{Name: "configured", Type: ToolTypeHTTP, Description: "local definition"}}
	a := NewAgent(&AgentConfig{ToolSources: []ToolSource{failed, first, second}})

	tools := a.appendSourceTools(context.Background(), base)

	require.Len(t, tools, 3)
	require.Equal(t, base[0], tools[0])
	require.Equal(t, "remote", tools[1].Name)
	require.Equal(t, "first definition", tools[1].Description)
	require.Equal(t, ToolTypeExternal, tools[1].Type)
	require.Same(t, first, tools[1].source)
	require.Equal(t, "another", tools[2].Name)
	require.Same(t, second, tools[2].source)
	require.Empty(t, first.tools[1].Type, "discovery must not mutate the source's definitions")
}

func TestAgentRun_DispatchesDiscoveredToolAndReplaysResult(t *testing.T) {
	source := &round5ToolSource{tools: []AgentTool{{Name: "round5_remote_lookup"}}, result: "CPU=12"}
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{{ToolCalls: []llm.ToolCall{{ID: "remote-call", Name: "round5_remote_lookup", Arguments: `{"host":"web-1"}`}}}},
		{{Content: "CPU healthy"}},
	}}
	a := NewAgent(&AgentConfig{ToolSources: []ToolSource{source}}, WithLLMClient(fake))

	response, err := a.Run(context.Background(), &AgentRequest{})

	require.NoError(t, err)
	require.True(t, response.Success)
	require.Equal(t, "CPU healthy", response.Content)
	require.Equal(t, "round5_remote_lookup", source.called)
	require.Equal(t, map[string]interface{}{"host": "web-1"}, source.args)
	require.Len(t, fake.reqs[0].Tools, 1)
	require.Equal(t, "round5_remote_lookup", fake.reqs[0].Tools[0].Name)
	turns := fake.reqs[1].Messages
	require.Equal(t, llm.RoleTool, turns[len(turns)-1].Role)
	require.Equal(t, "remote-call", turns[len(turns)-1].ToolCallID)
	require.Equal(t, "CPU=12", turns[len(turns)-1].Content)
	require.Empty(t, a.cfg.Tools, "discovered tools belong to this run only")
}

func TestExecuteExternalTool_TruncatesOversizedResults(t *testing.T) {
	source := &round5ToolSource{result: strings.Repeat("x", ToolOutputMaxBytes+10)}
	a := NewAgent(&AgentConfig{})
	tool := &AgentTool{Name: "remote", source: source}

	result := a.executeExternalTool(context.Background(), tool, nil)

	require.Equal(t, strings.Repeat("x", ToolOutputMaxBytes)+"\n... (truncated)", result)
	require.Equal(t, "remote", source.called)
}

func TestExecuteExternalTool_ReportsSourceErrors(t *testing.T) {
	cases := []struct {
		name   string
		source ToolSource
		want   string
	}{
		{name: "unbound", want: "Error: external tool 'remote' has no source bound"},
		{
			name: "source failure", source: &round5ToolSource{callErr: errors.New("access denied")},
			want: "Error: access denied",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAgent(&AgentConfig{})

			result := a.executeExternalTool(context.Background(), &AgentTool{Name: "remote", source: tc.source}, nil)

			require.Equal(t, tc.want, result)
		})
	}
}
