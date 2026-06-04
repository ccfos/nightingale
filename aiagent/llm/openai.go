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

const DefaultOpenAIURL = "https://api.openai.com/v1/chat/completions"

// maxRetries / initialRetryWait / maxRetryWait 迁移至 defaults.go

// OpenAI implements the LLM interface for OpenAI and compatible APIs
type OpenAI struct {
	config *Config
	client *http.Client
}

// NewOpenAI creates a new OpenAI provider
func NewOpenAI(cfg *Config, client *http.Client) (*OpenAI, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultOpenAIURL
	} else {
		cfg.BaseURL = NormalizeOpenAIURL(cfg.BaseURL)
	}
	return &OpenAI{
		config: cfg,
		client: client,
	}, nil
}

// NormalizeOpenAIURL 归一化 OpenAI 兼容端点：
// 用户常见填法包括 `https://host/v1` 或 `https://host/compatible-mode/v1`，
// 这里自动补 `/chat/completions`，避免漏路径时返回 404。
// 对于已知需要 /v1/chat/completions 路径的提供商（如 DeepSeek），
// 当用户只填写了根域名时自动补全完整路径。
func NormalizeOpenAIURL(rawURL string) string {
	u := strings.TrimRight(rawURL, "/")
	if strings.HasSuffix(u, "/chat/completions") {
		return u
	}
	if strings.HasSuffix(u, "/v1") {
		return u + "/chat/completions"
	}
	// DeepSeek 使用标准 OpenAI 兼容路径，用户填写根域名时自动补全
	if u == "https://api.deepseek.com" || u == "http://api.deepseek.com" {
		return u + "/v1/chat/completions"
	}
	return u
}

func (o *OpenAI) Name() string {
	return ProviderOpenAI
}

// OpenAI API request/response structures
type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Tools       []openAITool    `json:"tools,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
	Stream      bool            `json:"stream,omitempty"`

	// extraBody 不出现在 JSON tag 里——由 MarshalJSON 平铺到顶层。
	// 用于把 LLMConfig.ExtraBody 透传给厂商特定字段（如 dashscope 的 enable_thinking）。
	extraBody map[string]any
}

// MarshalJSON 把 extraBody 平铺到顶层 JSON。逻辑：先用别名走默认序列化（拿到所有
// 显式字段），unmarshal 回 map，再合并 extraBody，最后 marshal 出去。这种"先反射
// 再合并"的写法多绕一步，但好处是新增显式字段时不用同步改任何 reflection 代码。
// 已存在的 key 由显式字段优先（不让 extraBody 偷偷覆盖 model/messages 等关键字段）。
func (r openAIRequest) MarshalJSON() ([]byte, error) {
	type alias openAIRequest
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

type openAIMessage struct {
	Role             string           `json:"role"`
	Content          string           `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
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

func (o *OpenAI) GenerateStream(ctx context.Context, req *GenerateRequest) (<-chan StreamChunk, error) {
	openAIReq := o.convertRequest(req)
	openAIReq.Stream = true

	jsonData, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	// Debug 日志：确认 extraBody 等扩展字段是否真的被序列化进请求体。
	// 只在 ExtraBody 非空时打印，避免常态噪音；只打前 512 字节防止泄漏长 prompt。
	if len(o.config.ExtraBody) > 0 {
		preview := string(jsonData)
		if len(preview) > 512 {
			preview = preview[:512] + "...(truncated)"
		}
		logger.Debugf("[OpenAI] stream request body (with extra): %s", preview)
	}

	resp, err := doHTTPStreamWithRetry(ctx, o.client, "OpenAI",
		func() (*http.Request, error) {
			return http.NewRequestWithContext(ctx, "POST", o.config.BaseURL, bytes.NewBuffer(jsonData))
		},
		o.setHeaders,
	)
	if err != nil {
		return nil, err
	}

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
				Reasoning:    delta.ReasoningContent,
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
		Model:     o.config.Model,
		TopP:      req.TopP,
		Stop:      req.Stop,
		extraBody: o.config.ExtraBody,
	}

	// Temperature: request 优先，fallback 到 config 默认值
	switch {
	case req.Temperature != nil:
		openAIReq.Temperature = *req.Temperature
	case o.config.Temperature != nil:
		openAIReq.Temperature = *o.config.Temperature
	}

	// MaxTokens: 同上
	switch {
	case req.MaxTokens != nil:
		openAIReq.MaxTokens = *req.MaxTokens
	case o.config.MaxTokens != nil:
		openAIReq.MaxTokens = *o.config.MaxTokens
	}

	// Convert messages. 结构化工具轮：assistant 的 ToolCalls 与 RoleTool 结果轮
	// 直接映射到 OpenAI 原生 tool_calls/tool 角色。
	for _, msg := range req.Messages {
		oaMsg := openAIMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		for _, tc := range msg.ToolCalls {
			oaTC := openAIToolCall{ID: tc.ID, Type: "function"}
			oaTC.Function.Name = tc.Name
			oaTC.Function.Arguments = tc.Arguments
			oaMsg.ToolCalls = append(oaMsg.ToolCalls, oaTC)
		}
		openAIReq.Messages = append(openAIReq.Messages, oaMsg)
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
		result.ReasoningContent = choice.Message.ReasoningContent
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
	return doHTTPWithRetry(ctx, o.client, "OpenAI",
		func() (*http.Request, error) {
			return http.NewRequestWithContext(ctx, "POST", o.config.BaseURL, bytes.NewBuffer(jsonData))
		},
		o.setHeaders,
	)
}

func (o *OpenAI) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")

	if o.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.config.APIKey)
	}

	ApplyCustomHeaders(req, o.config.Headers)
}
