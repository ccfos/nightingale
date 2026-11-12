package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/stretchr/testify/require"
)

func registerRunTestTool(t *testing.T, name string, handler BuiltinToolFunc) {
	t.Helper()
	previous, exists := builtinTools[name]
	RegisterBuiltinTool(name, &BuiltinTool{Definition: AgentTool{Name: name, Type: ToolTypeBuiltin}, Handler: handler})
	t.Cleanup(func() {
		if exists {
			builtinTools[name] = previous
		} else {
			delete(builtinTools, name)
		}
	})
}

func TestAgentRun_PreservesHistoryAndRendersPrompt(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{{{Content: "done"}}}}
	a := NewAgent(&AgentConfig{
		UserPromptTemplate: "{{.Params.host}}: {{.Vars.query}} / {{.Event}}",
	}, WithLLMClient(fake))
	history := []ChatMessage{{Role: "user", Content: "earlier"}, {Role: "assistant", Content: "acknowledged"}}
	req := &AgentRequest{
		History: history, Params: map[string]string{"host": "web-1"},
		Vars: map[string]interface{}{"query": "up"}, TemplateExtra: map[string]interface{}{"Event": "CPU high"},
	}

	resp, err := a.Run(context.Background(), req)

	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Equal(t, "done", resp.Content)
	require.Len(t, fake.reqs, 1)
	messages := fake.reqs[0].Messages
	require.Len(t, messages, 4)
	require.Equal(t, llm.RoleSystem, messages[0].Role)
	require.Equal(t, "earlier", messages[1].Content)
	require.Equal(t, "acknowledged", messages[2].Content)
	require.Equal(t, "web-1: up / CPU high", messages[3].Content)
	require.Equal(t, history, req.History)
}

func TestAgentRun_HiddenPinnedSkillDoesNotReachModel(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, "visible-run-skill", "visible description")
	writeTestSkill(t, root, "hidden-run-skill", "private description")
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{{{Content: "done"}}}}
	a := NewAgent(&AgentConfig{Skills: &SkillConfig{
		SkillNames:       []string{"visible-run-skill", "hidden-run-skill", "missing"},
		HiddenSkillNames: []string{"hidden-run-skill"},
	}}, WithLLMClient(fake))
	a.InitSkills(root)

	resp, err := a.Run(context.Background(), &AgentRequest{UserMessage: "inspect"})

	require.NoError(t, err)
	require.True(t, resp.Success)
	prompt := fake.reqs[0].Messages[0].Content
	require.Contains(t, prompt, "body")
	require.NotContains(t, prompt, "hidden-run-skill")
	require.NotContains(t, prompt, "private description")
	require.Empty(t, a.cfg.Tools, "per-run tool preparation must not mutate the configured tool list")
	require.Equal(t, root, a.toolDeps.SkillsPath)
}

func TestAgentRun_DenyAllSkillsOmitsCatalogAndPinnedContent(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, "denied-run-skill", "private description")
	path := filepath.Join(root, "denied-run-skill", SkillFileName)
	md := "---\nname: denied-run-skill\n---\nPrivate runbook: deny-all."
	require.NoError(t, os.WriteFile(path, []byte(md), 0o644))
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{{{Content: "done"}}}}
	a := NewAgent(&AgentConfig{Skills: &SkillConfig{
		SkillNames: []string{"denied-run-skill"}, DenyAllSkills: true,
	}}, WithLLMClient(fake))
	a.InitSkills(root)

	resp, err := a.Run(context.Background(), &AgentRequest{})

	require.NoError(t, err)
	require.True(t, resp.Success)
	require.NotContains(t, fake.reqs[0].Messages[0].Content, "denied-run-skill")
	require.NotContains(t, fake.reqs[0].Messages[0].Content, "Available Skills")
	require.NotContains(t, fake.reqs[0].Messages[0].Content, "Private runbook: deny-all.")
}

func TestAgentRun_UsesPinnedSkillIterationBudget(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, "budget-run-skill", "inspect")
	path := filepath.Join(root, "budget-run-skill", SkillFileName)
	md := "---\nname: budget-run-skill\nmax_iterations: 3\n---\nInspect."
	require.NoError(t, os.WriteFile(path, []byte(md), 0o644))
	tool := AgentTool{Name: "read_budget_fixture", Type: ToolTypeProcessor}
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{{ToolCalls: []llm.ToolCall{{Name: tool.Name, Arguments: `{}`}}}},
		{{ToolCalls: []llm.ToolCall{{Name: tool.Name, Arguments: `{}`}}}},
		{{Content: "finished after two lookups"}},
	}}
	a := NewAgent(&AgentConfig{
		MaxIterations: 1, Tools: []AgentTool{tool}, Skills: &SkillConfig{SkillNames: []string{"budget-run-skill"}},
	}, WithLLMClient(fake))
	a.InitSkills(root)
	a.SetExternalToolHandler(func(context.Context, *AgentTool, map[string]interface{}, *AgentRequest) (string, error) {
		return "observed", nil
	})

	resp, err := a.Run(context.Background(), &AgentRequest{})

	require.NoError(t, err)
	require.True(t, resp.Success)
	require.False(t, resp.Incomplete)
	require.Equal(t, 3, resp.Iterations)
	require.Equal(t, "finished after two lookups", resp.Content)
	require.Len(t, resp.Steps, 2)
	require.Equal(t, "call_0_0", fake.reqs[1].Messages[len(fake.reqs[1].Messages)-1].ToolCallID)
	require.Equal(t, "call_1_0", fake.reqs[2].Messages[len(fake.reqs[2].Messages)-1].ToolCallID)
}

func TestRunNativeLoop_CanceledContextSkipsLLM(t *testing.T) {
	fake := &scriptedNativeLLM{}
	a := NewAgent(&AgentConfig{}, WithLLMClient(fake))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp := a.runNativeLoop(ctx, &AgentRequest{}, nil, nil, &ToolLoopConfig{
		MaxIterations: 3, TimeoutMessage: "canceled turn",
	})

	require.False(t, resp.Success)
	require.Equal(t, "canceled turn", resp.Error)
	require.Zero(t, fake.call)
	require.Empty(t, resp.Steps)
}

func TestRunNativeLoop_StreamErrorStopsBeforeToolDispatch(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{{{Error: errors.New("provider unavailable")}}}}
	a := NewAgent(&AgentConfig{}, WithLLMClient(fake))
	stream := make(chan *StreamChunk, 10)

	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, nil, nil, &ToolLoopConfig{
		MaxIterations: 3, StreamChan: stream,
	})

	require.False(t, resp.Success)
	require.Contains(t, resp.Error, "provider unavailable")
	require.Equal(t, 1, fake.call)
	require.Empty(t, resp.Steps)
}

func TestAgentRun_WriteDedupResetsBetweenTurns(t *testing.T) {
	calls := 0
	registerRunTestTool(t, "create_turn_fixture", func(
		_ context.Context, _ *ToolDeps, args map[string]interface{}, _ map[string]string,
	) (string, error) {
		calls++
		require.Equal(t, "web-1", args["name"])
		return `{"created":true}`, nil
	})
	fake := &scriptedNativeLLM{}
	for i := 0; i < 2; i++ {
		fake.rounds = append(fake.rounds,
			[]llm.StreamChunk{{ToolCalls: []llm.ToolCall{
				{Name: "create_turn_fixture", Arguments: `{"name":"web-1"}`},
			}}},
			[]llm.StreamChunk{{Content: "created"}},
		)
	}
	a := NewAgent(&AgentConfig{}, WithLLMClient(fake))

	first, err := a.Run(context.Background(), &AgentRequest{})
	require.NoError(t, err)
	second, err := a.Run(context.Background(), &AgentRequest{})

	require.NoError(t, err)
	require.True(t, first.Success)
	require.True(t, second.Success)
	require.Equal(t, 2, calls, "a new user turn may repeat a previously completed write")
	require.Len(t, first.Steps, 1)
	require.Len(t, second.Steps, 1)
	require.JSONEq(t, `{"created":true}`, second.Steps[0].Observation)
}

func TestBuildUserMessage_RenderedInputPreservesTemplateSyntax(t *testing.T) {
	a := NewAgent(&AgentConfig{UserPromptRendered: "explain {{ .Alertname }}", UserPromptTemplate: "{{"})

	got, err := a.buildUserMessage(&AgentRequest{})

	require.NoError(t, err)
	require.Equal(t, "explain {{ .Alertname }}", got)
}

func TestBuildDefaultUserMessage_ExcludesCredentials(t *testing.T) {
	a := NewAgent(&AgentConfig{})

	got := a.buildDefaultUserMessage(&AgentRequest{UserMessage: "inspect CPU", Params: map[string]string{
		"host": "web-1", "API_KEY": "key-value", "password": "password-value",
		"auth_token": "token-value", "secret": "secret-value",
	}})

	require.Contains(t, got, "inspect CPU")
	require.Contains(t, got, "- host: web-1")
	for _, value := range []string{"key-value", "password-value", "token-value", "secret-value"} {
		require.NotContains(t, got, value)
	}
}

func TestExecuteHTTPTool_RendersAndSendsRequest(t *testing.T) {
	type receivedRequest struct {
		method string
		path   string
		header http.Header
		body   []byte
		err    error
	}
	received := make(chan receivedRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		received <- receivedRequest{method: r.Method, path: r.URL.Path, header: r.Header.Clone(), body: body, err: err}
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)
	a := NewAgent(&AgentConfig{})
	tool := &AgentTool{
		Name: "lookup", Type: ToolTypeHTTP, URL: srv.URL + "/lookup", Method: "POST",
		Headers: map[string]string{"X-Tenant": "ops"},
		BodyTemplate: `{"host":"{{.args.host}}","scope":"{{.params.scope}}",` +
			`"query":"{{.vars.query}}","event":"{{.event}}"}`,
	}

	out, interrupt := a.executeToolI(context.Background(), tool.Name, `{"host":"web-1"}`, &AgentRequest{
		Params: map[string]string{"scope": "prod"}, Vars: map[string]interface{}{"query": "up"},
		TemplateExtra: map[string]interface{}{"event": "CPU high"},
	}, []AgentTool{*tool})

	require.Nil(t, interrupt)
	require.JSONEq(t, `{"ok":true}`, out)
	got := <-received
	require.NoError(t, got.err)
	require.Equal(t, "POST", got.method)
	require.Equal(t, "/lookup", got.path)
	require.Equal(t, "ops", got.header.Get("X-Tenant"))
	require.Equal(t, "application/json", got.header.Get("Content-Type"))
	var body map[string]string
	require.NoError(t, json.Unmarshal(got.body, &body))
	require.Equal(t, map[string]string{"host": "web-1", "scope": "prod", "query": "up", "event": "CPU high"}, body)
}
