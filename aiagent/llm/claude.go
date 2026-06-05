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

	"github.com/toolkits/pkg/logger"
)

const (
	DefaultClaudeURL       = "https://api.anthropic.com/v1/messages"
	ClaudeAPIVersion       = "2023-06-01"
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
	Stream      bool            `json:"stream,omitempty"`
	Tools       []claudeTool    `json:"tools,omitempty"`

	// extraBody 不出现在 JSON tag 里——由 MarshalJSON 平铺到顶层。
	// Kimi（走 Claude provider）的 thinking:{type:disabled} 通过这里注入。
	extraBody map[string]any
}

// MarshalJSON 把 extraBody 平铺到顶层 JSON。逻辑与 openAIRequest.MarshalJSON 一致：
// 先用别名走默认序列化（拿到所有显式字段），unmarshal 回 map，再合并 extraBody，
// 最后 marshal 出去。显式字段优先，不让 extraBody 覆盖 model/messages 等关键字段。
func (r claudeRequest) MarshalJSON() ([]byte, error) {
	type alias claudeRequest
	data, err := json.Marshal(alias(r))
	if err != nil {
		return nil, err
	}
	if len(r.extraBody) == 0 {
		return data, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	for k, v := range r.extraBody {
		if _, occupied := m[k]; occupied {
			continue
		}
		m[k] = v
	}
	return json.Marshal(m)
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

	// 扩展思考块（type=thinking / redacted_thinking）。开启 thinking 后工具
	// 续轮必须把这些块带签名回填进 assistant 消息（native 下放开 thinking 的前提）。
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
	Data      string `json:"data,omitempty"`
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
	Type         string              `json:"type"`
	Index        int                 `json:"index,omitempty"`
	ContentBlock *claudeContentBlock `json:"content_block,omitempty"`
	Delta        *claudeStreamDelta  `json:"delta,omitempty"`
	Message      *claudeResponse     `json:"message,omitempty"`
	Usage        *claudeStreamUsage  `json:"usage,omitempty"`
	// Error 仅 type=error 事件：服务端的具体失败原因（限流/超长/超额等），
	// 不解析就只剩一句没有信息量的 "stream error"。
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type claudeStreamDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
	Thinking    string `json:"thinking,omitempty"`  // thinking_delta 增量
	Signature   string `json:"signature,omitempty"` // signature_delta（思考块收尾签名）
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
	if len(c.config.ExtraBody) > 0 {
		preview := string(jsonData)
		if len(preview) > 512 {
			preview = preview[:512] + "...(truncated)"
		}
		logger.Debugf("[Claude] stream request body (with extra): %s", preview)
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
	var currentThinking *ThinkingBlock // 正在累积的扩展思考块（thinking/redacted_thinking）

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
			if event.ContentBlock != nil {
				switch event.ContentBlock.Type {
				case "tool_use":
					currentToolCall = &ToolCall{
						ID:   event.ContentBlock.ID,
						Name: event.ContentBlock.Name,
					}
				case "thinking":
					currentThinking = &ThinkingBlock{Type: "thinking"}
				case "redacted_thinking":
					// 整块随 start 事件给出（加密数据，无增量）。
					currentThinking = &ThinkingBlock{Type: "redacted_thinking", Data: event.ContentBlock.Data}
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
				case "thinking_delta":
					if currentThinking != nil {
						currentThinking.Thinking += event.Delta.Thinking
					}
					chunk.Reasoning = event.Delta.Thinking // 增量实时展示
				case "signature_delta":
					if currentThinking != nil {
						currentThinking.Signature += event.Delta.Signature
					}
				}

				if chunk.Content != "" || chunk.Reasoning != "" {
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
			if currentThinking != nil {
				// 完整思考块（含签名）交给调用方，随 assistant 轮回填（续轮硬性要求）。
				ch <- StreamChunk{ThinkingBlock: currentThinking}
				currentThinking = nil
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
			if event.Error != nil && event.Error.Message != "" {
				ch <- StreamChunk{Done: true, Error: fmt.Errorf("stream error: %s (%s)", event.Error.Message, event.Error.Type)}
			} else {
				ch <- StreamChunk{Done: true, Error: fmt.Errorf("stream error")}
			}
			return
		}
	}
}

func (c *Claude) convertRequest(req *GenerateRequest) *claudeRequest {
	claudeReq := &claudeRequest{
		Model:     c.config.Model,
		TopP:      req.TopP,
		extraBody: c.config.ExtraBody,
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

		// 工具结果轮：Claude 没有 tool 角色，tool_result
		// 是 user 轮的 content block。连续多条结果（并行工具调用）必须合并进同
		// 一个 user 轮 —— Messages API 要求 user/assistant 严格交替。
		if msg.Role == RoleTool {
			block := claudeContentBlock{Type: "tool_result", ToolUseID: msg.ToolCallID, Content: msg.Content}
			if n := len(claudeReq.Messages); n > 0 && claudeReq.Messages[n-1].Role == RoleUser &&
				len(claudeReq.Messages[n-1].Content) > 0 && claudeReq.Messages[n-1].Content[0].Type == "tool_result" {
				claudeReq.Messages[n-1].Content = append(claudeReq.Messages[n-1].Content, block)
			} else {
				claudeReq.Messages = append(claudeReq.Messages, claudeMessage{
					Role:    RoleUser,
					Content: []claudeContentBlock{block},
				})
			}
			continue
		}

		// Claude uses content blocks instead of plain strings.
		// assistant 轮块顺序：thinking 块（如有，必须在最前——扩展思考开启时
		// API 要求 assistant 消息以 thinking 块开头）→ text block（如有）→
		// tool_use blocks。纯文本轮保持原有"单 text block"形态（空文本也保留，
		// 维持旧行为）。
		claudeMsg := claudeMessage{Role: msg.Role}
		for _, tb := range msg.ThinkingBlocks {
			claudeMsg.Content = append(claudeMsg.Content, claudeContentBlock{
				Type:      tb.Type,
				Thinking:  tb.Thinking,
				Signature: tb.Signature,
				Data:      tb.Data,
			})
		}
		if msg.Content != "" || (len(msg.ToolCalls) == 0 && len(msg.ThinkingBlocks) == 0) {
			claudeMsg.Content = append(claudeMsg.Content, claudeContentBlock{Type: "text", Text: msg.Content})
		}
		for _, tc := range msg.ToolCalls {
			claudeMsg.Content = append(claudeMsg.Content, claudeContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Name,
				Input: parseToolJSONObject(tc.Arguments, "input"),
			})
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

	// Extract text content, thinking blocks and tool calls
	var textParts []string
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "thinking":
			result.ReasoningContent += block.Thinking
			result.ThinkingBlocks = append(result.ThinkingBlocks, ThinkingBlock{
				Type: "thinking", Thinking: block.Thinking, Signature: block.Signature,
			})
		case "redacted_thinking":
			result.ThinkingBlocks = append(result.ThinkingBlocks, ThinkingBlock{
				Type: "redacted_thinking", Data: block.Data,
			})
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

	ApplyCustomHeaders(req, c.config.Headers)
}
