package llm

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNew_SelectsProviderAndNormalizesEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		config   Config
		provider string
		endpoint string
	}{
		{name: "default", provider: ProviderOpenAI, endpoint: DefaultOpenAIURL},
		{name: "claude", config: Config{Provider: ProviderClaude}, provider: ProviderClaude, endpoint: DefaultClaudeURL},
		{
			name: "gemini", config: Config{Provider: ProviderGemini},
			provider: ProviderGemini, endpoint: DefaultGeminiURL,
		},
		{
			name: "ollama", config: Config{Provider: ProviderOllama},
			provider: ProviderOpenAI, endpoint: "http://localhost:11434/v1/chat/completions",
		},
		{
			name: "kimi default", config: Config{Provider: ProviderKimi},
			provider: ProviderClaude, endpoint: "https://api.kimi.com/coding/v1/messages",
		},
		{
			name: "kimi root", config: Config{Provider: ProviderKimi, BaseURL: "http://127.0.0.1:1/gateway/"},
			provider: ProviderClaude, endpoint: "http://127.0.0.1:1/gateway/v1/messages",
		},
		{
			name: "kimi version", config: Config{Provider: ProviderKimi, BaseURL: "http://127.0.0.1:1/v1/"},
			provider: ProviderClaude, endpoint: "http://127.0.0.1:1/v1/messages",
		},
		{
			name: "kimi full endpoint", config: Config{Provider: ProviderKimi, BaseURL: "http://127.0.0.1:1/v1/messages/"},
			provider: ProviderClaude, endpoint: "http://127.0.0.1:1/v1/messages",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := New(&tc.config)

			require.NoError(t, err)
			require.Equal(t, tc.provider, client.Name())
			require.Equal(t, tc.endpoint, tc.config.BaseURL)
		})
	}
}

func TestNew_NilConfigUsesDefaults(t *testing.T) {
	client, err := New(nil)

	require.NoError(t, err)
	openai, ok := client.(*OpenAI)
	require.True(t, ok)
	require.Equal(t, ProviderOpenAI, openai.Name())
	require.Equal(t, 60000, openai.config.Timeout)
	require.Equal(t, DefaultOpenAIURL, openai.config.BaseURL)
}

func TestNew_RejectsUnsupportedProvider(t *testing.T) {
	client, err := New(&Config{Provider: "unknown"})

	require.Nil(t, client)
	require.EqualError(t, err, "unsupported LLM provider: unknown")
}

func TestCreateHTTPClient_ConfiguresHeaderTimeoutAndProxy(t *testing.T) {
	cases := []struct {
		name    string
		config  Config
		timeout time.Duration
		proxy   string
	}{
		{name: "default", timeout: time.Duration(DefaultHTTPTimeout) * time.Millisecond},
		{
			name: "custom", config: Config{Timeout: 1234, SkipSSLVerify: true, Proxy: "http://127.0.0.1:8123"},
			timeout: 1234 * time.Millisecond, proxy: "http://127.0.0.1:8123",
		},
		{
			name: "invalid proxy", config: Config{Proxy: "://invalid"},
			timeout: time.Duration(DefaultHTTPTimeout) * time.Millisecond,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := createHTTPClient(&tc.config)

			// Streaming bodies must remain governed by context, not a client-wide timeout.
			require.Zero(t, client.Timeout)
			transport := client.Transport.(*http.Transport)
			require.Equal(t, tc.timeout, transport.ResponseHeaderTimeout)
			require.Equal(t, tc.config.SkipSSLVerify, transport.TLSClientConfig.InsecureSkipVerify)
			if tc.proxy == "" {
				require.Nil(t, transport.Proxy)
			} else {
				proxy, err := transport.Proxy(&http.Request{})
				require.NoError(t, err)
				require.Equal(t, tc.proxy, proxy.String())
			}
		})
	}
}

func TestClientCache_ReusesEquivalentConfig(t *testing.T) {
	cache := &ClientCache{}
	temperature, maxTokens := 0.5, 128
	first := Config{
		Provider: ProviderOpenAI, BaseURL: "http://127.0.0.1:1/v1/chat/completions", Model: "fixture",
		Temperature: &temperature, MaxTokens: &maxTokens,
		Headers:   map[string]string{"X-Tenant": "ops", "X-Route": "local"},
		ExtraBody: map[string]any{"seed": 42, "options": map[string]any{"a": 1, "b": 2}},
	}
	second := first
	second.Headers = map[string]string{"X-Route": "local", "X-Tenant": "ops"}
	second.ExtraBody = map[string]any{"options": map[string]any{"b": 2, "a": 1}, "seed": 42}

	one, err := cache.GetOrCreate(&first)
	require.NoError(t, err)
	two, err := cache.GetOrCreate(&second)

	require.NoError(t, err)
	require.Same(t, one, two)
}

func TestClientCache_SeparatesRequestAffectingConfig(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Config)
	}{
		{name: "provider", change: func(c *Config) { c.Provider = ProviderClaude }},
		{name: "endpoint", change: func(c *Config) { c.BaseURL += "/other" }},
		{name: "key", change: func(c *Config) { c.APIKey = "different" }},
		{name: "model", change: func(c *Config) { c.Model = "different" }},
		{name: "TLS", change: func(c *Config) { c.SkipSSLVerify = true }},
		{name: "proxy", change: func(c *Config) { c.Proxy = "http://127.0.0.1:8123" }},
		{name: "timeout", change: func(c *Config) { c.Timeout = 123 }},
		{name: "temperature", change: func(c *Config) { value := 0.7; c.Temperature = &value }},
		{name: "token limit", change: func(c *Config) { value := 99; c.MaxTokens = &value }},
		{name: "headers", change: func(c *Config) { c.Headers = map[string]string{"X-Tenant": "other"} }},
		{name: "extra body", change: func(c *Config) { c.ExtraBody = map[string]any{"seed": 43} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cache := &ClientCache{}
			base := Config{Provider: ProviderOpenAI, BaseURL: "http://127.0.0.1:1/v1/chat/completions"}
			changed := base
			tc.change(&changed)
			first, err := cache.GetOrCreate(&base)
			require.NoError(t, err)

			second, err := cache.GetOrCreate(&changed)

			require.NoError(t, err)
			require.NotSame(t, first, second)
		})
	}
}

func TestClientCache_DoesNotCacheConstructionErrors(t *testing.T) {
	cache := &ClientCache{}
	cfg := &Config{Provider: "unknown"}

	client, err := cache.GetOrCreate(cfg)

	require.Nil(t, client)
	require.ErrorContains(t, err, "unsupported LLM provider")
	_, cached := cache.clients.Load(cache.fingerprint(cfg))
	require.False(t, cached)
}
