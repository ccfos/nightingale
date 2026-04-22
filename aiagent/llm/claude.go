package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	DefaultClaudeURL     = "https://api.anthropic.com/v1/messages"
	ClaudeAPIVersion     = "2023-06-01"
	DefaultClaudeMaxTokens = 4096
)

// Claude implements the LLM interface for Anthropic Claude API
type Claude struct {
	config *Config
	client *http.Client
}

// NewClaude creates a new Claude provider
func NewClaude(cfg *Config, client *http.Client) (*Claude, error) {
	cfg.BaseURL = NormalizeClaudeURL(cfg.BaseURL)
	return &Claude{
		config: cfg,
		client: client,
	}, nil
}

func (c *Claude) Name() string {
	return ProviderClaude
}

// Claude API request/response structures
type claudeRequest struct {
	Model       string          `json:"model"`
	Messages    []claudeMessage `json:"messages"`
	System      string          `json:"system,omitempty"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Stop        []string        `json:"stop_sequences,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Tools       []claudeTool    `json:"tools,omitempty"`
}

type claudeMessage struct {
	Role    string               `json:"role"`
	Content []claudeContentBlock `json:"content"`
}

type claudeContentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     any    `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

type claudeTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type claudeResponse struct {
	ID           string               `json:"id"`
	Type         string               `json:"type"`
	Role         string               `json:"role"`
	Content      []claudeContentBlock `json:"content"`
	Model        string               `json:"model"`
	StopReason   string               `json:"stop_reason"`
	StopSequence string               `json:"stop_sequence,omitempty"`
	Usage        *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Claude streaming event types
type claudeStreamEvent struct {
	Type         string               `json:"type"`
	Index        int                  `json:"index,omitempty"`
	ContentBlock *claudeContentBlock  `json:"content_block,omitempty"`
	Delta        *claudeStreamDelta   `json:"delta,omitempty"`
	Message      *claudeResponse      `json:"message,omitempty"`
	Usage        *claudeStreamUsage   `json:"usage,omitempty"`
}

type claudeStreamDelta struct {
	Type         string `json:"type"`
	Text         string `json:"text,omitempty"`
	PartialJSON  string `json:"partial_json,omitempty"`
	StopReason   string `json:"stop_reason,omitempty"`
}

type claudeStreamUsage struct {
	OutputTokens int `json:"output_tokens"`
}

func (c *Claude) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	claudeReq := c.convertRequest(req)
	claudeReq.Stream = false

	respBody, err := c.doRequest(ctx, claudeReq)
	if err != nil {
		return nil, err
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if claudeResp.Error != nil {
		return nil, fmt.Errorf("Claude API error: %s", claudeResp.Error.Message)
	}

	return c.convertResponse(&claudeResp), nil
}

func (c *Claude) GenerateStream(ctx context.Context, req *GenerateRequest) (<-chan StreamChunk, error) {
	claudeReq := c.convertRequest(req)
	claudeReq.Stream = true

	jsonData, err := json.Marshal(claudeReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := doHTTPStreamWithRetry(ctx, c.client, "Claude",
		func() (*http.Request, error) {
			return http.NewRequestWithContext(ctx, "POST", c.config.BaseURL, bytes.NewBuffer(jsonData))
		},
		c.setHeaders,
	)
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamChunk, 100)
	go c.streamResponse(ctx, resp, ch)
	return ch, nil
}

func (c *Claude) streamResponse(ctx context.Context, resp *http.Response, ch chan<- StreamChunk) {
	defer close(ch)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	var currentToolCall *ToolCall

	for {
		select {
		case <-ctx.Done():
			ch <- StreamChunk{Done: true, Error: ctx.Err()}
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				ch <- StreamChunk{Done: true, Error: err}
			} else {
				ch <- StreamChunk{Done: true}
			}
			return
		}

		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		var event claudeStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_start":
			if event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
				currentToolCall = &ToolCall{
					ID:   event.ContentBlock.ID,
					Name: event.ContentBlock.Name,
				}
			}

		case "content_block_delta":
			if event.Delta != nil {
				chunk := StreamChunk{}

				switch event.Delta.Type {
				case "text_delta":
					chunk.Content = event.Delta.Text
				case "input_json_delta":
					if currentToolCall != nil {
						currentToolCall.Arguments += event.Delta.PartialJSON
					}
				}

				if chunk.Content != "" {
					ch <- chunk
				}
			}

		case "content_block_stop":
			if currentToolCall != nil {
				ch <- StreamChunk{
					ToolCalls: []ToolCall{*currentToolCall},
				}
				currentToolCall = nil
			}

		case "message_delta":
			if event.Delta != nil && event.Delta.StopReason != "" {
				ch <- StreamChunk{
					FinishReason: event.Delta.StopReason,
				}
			}

		case "message_stop":
			ch <- StreamChunk{Done: true}
			return

		case "error":
			ch <- StreamChunk{Done: true, Error: fmt.Errorf("stream error")}
			return
		}
	}
}

func (c *Claude) convertRequest(req *GenerateRequest) *claudeRequest {
	claudeReq := &claudeRequest{
		Model: c.config.Model,
		TopP:  req.TopP,
		Stop:  req.Stop,
	}

	switch {
	case req.Temperature != nil:
		claudeReq.Temperature = *req.Temperature
	case c.config.Temperature != nil:
		claudeReq.Temperature = *c.config.Temperature
	}

	switch {
	case req.MaxTokens != nil:
		claudeReq.MaxTokens = *req.MaxTokens
	case c.config.MaxTokens != nil:
		claudeReq.MaxTokens = *c.config.MaxTokens
	}

	if claudeReq.MaxTokens <= 0 {
		claudeReq.MaxTokens = DefaultClaudeMaxTokens
	}

	// Extract system message and convert other messages
	for _, msg := range req.Messages {
		if msg.Role == RoleSystem {
			claudeReq.System = msg.Content
			continue
		}

		// Claude uses content blocks instead of plain strings
		claudeMsg := claudeMessage{
			Role: msg.Role,
			Content: []claudeContentBlock{
				{Type: "text", Text: msg.Content},
			},
		}
		claudeReq.Messages = append(claudeReq.Messages, claudeMsg)
	}

	// Convert tools
	for _, tool := range req.Tools {
		claudeReq.Tools = append(claudeReq.Tools, claudeTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.Parameters,
		})
	}

	return claudeReq
}

func (c *Claude) convertResponse(resp *claudeResponse) *GenerateResponse {
	result := &GenerateResponse{
		FinishReason: resp.StopReason,
	}

	// Extract text content and tool calls
	var textParts []string
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			inputJSON, _ := json.Marshal(block.Input)
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(inputJSON),
			})
		}
	}

	result.Content = strings.Join(textParts, "")

	if resp.Usage != nil {
		result.Usage = &Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		}
	}

	return result
}

func (c *Claude) doRequest(ctx context.Context, req *claudeRequest) ([]byte, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	return doHTTPWithRetry(ctx, c.client, "Claude",
		func() (*http.Request, error) {
			return http.NewRequestWithContext(ctx, "POST", c.config.BaseURL, bytes.NewBuffer(jsonData))
		},
		c.setHeaders,
	)
}

func (c *Claude) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", ClaudeAPIVersion)

	if c.config.APIKey != "" {
		req.Header.Set("x-api-key", c.config.APIKey)
	}

	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}
}
