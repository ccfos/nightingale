// Package llm provides a unified interface for multiple LLM providers.
// Supports OpenAI-compatible APIs, Claude/Anthropic, and Gemini.
package llm

import (
	"context"
	"crypto/tls"
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
	ProviderKimi    = "kimi"   // Kimi Code (OpenAI-compatible)
)

// Role constants
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolCall represents a tool/function call from the LLM
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
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
	Stop        []string         `json:"stop,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
}

// GenerateResponse is the unified response from LLM generation
type GenerateResponse struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason"`
	Usage        *Usage     `json:"usage,omitempty"`
}

// Usage represents token usage statistics
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamChunk represents a chunk in streaming response
type StreamChunk struct {
	Content      string     `json:"content,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
	Done         bool       `json:"done"`
	Error        error      `json:"error,omitempty"`
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
