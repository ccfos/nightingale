// Package llm provides a unified interface for multiple LLM providers.
// Supports OpenAI-compatible APIs, Claude/Anthropic, and Gemini.
package llm

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Provider types
const (
	ProviderOpenAI  = "openai"  // OpenAI and compatible APIs (Azure, vLLM, etc.)
	ProviderClaude  = "claude"  // Anthropic Claude
	ProviderGemini  = "gemini"  // Google Gemini
	ProviderOllama  = "ollama"  // Ollama local models
	ProviderBedrock = "bedrock" // AWS Bedrock
	ProviderVertex  = "vertex"  // Google Vertex AI
	ProviderKimi    = "kimi"    // Kimi Code (OpenAI-compatible)
)

// Role constants
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	// RoleTool 标记一条工具结果消息（原生 function calling 协议）。各 provider
	// 适配：OpenAI → role:"tool"+tool_call_id；Claude → user 轮 tool_result block；
	// Gemini → user 轮 functionResponse part（按 ToolName 匹配）。
	RoleTool = "tool"
)

// Message represents a chat message
//
// 工具相关字段构成统一的"结构化工具轮"表示，由各 provider 的 convertRequest
// 翻译成原生形态。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`

	// ToolCalls 仅 assistant 轮：本轮发起的工具调用。
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID/ToolName 仅 RoleTool 结果轮：ID 对应 assistant 轮 ToolCall.ID
	// （OpenAI tool_call_id / Claude tool_use_id）；ToolName 是函数名——Gemini
	// 的 functionResponse 按名字而非 id 匹配，必须带上。
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`

	// ThinkingBlocks 仅 assistant 轮：Anthropic 扩展思考块（带签名），续轮回填用。
	ThinkingBlocks []ThinkingBlock `json:"thinking_blocks,omitempty"`
}

// ToolCall represents a tool/function call from the LLM
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`

	// ThoughtSignature 是 Gemini 思考模型附在 functionCall part 上的签名，
	// 续轮请求必须原样回传（Gemini 3 缺失会 4xx）。其它 provider 恒空。
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

// ThinkingBlock 是 Anthropic 扩展思考的内容块（Kimi 的 Claude 兼容线同语义）。
// 开启 thinking 后工具续轮必须把上一条 assistant 消息的思考块**带签名原样回填**，
// 否则 API 报 400——这是"有状态思考协议"，与 OpenAI 系无状态的 reasoning_content
// 本质不同。Type: "thinking"（Thinking+Signature）| "redacted_thinking"（Data）。
type ThinkingBlock struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
	Data      string `json:"data,omitempty"`
}

// ToolDefinition defines a tool that the LLM can call
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// GenerateRequest is the unified request for LLM generation
type GenerateRequest struct {
	Messages    []Message        `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	MaxTokens   *int             `json:"max_tokens,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	TopP        float64          `json:"top_p,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
}

// GenerateResponse is the unified response from LLM generation
type GenerateResponse struct {
	Content          string          `json:"content"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ThinkingBlocks   []ThinkingBlock `json:"thinking_blocks,omitempty"` // Anthropic 系：续轮回填用
	ToolCalls        []ToolCall      `json:"tool_calls,omitempty"`
	FinishReason     string          `json:"finish_reason"`
	Usage            *Usage          `json:"usage,omitempty"`
}

// Usage represents token usage statistics
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamChunk represents a chunk in streaming response
type StreamChunk struct {
	Content string `json:"content,omitempty"`
	// Reasoning 是思考模型的推理增量（OpenAI 兼容端点的 delta.reasoning_content、
	// Anthropic 的 thinking_delta、Gemini 的 thought part），用于实时展示。
	// 与 Content 分通道，宿主可分别路由到 thinking/answer。
	Reasoning string `json:"reasoning,omitempty"`
	// ThinkingBlock 是 Anthropic 系思考块收尾时（content_block_stop）发出的完整
	// 块（含签名），调用方累积后随 assistant 轮回填。与 Reasoning 增量并存：
	// 增量管展示，块管协议正确性。
	ThinkingBlock *ThinkingBlock `json:"thinking_block,omitempty"`
	ToolCalls     []ToolCall     `json:"tool_calls,omitempty"`
	FinishReason  string         `json:"finish_reason,omitempty"`
	Done          bool           `json:"done"`
	Error         error          `json:"error,omitempty"`
}

// parseToolJSONObject 把工具调用参数/结果字符串解析为 JSON 对象——Claude 的
// tool_use.input、Gemini 的 functionCall.args / functionResponse.response 都要求
// 对象形态。非对象 JSON（数组/标量）或非 JSON 文本退化为 {fallbackKey: raw}，
// 空串退化为空对象，保证请求始终合法。
func parseToolJSONObject(raw, fallbackKey string) map[string]interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]interface{}{}
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err == nil && m != nil {
		return m
	}
	return map[string]interface{}{fallbackKey: raw}
}

// LLM is the unified interface for all LLM providers
type LLM interface {
	// Name returns the provider name
	Name() string

	// Generate sends a request to the LLM and returns the response
	Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)

	// GenerateStream sends a request and returns a channel for streaming responses
	GenerateStream(ctx context.Context, req *GenerateRequest) (<-chan StreamChunk, error)
}

// Config is the configuration for creating an LLM provider
type Config struct {
	// Provider type: openai, claude, gemini, ollama, bedrock, vertex
	Provider string `json:"provider"`

	// API endpoint URL
	BaseURL string `json:"base_url,omitempty"`

	// API key or token
	APIKey string `json:"api_key,omitempty"`

	// Model name (e.g., "gpt-4", "claude-3-opus", "gemini-pro")
	Model string `json:"model"`

	// Additional headers for API requests
	Headers map[string]string `json:"headers,omitempty"`

	// HTTP timeout in milliseconds
	Timeout int `json:"timeout,omitempty"`

	// Skip SSL verification (for self-signed certs)
	SkipSSLVerify bool `json:"skip_ssl_verify,omitempty"`

	// HTTP proxy URL
	Proxy string `json:"proxy,omitempty"`

	// LLM generation defaults (applied when GenerateRequest does not set them)
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`

	// ExtraBody 用于厂商特定字段。各 provider 消费方式不同：
	//   - OpenAI 兼容（含 Dashscope/Ark/SiliconFlow/Kimi-OpenAI 等）：整张 map 平铺到
	//     请求体顶层。例：{"enable_thinking": false} 直接成为顶层字段。
	//   - Claude（含 Kimi-Claude 兼容路径）：同样平铺到 Anthropic Messages 请求顶层。
	//   - Gemini：API 形态没有"顶层平铺"逃生口，**仅消费 thinking_config / thinkingConfig
	//     这一个 key**，桥接进 generationConfig.thinkingConfig；其它 key 会被忽略
	//     （会打 debug 日志提示）。
	// chat 路径直接透传 LLMConfig 的 CustomParams；连接测试 probe 路径上会经
	// NormalizeThinkingParams 叠加"关思考"控制字段。
	ExtraBody map[string]any `json:"extra_body,omitempty"`
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		Provider: ProviderOpenAI,
		Timeout:  60000,
	}
}

// New creates an LLM instance based on the config
func New(cfg *Config) (LLM, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Create HTTP client
	client := createHTTPClient(cfg)

	switch cfg.Provider {
	case ProviderOpenAI, "":
		return NewOpenAI(cfg, client)
	case ProviderClaude:
		return NewClaude(cfg, client)
	case ProviderGemini:
		return NewGemini(cfg, client)
	case ProviderOllama:
		// Ollama uses OpenAI-compatible API
		if cfg.BaseURL == "" {
			cfg.BaseURL = "http://localhost:11434/v1"
		}
		return NewOpenAI(cfg, client)
	case ProviderKimi:
		// Kimi Code uses Anthropic Claude-compatible API
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.kimi.com/coding/v1/messages"
		} else {
			base := strings.TrimRight(cfg.BaseURL, "/")
			if strings.HasSuffix(base, "/v1/messages") {
				cfg.BaseURL = base
			} else if strings.HasSuffix(base, "/v1") {
				cfg.BaseURL = base + "/messages"
			} else {
				cfg.BaseURL = base + "/v1/messages"
			}
		}
		return NewClaude(cfg, client)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", cfg.Provider)
	}
}

// createHTTPClient creates an HTTP client with the given config.
//
// 注意：Client.Timeout 被刻意设为 0。原因是同一个 http.Client 被流式路径
// （GenerateStream）和非流式路径（Generate）共用，Client.Timeout 会覆盖 body 读取阶段
// 长流式响应（LLM 多轮 reasoning 或工具调用）持续几分钟是常态，Timeout 会把 body
// 中途拉断，前端收到截断输出。
//
// 改由两条更精确的限制替代：
//   - Transport.ResponseHeaderTimeout = cfg.Timeout —— 发请求到首个 header 返回的上限；
//     真正 hung 住的连接会被这里拦住，而正在持续吐字的流不会。
//   - 调用方 ctx（context.WithTimeout）—— 总时长上限由 Agent.Run 的 a.cfg.Timeout 负责。
func createHTTPClient(cfg *Config) *http.Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultHTTPTimeout
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.SkipSSLVerify,
		},
		ResponseHeaderTimeout: time.Duration(timeout) * time.Millisecond,
		IdleConnTimeout:       90 * time.Second,
	}

	if cfg.Proxy != "" {
		if proxyURL, err := url.Parse(cfg.Proxy); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &http.Client{
		Timeout:   0, // 见函数顶部注释：流式路径禁用总时长封顶，由 ctx + ResponseHeaderTimeout 负责
		Transport: transport,
	}
}

// Helper function to convert internal messages to provider-specific format
func ConvertMessages(messages []Message) []Message {
	result := make([]Message, len(messages))
	copy(result, messages)
	return result
}
