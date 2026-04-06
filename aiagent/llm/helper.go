package llm

import (
	"context"
	"strings"
)

// Chat is a convenience function for simple chat completions
func Chat(ctx context.Context, llm LLM, messages []Message) (string, error) {
	resp, err := llm.Generate(ctx, &GenerateRequest{
		Messages: messages,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// ChatWithSystem is a convenience function for chat with a system prompt
func ChatWithSystem(ctx context.Context, llm LLM, systemPrompt string, userMessage string) (string, error) {
	messages := []Message{
		{Role: RoleSystem, Content: systemPrompt},
		{Role: RoleUser, Content: userMessage},
	}
	return Chat(ctx, llm, messages)
}

// NewMessage creates a new message
func NewMessage(role, content string) Message {
	return Message{Role: role, Content: content}
}

// SystemMessage creates a system message
func SystemMessage(content string) Message {
	return Message{Role: RoleSystem, Content: content}
}

// UserMessage creates a user message
func UserMessage(content string) Message {
	return Message{Role: RoleUser, Content: content}
}

// AssistantMessage creates an assistant message
func AssistantMessage(content string) Message {
	return Message{Role: RoleAssistant, Content: content}
}

// DetectProvider attempts to detect the provider from the base URL
func DetectProvider(baseURL string) string {
	baseURL = strings.ToLower(baseURL)

	switch {
	case strings.Contains(baseURL, "anthropic.com"):
		return ProviderClaude
	case strings.Contains(baseURL, "generativelanguage.googleapis.com"):
		return ProviderGemini
	case strings.Contains(baseURL, "aiplatform.googleapis.com"):
		return ProviderVertex
	case strings.Contains(baseURL, "bedrock"):
		return ProviderBedrock
	case strings.Contains(baseURL, "localhost:11434"):
		return ProviderOllama
	case strings.Contains(baseURL, "api.kimi.com"):
		return ProviderKimi
	default:
		// Default to OpenAI-compatible
		return ProviderOpenAI
	}
}

// DetectProviderFromModel attempts to detect the provider from the model name
func DetectProviderFromModel(model string) string {
	model = strings.ToLower(model)

	switch {
	case strings.HasPrefix(model, "claude"):
		return ProviderClaude
	case strings.HasPrefix(model, "gemini"):
		return ProviderGemini
	case strings.HasPrefix(model, "gpt") || strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3"):
		return ProviderOpenAI
	case strings.HasPrefix(model, "llama") || strings.HasPrefix(model, "mistral") || strings.HasPrefix(model, "qwen"):
		return ProviderOllama
	case strings.HasPrefix(model, "kimi"):
		return ProviderKimi
	default:
		return ProviderOpenAI
	}
}

// BuildToolDefinition creates a tool definition with JSON schema parameters
func BuildToolDefinition(name, description string, properties map[string]interface{}, required []string) ToolDefinition {
	params := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		params["required"] = required
	}

	return ToolDefinition{
		Name:        name,
		Description: description,
		Parameters:  params,
	}
}

// CollectStream collects all chunks from a stream into a single response
func CollectStream(ch <-chan StreamChunk) (*GenerateResponse, error) {
	var content strings.Builder
	var toolCalls []ToolCall
	var finishReason string
	var lastErr error

	for chunk := range ch {
		if chunk.Error != nil {
			lastErr = chunk.Error
		}
		if chunk.Content != "" {
			content.WriteString(chunk.Content)
		}
		if len(chunk.ToolCalls) > 0 {
			toolCalls = append(toolCalls, chunk.ToolCalls...)
		}
		if chunk.FinishReason != "" {
			finishReason = chunk.FinishReason
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return &GenerateResponse{
		Content:      content.String(),
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
	}, nil
}
