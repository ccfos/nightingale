package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatWithSystem_SendsOrderedMessages(t *testing.T) {
	srv, received := providerHTTPServer(t, 200, `{"choices":[{"message":{"content":"healthy"},"finish_reason":"stop"}]}`)
	client, err := NewOpenAI(&Config{BaseURL: srv.URL, Model: "fixture"}, srv.Client())
	require.NoError(t, err)

	content, err := ChatWithSystem(context.Background(), client, "inspect metrics", "is CPU healthy?")

	require.NoError(t, err)
	require.Equal(t, "healthy", content)
	request := <-received
	require.NoError(t, request.err)
	require.Equal(t, []any{
		map[string]any{"role": "system", "content": "inspect metrics"},
		map[string]any{"role": "user", "content": "is CPU healthy?"},
	}, request.body["messages"])
}

func TestChat_PropagatesProviderError(t *testing.T) {
	srv, received := providerHTTPServer(t, 401, `{"error":{"message":"invalid key"}}`)
	client, err := NewOpenAI(&Config{BaseURL: srv.URL}, srv.Client())
	require.NoError(t, err)

	content, err := Chat(context.Background(), client, []Message{UserMessage("inspect")})

	require.Empty(t, content)
	require.ErrorContains(t, err, "401")
	require.NoError(t, (<-received).err)
}

func TestCollectStream_AssemblesContentAndToolCalls(t *testing.T) {
	stream := make(chan StreamChunk, 4)
	stream <- StreamChunk{Content: "CPU "}
	stream <- StreamChunk{ToolCalls: []ToolCall{{ID: "one", Name: "query", Arguments: `{"expr":"up"}`}}}
	stream <- StreamChunk{Content: "healthy", FinishReason: "tool_calls"}
	stream <- StreamChunk{ToolCalls: []ToolCall{{ID: "two", Name: "detail", Arguments: `{}`}}, FinishReason: "stop"}
	close(stream)

	response, err := CollectStream(stream)

	require.NoError(t, err)
	require.Equal(t, "CPU healthy", response.Content)
	require.Equal(t, "stop", response.FinishReason)
	require.Equal(t, []ToolCall{
		{ID: "one", Name: "query", Arguments: `{"expr":"up"}`},
		{ID: "two", Name: "detail", Arguments: `{}`},
	}, response.ToolCalls)
}

func TestCollectStream_ReturnsLastErrorWithoutPartialSuccess(t *testing.T) {
	lastError := errors.New("stream interrupted")
	stream := make(chan StreamChunk, 3)
	stream <- StreamChunk{Content: "partial"}
	stream <- StreamChunk{Error: errors.New("first failure")}
	stream <- StreamChunk{Error: lastError}
	close(stream)

	response, err := CollectStream(stream)

	require.Nil(t, response)
	require.ErrorIs(t, err, lastError)
}

func TestDetectProvider_RecognizesEndpointFamilies(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{url: "https://API.ANTHROPIC.COM/v1/messages", want: ProviderClaude},
		{url: "https://generativelanguage.googleapis.com/v1beta", want: ProviderGemini},
		{url: "https://us-central1-aiplatform.googleapis.com", want: ProviderVertex},
		{url: "https://bedrock-runtime.us-east-1.amazonaws.com", want: ProviderBedrock},
		{url: "http://localhost:11434/v1", want: ProviderOllama},
		{url: "https://api.kimi.com/coding", want: ProviderKimi},
		{url: "http://127.0.0.1:1234/v1", want: ProviderOpenAI},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			got := DetectProvider(tc.url)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestDetectProviderFromModel_RecognizesModelFamilies(t *testing.T) {
	cases := []struct {
		model string
		want  string
	}{
		{model: "CLAUDE-sonnet", want: ProviderClaude},
		{model: "gemini-pro", want: ProviderGemini},
		{model: "gpt-4", want: ProviderOpenAI},
		{model: "o1-mini", want: ProviderOpenAI},
		{model: "o3-mini", want: ProviderOpenAI},
		{model: "llama3", want: ProviderOllama},
		{model: "mistral", want: ProviderOllama},
		{model: "qwen3", want: ProviderOllama},
		{model: "kimi-code", want: ProviderKimi},
		{model: "custom", want: ProviderOpenAI},
	}
	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			got := DetectProviderFromModel(tc.model)

			require.Equal(t, tc.want, got)
		})
	}
}
