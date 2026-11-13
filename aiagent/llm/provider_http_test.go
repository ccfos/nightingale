package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type providerRequest struct {
	method string
	uri    string
	header http.Header
	body   map[string]any
	err    error
}

func providerHTTPServer(t *testing.T, status int, response string) (*httptest.Server, <-chan providerRequest) {
	t.Helper()
	received := make(chan providerRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := providerRequest{method: r.Method, uri: r.URL.RequestURI(), header: r.Header.Clone()}
		got.err = json.NewDecoder(r.Body).Decode(&got.body)
		received <- got
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, response)
	}))
	t.Cleanup(srv.Close)
	return srv, received
}

func TestClaudeGenerate_PostsMessagesAndParsesResponse(t *testing.T) {
	srv, received := providerHTTPServer(t, http.StatusOK, `{
		"content":[{"type":"text","text":"CPU "},{"type":"text","text":"normal"},
		{"type":"thinking","thinking":"inspect","signature":"sig"},
		{"type":"redacted_thinking","data":"opaque"},
		{"type":"tool_use","id":"c1","name":"lookup","input":{"id":7}}],
		"stop_reason":"tool_use","usage":{"input_tokens":11,"output_tokens":5}
	}`)
	client, err := NewClaude(&Config{
		Model: "claude-test", BaseURL: srv.URL + "/v1/", Headers: map[string]string{"X-Tenant": "ops"},
	}, srv.Client())
	require.NoError(t, err)
	limit := 99

	resp, err := client.Generate(context.Background(), &GenerateRequest{
		Messages:  []Message{{Role: RoleSystem, Content: "system rules"}, {Role: RoleUser, Content: "inspect CPU"}},
		MaxTokens: &limit,
		Tools: []ToolDefinition{{
			Name: "lookup", Description: "find a target", Parameters: map[string]interface{}{"type": "object"},
		}},
	})

	require.NoError(t, err)
	got := <-received
	require.NoError(t, got.err)
	require.Equal(t, "POST", got.method)
	require.Equal(t, "/v1/messages", got.uri)
	require.Equal(t, "application/json", got.header.Get("Content-Type"))
	require.Equal(t, ClaudeAPIVersion, got.header.Get("anthropic-version"))
	require.Equal(t, "ops", got.header.Get("X-Tenant"))
	require.Empty(t, got.header.Get("x-api-key"))
	require.Equal(t, "claude-test", got.body["model"])
	require.Equal(t, "system rules", got.body["system"])
	require.Equal(t, float64(99), got.body["max_tokens"])
	require.Equal(t, []any{map[string]any{
		"role": "user", "content": []any{map[string]any{"type": "text", "text": "inspect CPU"}},
	}}, got.body["messages"])
	require.Equal(t, []any{map[string]any{
		"name": "lookup", "description": "find a target", "input_schema": map[string]any{"type": "object"},
	}}, got.body["tools"])
	require.Equal(t, "CPU normal", resp.Content)
	require.Equal(t, "inspect", resp.ReasoningContent)
	require.Equal(t, "tool_use", resp.FinishReason)
	require.Equal(t, &Usage{PromptTokens: 11, CompletionTokens: 5, TotalTokens: 16}, resp.Usage)
	require.Equal(t, []ThinkingBlock{
		{Type: "thinking", Thinking: "inspect", Signature: "sig"},
		{Type: "redacted_thinking", Data: "opaque"},
	}, resp.ThinkingBlocks)
	require.Equal(t, []ToolCall{{ID: "c1", Name: "lookup", Arguments: `{"id":7}`}}, resp.ToolCalls)
}

func TestClaudeGenerateStream_AssemblesToolArgumentsAndThinking(t *testing.T) {
	sse := strings.Join([]string{
		"event: ping", "data: invalid-json",
		`data: {"type":"content_block_start","content_block":{"type":"thinking"}}`,
		`data: {"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"inspect"}}`,
		`data: {"type":"content_block_delta","delta":{"type":"signature_delta","signature":"sig"}}`,
		`data: {"type":"content_block_stop"}`,
		`data: {"type":"content_block_start","content_block":{"type":"redacted_thinking","data":"opaque"}}`,
		`data: {"type":"content_block_stop"}`,
		`data: {"type":"content_block_start","content_block":{"type":"tool_use","id":"c1","name":"lookup"}}`,
		`data: {"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{\"id\":"}}`,
		`data: {"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"7}"}}`,
		`data: {"type":"content_block_stop"}`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"checking"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		`data: {"type":"message_stop"}`,
	}, "\n\n") + "\n\n"
	srv, received := providerHTTPServer(t, http.StatusOK, sse)
	client, err := NewClaude(&Config{Model: "claude-test", BaseURL: srv.URL}, srv.Client())
	require.NoError(t, err)

	ch, err := client.GenerateStream(context.Background(), &GenerateRequest{
		Messages: []Message{UserMessage("inspect")},
	})

	require.NoError(t, err)
	var chunks []StreamChunk
	var content, reasoning string
	var thinking []ThinkingBlock
	for chunk := range ch {
		require.NoError(t, chunk.Error)
		chunks = append(chunks, chunk)
		content += chunk.Content
		reasoning += chunk.Reasoning
		if chunk.ThinkingBlock != nil {
			thinking = append(thinking, *chunk.ThinkingBlock)
		}
	}
	got := <-received
	require.NoError(t, got.err)
	require.Equal(t, true, got.body["stream"])
	require.Equal(t, "checking", content)
	require.Equal(t, "inspect", reasoning)
	require.Equal(t, []ThinkingBlock{
		{Type: "thinking", Thinking: "inspect", Signature: "sig"},
		{Type: "redacted_thinking", Data: "opaque"},
	}, thinking)
	require.Equal(t, []ToolCall{{ID: "c1", Name: "lookup", Arguments: `{"id":7}`}}, collectToolCalls(chunks))
	require.True(t, chunks[len(chunks)-1].Done)
	require.Equal(t, "tool_use", chunks[len(chunks)-2].FinishReason)
}

func TestClaudeGenerateStream_ReportsProviderError(t *testing.T) {
	srv, received := providerHTTPServer(t, http.StatusOK,
		"data: "+`{"type":"error","error":{"type":"overloaded_error","message":"capacity exhausted"}}`+"\n\n")
	client, err := NewClaude(&Config{Model: "claude-test", BaseURL: srv.URL}, srv.Client())
	require.NoError(t, err)

	ch, err := client.GenerateStream(context.Background(), &GenerateRequest{})

	require.NoError(t, err)
	var chunks []StreamChunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	require.Len(t, chunks, 1)
	require.True(t, chunks[0].Done)
	require.ErrorContains(t, chunks[0].Error, "capacity exhausted (overloaded_error)")
	require.NoError(t, (<-received).err)
}

func TestGeminiGenerate_PostsContentAndParsesUsage(t *testing.T) {
	srv, received := providerHTTPServer(t, http.StatusOK, `{
		"candidates":[{"content":{"role":"model","parts":[{"text":"inspect","thought":true},{"text":"normal"}]},
		"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":3,"totalTokenCount":11}
	}`)
	client, err := NewGemini(&Config{
		Model: "gemini-test", BaseURL: srv.URL + "/v1beta", Headers: map[string]string{"X-Tenant": "ops"},
	}, srv.Client())
	require.NoError(t, err)
	limit := 50

	resp, err := client.Generate(context.Background(), &GenerateRequest{
		Messages:  []Message{{Role: RoleSystem, Content: "system rules"}, {Role: RoleUser, Content: "inspect CPU"}},
		MaxTokens: &limit,
		Tools: []ToolDefinition{{
			Name: "lookup", Description: "find a target", Parameters: map[string]interface{}{"type": "object"},
		}},
	})

	require.NoError(t, err)
	got := <-received
	require.NoError(t, got.err)
	require.Equal(t, "POST", got.method)
	require.Equal(t, "/v1beta/models/gemini-test:generateContent?key=", got.uri)
	require.Equal(t, "application/json", got.header.Get("Content-Type"))
	require.Equal(t, "ops", got.header.Get("X-Tenant"))
	require.Equal(t, []any{map[string]any{
		"role": "user", "parts": []any{map[string]any{"text": "inspect CPU"}},
	}}, got.body["contents"])
	require.Equal(t, map[string]any{
		"parts": []any{map[string]any{"text": "system rules"}},
	}, got.body["systemInstruction"])
	require.Equal(t, float64(50), got.body["generationConfig"].(map[string]any)["maxOutputTokens"])
	require.Equal(t, []any{map[string]any{"functionDeclarations": []any{map[string]any{
		"name": "lookup", "description": "find a target", "parameters": map[string]any{"type": "object"},
	}}}}, got.body["tools"])
	require.Equal(t, "normal", resp.Content)
	require.Equal(t, "inspect", resp.ReasoningContent)
	require.Equal(t, "STOP", resp.FinishReason)
	require.Equal(t, &Usage{PromptTokens: 8, CompletionTokens: 3, TotalTokens: 11}, resp.Usage)
}

func TestGeminiGenerateStream_ParsesSplitJSONAndSignedCall(t *testing.T) {
	sse := "\n" + strings.Join([]string{
		`data: {"candidates":[{"content":{"parts":[`,
		`{"text":"inspect","thought":true},{"text":"checking"},`,
		`{"functionCall":{"name":"lookup","args":{"id":7}},"thoughtSignature":"sig"}]}}]}`,
		`data: {"candidates":[{"content":{"parts":[{"text":" done"}]},"finishReason":"MAX_TOKENS"}]}`,
	}, "\n") + "\n"
	srv, received := providerHTTPServer(t, http.StatusOK, sse)
	client, err := NewGemini(&Config{Model: "gemini-test", BaseURL: srv.URL}, srv.Client())
	require.NoError(t, err)

	ch, err := client.GenerateStream(context.Background(), &GenerateRequest{})

	require.NoError(t, err)
	var chunks []StreamChunk
	var content, reasoning string
	for chunk := range ch {
		require.NoError(t, chunk.Error)
		chunks = append(chunks, chunk)
		content += chunk.Content
		reasoning += chunk.Reasoning
	}
	got := <-received
	require.NoError(t, got.err)
	require.Equal(t, "/v1beta/models/gemini-test:streamGenerateContent?key=", got.uri)
	require.Equal(t, "checking done", content)
	require.Equal(t, "inspect", reasoning)
	require.Equal(t, []ToolCall{{
		Name: "lookup", Arguments: `{"id":7}`, ThoughtSignature: "sig",
	}}, collectToolCalls(chunks))
	require.Len(t, chunks, 3)
	require.Equal(t, "MAX_TOKENS", chunks[1].FinishReason)
	require.True(t, chunks[2].Done)
}

func TestGeminiGenerateStream_ReportsProviderError(t *testing.T) {
	body := `data: {"error":{"code":429,"message":"quota exhausted","status":"RESOURCE_EXHAUSTED"}}` + "\n"
	srv, _ := providerHTTPServer(t, http.StatusOK, body)
	client, err := NewGemini(&Config{Model: "gemini-test", BaseURL: srv.URL}, srv.Client())
	require.NoError(t, err)

	ch, err := client.GenerateStream(context.Background(), &GenerateRequest{})

	require.NoError(t, err)
	var chunks []StreamChunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	require.Len(t, chunks, 1)
	require.True(t, chunks[0].Done)
	require.ErrorContains(t, chunks[0].Error, "quota exhausted")
	require.ErrorContains(t, chunks[0].Error, "RESOURCE_EXHAUSTED")
}

func TestProviderGenerate_RejectsInvalidResponses(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		status   int
		body     string
		error    string
	}{
		{name: "Claude invalid JSON", provider: "claude", status: 200, body: "{", error: "failed to parse response"},
		{name: "Gemini invalid JSON", provider: "gemini", status: 200, body: "{", error: "failed to parse response"},
		{name: "OpenAI invalid JSON", provider: "openai", status: 200, body: "{", error: "failed to parse response"},
		{
			name: "Claude API error", provider: "claude", status: 200,
			body: `{"error":{"message":"bad model"}}`, error: "bad model",
		},
		{
			name: "Gemini API error", provider: "gemini", status: 200,
			body: `{"error":{"message":"bad model"}}`, error: "bad model",
		},
		{
			name: "OpenAI API error", provider: "openai", status: 200,
			body: `{"error":{"message":"bad model"}}`, error: "bad model",
		},
		{name: "Claude HTTP error", provider: "claude", status: 403, body: "forbidden", error: "status 403"},
		{name: "Gemini HTTP error", provider: "gemini", status: 403, body: "forbidden", error: "status 403"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, received := providerHTTPServer(t, tc.status, tc.body)
			cfg := &Config{Model: "test", BaseURL: srv.URL}
			var client LLM
			var err error
			switch tc.provider {
			case "claude":
				client, err = NewClaude(cfg, srv.Client())
			case "gemini":
				client, err = NewGemini(cfg, srv.Client())
			default:
				client, err = NewOpenAI(cfg, srv.Client())
			}
			require.NoError(t, err)

			resp, err := client.Generate(context.Background(), &GenerateRequest{})

			require.Nil(t, resp)
			require.ErrorContains(t, err, tc.error)
			require.NoError(t, (<-received).err)
		})
	}
}
