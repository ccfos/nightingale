package mcp

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/toolkits/pkg/logger"
)

// Client MCP 客户端（基于官方 go-sdk）。stdio 与 HTTP(Streamable) 传输统一走
// SDK 的 ClientSession，因此 ListTools/CallTool 两条传输共用一套代码。
type Client struct {
	config *ServerConfig

	client  *sdkmcp.Client
	session *sdkmcp.ClientSession

	mu          sync.Mutex
	initialized bool
	tools       []Tool // 缓存的工具列表
}

// expandEnvVars 展开字符串中的环境变量引用
func expandEnvVars(s string) string {
	return os.ExpandEnv(s)
}

// NewClient 创建 MCP 客户端
func NewClient(config *ServerConfig) (*Client, error) {
	return &Client{config: config}, nil
}

// Connect 连接到 MCP 服务器
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil
	}

	var err error
	switch c.config.Transport {
	case MCPTransportStdio:
		err = c.connectStdio(ctx)
	default:
		// sse / http / 空值 —— 一律走 SDK 的 Streamable HTTP 客户端
		err = c.connectHTTP(ctx)
	}
	if err != nil {
		return err
	}

	c.initialized = true
	return nil
}

// connectHTTP 通过 SDK 的 StreamableClientTransport 连接（HTTP Streamable）。
func (c *Client) connectHTTP(ctx context.Context) error {
	if c.config.URL == "" {
		return fmt.Errorf("http transport requires URL")
	}

	hc := c.buildHTTPClient()
	transport := &sdkmcp.StreamableClientTransport{
		Endpoint:   c.config.URL,
		HTTPClient: hc,
		// 只做请求-响应，不维持长连接 SSE 流（避免 http.Client.Timeout 打断）。
		DisableStandaloneSSE: true,
	}
	if c.config.AuthMode == MCPAuthOAuth && c.config.OAuth != nil {
		transport.OAuthHandler = NewOAuthHandler(c.config.OAuth, hc)
	}

	c.client = sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "nightingale-aiagent", Version: "1.0.0"},
		nil,
	)
	session, err := c.client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("failed to connect MCP client: %v", err)
	}
	c.session = session

	logger.Infof("MCP http server connected: %s", c.config.Name)
	return nil
}

// buildHTTPClient 构造 HTTP 客户端。none/header 模式把静态头注入 RoundTripper；
// oauth 模式不注入（由 SDK 通过 OAuthHandler 设置 Authorization）。
func (c *Client) buildHTTPClient() *http.Client {
	base := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: c.config.SkipSSLVerify},
	}
	timeout := c.config.Timeout
	if timeout <= 0 {
		timeout = DefaultMCPTimeout
	}

	var rt http.RoundTripper = base
	if c.config.AuthMode != MCPAuthOAuth {
		if hdrs := c.staticHeaders(); len(hdrs) > 0 {
			rt = &headerRoundTripper{base: base, headers: hdrs}
		}
	}
	return &http.Client{
		Timeout:   time.Duration(timeout) * time.Millisecond,
		Transport: rt,
	}
}

// staticHeaders 合并自定义请求头与便捷鉴权（bearer/api_key/basic），值支持 ${VAR}。
func (c *Client) staticHeaders() map[string]string {
	cfg := c.config
	out := make(map[string]string, len(cfg.Headers)+1)
	for k, v := range cfg.Headers {
		out[k] = expandEnvVars(v)
	}

	apiKey := expandEnvVars(cfg.APIKey)
	username := expandEnvVars(cfg.Username)
	password := expandEnvVars(cfg.Password)
	switch strings.ToLower(cfg.AuthType) {
	case "bearer":
		if apiKey != "" {
			out["Authorization"] = "Bearer " + apiKey
		}
	case "api_key", "apikey":
		if apiKey != "" {
			out["X-API-Key"] = apiKey
		}
	case "basic":
		if username != "" {
			out["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
		}
	default:
		if apiKey != "" {
			out["Authorization"] = "Bearer " + apiKey
		}
	}
	return out
}

// headerRoundTripper 在每个请求上注入静态头。
type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}
	return h.base.RoundTrip(req)
}

// ListTools 获取工具列表（stdio / http 共用 SDK session）
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	c.mu.Lock()
	if len(c.tools) > 0 {
		tools := c.tools
		c.mu.Unlock()
		return tools, nil
	}
	c.mu.Unlock()

	if c.session == nil {
		return nil, fmt.Errorf("MCP session not initialized")
	}

	result, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %v", err)
	}

	var tools []Tool
	for _, tool := range result.Tools {
		inputSchema := make(map[string]interface{})
		if tool.InputSchema != nil {
			if schemaBytes, err := json.Marshal(tool.InputSchema); err == nil {
				json.Unmarshal(schemaBytes, &inputSchema)
			}
		}
		tools = append(tools, Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: inputSchema,
		})
	}

	c.mu.Lock()
	c.tools = tools
	c.mu.Unlock()
	return tools, nil
}

// CallTool 调用工具（stdio / http 共用 SDK session）
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*ToolsCallResult, error) {
	if c.session == nil {
		return nil, fmt.Errorf("MCP session not initialized")
	}

	result, err := c.session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("tool call failed: %v", err)
	}

	mcpResult := &ToolsCallResult{IsError: result.IsError}
	for _, content := range result.Content {
		mcpResult.Content = append(mcpResult.Content, convertContent(content))
	}
	return mcpResult, nil
}

// convertContent 把 SDK 的 Content 转成内部 Content 表示。
func convertContent(content sdkmcp.Content) Content {
	mc := Content{}
	switch c := content.(type) {
	case *sdkmcp.TextContent:
		mc.Type = "text"
		mc.Text = c.Text
	case *sdkmcp.ImageContent:
		mc.Type = "image"
		mc.Data = string(c.Data)
		mc.MimeType = c.MIMEType
	case *sdkmcp.AudioContent:
		mc.Type = "audio"
		mc.Data = string(c.Data)
		mc.MimeType = c.MIMEType
	case *sdkmcp.EmbeddedResource:
		mc.Type = "resource"
		if c.Resource != nil {
			if c.Resource.Text != "" {
				mc.Text = c.Resource.Text
			} else if c.Resource.Blob != nil {
				mc.Data = string(c.Resource.Blob)
			}
			mc.MimeType = c.Resource.MIMEType
		}
	case *sdkmcp.ResourceLink:
		mc.Type = "resource_link"
		mc.Text = c.URI
	default:
		if data, err := json.Marshal(content); err == nil {
			mc.Type = "unknown"
			mc.Text = string(data)
		}
	}
	return mc
}

// Close 关闭连接
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.session != nil {
		c.session.Close()
		c.session = nil
	}
	c.client = nil
	c.initialized = false
	logger.Infof("MCP client closed: %s", c.config.Name)
	return nil
}

// ListToolsForConfig 是一次性连通性/工具探测：建客户端 → 连接 → 列工具 → 关闭。
// 用于 HTTP 处理器的「测试连接」和工具预览，替代旧的 ListToolsHTTP。
func ListToolsForConfig(ctx context.Context, config *ServerConfig) ([]Tool, error) {
	client, err := NewClient(config)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	if err := client.Connect(ctx); err != nil {
		return nil, err
	}
	return client.ListTools(ctx)
}
