package aiagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/toolkits/pkg/logger"
)

const (
	// MCP 传输类型
	MCPTransportStdio = "stdio" // 标准输入/输出传输
	MCPTransportSSE   = "sse"   // HTTP Server-Sent Events 传输

	// 默认超时
	DefaultMCPTimeout        = 30000 // 30 秒
	DefaultMCPConnectTimeout = 10000 // 10 秒
)

// MCPConfig MCP 服务器配置（在 AIAgentConfig 中使用）
type MCPConfig struct {
	// MCP 服务器列表
	Servers []MCPServerConfig `json:"servers"`
}

// MCPServerConfig 单个 MCP 服务器配置
type MCPServerConfig struct {
	// 服务器名称（唯一标识）
	Name string `json:"name"`

	// 传输类型：stdio 或 sse
	Transport string `json:"transport"`

	// === stdio 传输配置 ===
	Command string            `json:"command,omitempty"` // 启动命令
	Args    []string          `json:"args,omitempty"`    // 命令参数
	Env     map[string]string `json:"env,omitempty"`     // 环境变量（支持 ${VAR} 从系统环境变量读取）

	// === SSE 传输配置 ===
	URL           string            `json:"url,omitempty"`             // SSE 服务器 URL
	Headers       map[string]string `json:"headers,omitempty"`         // 请求头（支持 ${VAR} 从系统环境变量读取）
	SkipSSLVerify bool              `json:"skip_ssl_verify,omitempty"` // 跳过 SSL 验证

	// === 鉴权配置（SSE 传输）===
	// 便捷鉴权配置，会自动设置对应的 Header
	AuthType string `json:"auth_type,omitempty"` // 鉴权类型：bearer, api_key, basic
	APIKey   string `json:"api_key,omitempty"`   // API Key（支持 ${VAR} 从系统环境变量读取）
	Username string `json:"username,omitempty"`  // Basic Auth 用户名
	Password string `json:"password,omitempty"`  // Basic Auth 密码（支持 ${VAR}）

	// 通用配置
	Timeout        int `json:"timeout,omitempty"`         // 工具调用超时（毫秒）
	ConnectTimeout int `json:"connect_timeout,omitempty"` // 连接超时（毫秒）
}

// MCPToolConfig MCP 工具配置（在 AgentTool 中使用）
type MCPToolConfig struct {
	// MCP 服务器名称（引用 MCPConfig.Servers 中的配置）
	ServerName string `json:"server_name"`

	// 工具名称（MCP 服务器返回的工具名）
	ToolName string `json:"tool_name"`
}

// MCPTool MCP 工具定义（用于内部表示）
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// MCPToolsCallResult 工具调用结果
type MCPToolsCallResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// MCPContent 工具返回内容
type MCPContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// MCPClient MCP 客户端（基于官方 go-sdk）
type MCPClient struct {
	config *MCPServerConfig

	// SDK 客户端和会话（stdio 传输）
	client  *mcp.Client
	session *mcp.ClientSession

	// SSE 传输（SDK 暂不支持 SSE 客户端，保留自定义实现）
	httpClient *http.Client
	sseURL     string

	// 通用
	mu          sync.Mutex
	initialized bool
	tools       []MCPTool // 缓存的工具列表
}

// expandEnvVars 展开字符串中的环境变量引用
func expandEnvVars(s string) string {
	return os.ExpandEnv(s)
}

// NewMCPClient 创建 MCP 客户端
func NewMCPClient(config *MCPServerConfig) (*MCPClient, error) {
	client := &MCPClient{
		config: config,
	}

	return client, nil
}

// Connect 连接到 MCP 服务器
func (c *MCPClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil
	}

	var err error
	switch c.config.Transport {
	case MCPTransportStdio:
		err = c.connectStdio(ctx)
	case MCPTransportSSE:
		err = c.connectSSE(ctx)
	default:
		return fmt.Errorf("unsupported MCP transport: %s", c.config.Transport)
	}

	if err != nil {
		return err
	}

	c.initialized = true
	return nil
}

// connectStdio 通过 stdio 连接（使用官方 SDK）
func (c *MCPClient) connectStdio(ctx context.Context) error {
	if c.config.Command == "" {
		return fmt.Errorf("stdio transport requires command")
	}

	// 准备环境变量
	env := os.Environ()
	for k, v := range c.config.Env {
		expandedValue := expandEnvVars(v)
		env = append(env, fmt.Sprintf("%s=%s", k, expandedValue))
	}

	// 创建 exec.Cmd
	cmd := exec.CommandContext(ctx, c.config.Command, c.config.Args...)
	cmd.Env = env

	// 使用官方 SDK 的 CommandTransport
	transport := &mcp.CommandTransport{
		Command: cmd,
	}

	// 创建 MCP 客户端
	c.client = mcp.NewClient(
		&mcp.Implementation{
			Name:    "nightingale-aiagent",
			Version: "1.0.0",
		},
		nil,
	)

	// 连接并初始化（Connect 会自动进行 initialize 握手）
	session, err := c.client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("failed to connect MCP client: %v", err)
	}
	c.session = session

	logger.Infof("MCP stdio server started: %s", c.config.Name)
	return nil
}

// connectSSE 通过 SSE 连接（保留自定义实现，SDK 暂不支持 SSE 客户端）
func (c *MCPClient) connectSSE(ctx context.Context) error {
	if c.config.URL == "" {
		return fmt.Errorf("SSE transport requires URL")
	}

	// 创建 HTTP 客户端
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: c.config.SkipSSLVerify},
	}

	timeout := c.config.ConnectTimeout
	if timeout <= 0 {
		timeout = DefaultMCPConnectTimeout
	}

	c.httpClient = &http.Client{
		Timeout:   time.Duration(timeout) * time.Millisecond,
		Transport: transport,
	}

	c.sseURL = c.config.URL

	logger.Infof("MCP SSE client configured: %s", c.config.Name)
	return nil
}

// ListTools 获取工具列表
func (c *MCPClient) ListTools(ctx context.Context) ([]MCPTool, error) {
	c.mu.Lock()
	if len(c.tools) > 0 {
		tools := c.tools
		c.mu.Unlock()
		return tools, nil
	}
	c.mu.Unlock()

	var tools []MCPTool

	switch c.config.Transport {
	case MCPTransportStdio:
		// 使用官方 SDK
		if c.session == nil {
			return nil, fmt.Errorf("MCP session not initialized")
		}

		result, err := c.session.ListTools(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list tools: %v", err)
		}

		// 转换为内部格式
		for _, tool := range result.Tools {
			inputSchema := make(map[string]interface{})
			if tool.InputSchema != nil {
				// 将 SDK 的 InputSchema 转换为 map
				schemaBytes, err := json.Marshal(tool.InputSchema)
				if err == nil {
					json.Unmarshal(schemaBytes, &inputSchema)
				}
			}

			tools = append(tools, MCPTool{
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: inputSchema,
			})
		}

	case MCPTransportSSE:
		// 使用自定义 HTTP 实现
		var err error
		tools, err = c.listToolsSSE(ctx)
		if err != nil {
			return nil, err
		}
	}

	c.mu.Lock()
	c.tools = tools
	c.mu.Unlock()

	return tools, nil
}

// listToolsSSE 通过 SSE 获取工具列表
func (c *MCPClient) listToolsSSE(ctx context.Context) ([]MCPTool, error) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	}

	resp, err := c.sendSSERequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resultBytes, _ := json.Marshal(resp["result"])
	var result struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse tools list: %v", err)
	}

	return result.Tools, nil
}

// CallTool 调用工具
func (c *MCPClient) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*MCPToolsCallResult, error) {
	switch c.config.Transport {
	case MCPTransportStdio:
		return c.callToolStdio(ctx, name, arguments)
	case MCPTransportSSE:
		return c.callToolSSE(ctx, name, arguments)
	default:
		return nil, fmt.Errorf("unsupported transport: %s", c.config.Transport)
	}
}

// callToolStdio 通过 stdio 调用工具（使用官方 SDK）
func (c *MCPClient) callToolStdio(ctx context.Context, name string, arguments map[string]interface{}) (*MCPToolsCallResult, error) {
	if c.session == nil {
		return nil, fmt.Errorf("MCP session not initialized")
	}

	// 调用工具
	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("tool call failed: %v", err)
	}

	// 转换结果
	mcpResult := &MCPToolsCallResult{
		IsError: result.IsError,
	}

	for _, content := range result.Content {
		mc := MCPContent{}

		// 根据具体类型提取内容
		switch c := content.(type) {
		case *mcp.TextContent:
			mc.Type = "text"
			mc.Text = c.Text
		case *mcp.ImageContent:
			mc.Type = "image"
			mc.Data = string(c.Data)
			mc.MimeType = c.MIMEType
		case *mcp.AudioContent:
			mc.Type = "audio"
			mc.Data = string(c.Data)
			mc.MimeType = c.MIMEType
		case *mcp.EmbeddedResource:
			mc.Type = "resource"
			if c.Resource != nil {
				if c.Resource.Text != "" {
					mc.Text = c.Resource.Text
				} else if c.Resource.Blob != nil {
					mc.Data = string(c.Resource.Blob)
				}
				mc.MimeType = c.Resource.MIMEType
			}
		case *mcp.ResourceLink:
			mc.Type = "resource_link"
			mc.Text = c.URI
		default:
			// 尝试通过 JSON 序列化获取内容
			if data, err := json.Marshal(content); err == nil {
				mc.Type = "unknown"
				mc.Text = string(data)
			}
		}

		mcpResult.Content = append(mcpResult.Content, mc)
	}

	return mcpResult, nil
}

// callToolSSE 通过 SSE 调用工具
func (c *MCPClient) callToolSSE(ctx context.Context, name string, arguments map[string]interface{}) (*MCPToolsCallResult, error) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      name,
			"arguments": arguments,
		},
	}

	resp, err := c.sendSSERequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if errObj, ok := resp["error"].(map[string]interface{}); ok {
		return nil, fmt.Errorf("MCP error: %v", errObj["message"])
	}

	resultBytes, _ := json.Marshal(resp["result"])
	var result MCPToolsCallResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse tool call result: %v", err)
	}

	return &result, nil
}

// setAuthHeaders 设置鉴权请求头
func (c *MCPClient) setAuthHeaders(req *http.Request) {
	cfg := c.config

	if cfg.AuthType == "" && cfg.APIKey == "" {
		return
	}

	apiKey := expandEnvVars(cfg.APIKey)
	username := expandEnvVars(cfg.Username)
	password := expandEnvVars(cfg.Password)

	switch strings.ToLower(cfg.AuthType) {
	case "bearer":
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	case "api_key", "apikey":
		if apiKey != "" {
			req.Header.Set("X-API-Key", apiKey)
		}
	case "basic":
		if username != "" {
			req.SetBasicAuth(username, password)
		}
	default:
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}
}

// sendSSERequest 通过 HTTP 发送请求
func (c *MCPClient) sendSSERequest(ctx context.Context, req map[string]interface{}) (map[string]interface{}, error) {
	baseURL := c.sseURL
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	postURL := baseURL + "message"
	if _, err := url.Parse(postURL); err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", postURL, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeaders(httpReq)

	for k, v := range c.config.Headers {
		httpReq.Header.Set(k, expandEnvVars(v))
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	return result, nil
}

// Close 关闭连接
func (c *MCPClient) Close() error {
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

// MCPClientManager MCP 客户端管理器
type MCPClientManager struct {
	clients map[string]*MCPClient
	mu      sync.RWMutex
}

// NewMCPClientManager 创建 MCP 客户端管理器
func NewMCPClientManager() *MCPClientManager {
	return &MCPClientManager{
		clients: make(map[string]*MCPClient),
	}
}

// GetOrCreateClient 获取或创建 MCP 客户端
func (m *MCPClientManager) GetOrCreateClient(ctx context.Context, config *MCPServerConfig) (*MCPClient, error) {
	m.mu.RLock()
	client, ok := m.clients[config.Name]
	m.mu.RUnlock()

	if ok {
		return client, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 再次检查（double-check locking）
	if client, ok := m.clients[config.Name]; ok {
		return client, nil
	}

	// 创建新客户端
	client, err := NewMCPClient(config)
	if err != nil {
		return nil, err
	}

	// 连接
	if err := client.Connect(ctx); err != nil {
		return nil, err
	}

	m.clients[config.Name] = client
	return client, nil
}

// CloseAll 关闭所有客户端
func (m *MCPClientManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			logger.Warningf("Failed to close MCP client %s: %v", name, err)
		}
	}
	m.clients = make(map[string]*MCPClient)
}
