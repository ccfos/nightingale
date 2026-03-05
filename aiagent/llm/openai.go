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
	"time"
)

const (
	DefaultOpenAIURL = "https://api.openai.com/v1/chat/completions"

	// 重试相关配置
	maxRetries       = 3
	initialRetryWait = 5 * time.Second  // rate limit 时初始等待 5 秒
	maxRetryWait     = 60 * time.Second // 最大等待 60 秒
)

// OpenAI implements the LLM interface for OpenAI and compatible APIs
type OpenAI struct {
	config *Config
	client *http.Client
}

// NewOpenAI creates a new OpenAI provider
func NewOpenAI(cfg *Config, client *http.Client) (*OpenAI, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultOpenAIURL
	}
	return &OpenAI{
		config: cfg,
		client: client,
	}, nil
}

func (o *OpenAI) Name() string {
	return ProviderOpenAI
}

// OpenAI API request/response structures
type openAIRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIMessage     `json:"messages"`
	Tools       []openAITool        `json:"tools,omitempty"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Temperature float64             `json:"temperature,omitempty"`
	TopP        float64             `json:"top_p,omitempty"`
	Stop        []string            `json:"stop,omitempty"`
	Stream      bool                `json:"stream,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string           `json:"type"`
	Function openAIFunction   `json:"function"`
}

type openAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int           `json:"index"`
		Message      openAIMessage `json:"message"`
		Delta        openAIMessage `json:"delta"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

func (o *OpenAI) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	// Convert to OpenAI format
	openAIReq := o.convertRequest(req)
	openAIReq.Stream = false

	// Make request
	respBody, err := o.doRequest(ctx, openAIReq)
	if err != nil {
		return nil, err
	}

	// Parse response
	var openAIResp openAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if openAIResp.Error != nil {
		return nil, fmt.Errorf("OpenAI API error: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	// Convert to unified response
	return o.convertResponse(&openAIResp), nil
}

// isRetryableStatus 检查是否是可重试的 HTTP 状态码
func isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

func (o *OpenAI) GenerateStream(ctx context.Context, req *GenerateRequest) (<-chan StreamChunk, error) {
	// Convert to OpenAI format
	openAIReq := o.convertRequest(req)
	openAIReq.Stream = true

	// Create request body
	jsonData, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var resp *http.Response
	var lastErr error
	retryWait := initialRetryWait

	// 重试循环
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 等待后重试
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryWait):
			}
			// 指数退避，但不超过最大等待时间
			retryWait *= 2
			if retryWait > maxRetryWait {
				retryWait = maxRetryWait
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", o.config.BaseURL, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		o.setHeaders(httpReq)

		// Make request
		resp, err = o.client.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("failed to send request: %w", err)
			continue // 网络错误，重试
		}

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(body))

			// 检查是否可重试
			if isRetryableStatus(resp.StatusCode) && attempt < maxRetries {
				continue // 可重试的错误，继续重试
			}
			return nil, lastErr
		}

		// 成功，跳出循环
		break
	}

	if resp == nil {
		return nil, lastErr
	}

	// Create channel and start streaming
	ch := make(chan StreamChunk, 100)
	go o.streamResponse(ctx, resp, ch)

	return ch, nil
}

func (o *OpenAI) streamResponse(ctx context.Context, resp *http.Response, ch chan<- StreamChunk) {
	defer close(ch)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)

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
		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			ch <- StreamChunk{Done: true}
			return
		}

		var streamResp openAIResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			continue
		}

		if len(streamResp.Choices) > 0 {
			delta := streamResp.Choices[0].Delta
			chunk := StreamChunk{
				Content:      delta.Content,
				FinishReason: streamResp.Choices[0].FinishReason,
			}

			// Handle tool calls in stream
			if len(delta.ToolCalls) > 0 {
				for _, tc := range delta.ToolCalls {
					chunk.ToolCalls = append(chunk.ToolCalls, ToolCall{
						ID:        tc.ID,
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					})
				}
			}

			ch <- chunk
		}
	}
}

func (o *OpenAI) convertRequest(req *GenerateRequest) *openAIRequest {
	openAIReq := &openAIRequest{
		Model:       o.config.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
	}

	// Convert messages
	for _, msg := range req.Messages {
		openAIReq.Messages = append(openAIReq.Messages, openAIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Convert tools
	for _, tool := range req.Tools {
		openAIReq.Tools = append(openAIReq.Tools, openAITool{
			Type: "function",
			Function: openAIFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}

	return openAIReq
}

func (o *OpenAI) convertResponse(resp *openAIResponse) *GenerateResponse {
	result := &GenerateResponse{}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		result.Content = choice.Message.Content
		result.FinishReason = choice.FinishReason

		// Convert tool calls
		for _, tc := range choice.Message.ToolCalls {
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
	}

	if resp.Usage != nil {
		result.Usage = &Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	return result
}

func (o *OpenAI) doRequest(ctx context.Context, req *openAIRequest) ([]byte, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	retryWait := initialRetryWait

	// 重试循环
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 等待后重试
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryWait):
			}
			// 指数退避
			retryWait *= 2
			if retryWait > maxRetryWait {
				retryWait = maxRetryWait
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", o.config.BaseURL, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		o.setHeaders(httpReq)

		resp, err := o.client.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("failed to send request: %w", err)
			continue // 网络错误，重试
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(body))
			// 检查是否可重试
			if isRetryableStatus(resp.StatusCode) && attempt < maxRetries {
				continue
			}
			return nil, lastErr
		}

		return body, nil
	}

	return nil, lastErr
}

func (o *OpenAI) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")

	if o.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.config.APIKey)
	}

	for k, v := range o.config.Headers {
		req.Header.Set(k, v)
	}
}
