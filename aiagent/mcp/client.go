package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/toolkits/pkg/logger"
)

// Client MCP 客户端（基于官方 go-sdk）
type Client struct {
	config *ServerConfig

	// SDK 客户端和会话（stdio 传输）
	client  *sdkmcp.Client
	session *sdkmcp.ClientSession

	// SSE 传输（SDK 暂不支持 SSE 客户端，保留自定义实现）
	httpClient *http.Client
	sseURL     string

	// 通用
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
	client := &Client{
		config: config,
	}

	return client, nil
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

// ListTools 获取工具列表
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	c.mu.Lock()
	if len(c.tools) > 0 {
		tools := c.tools
		c.mu.Unlock()
		return tools, nil
	}
	c.mu.Unlock()

	var tools []Tool

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

			tools = append(tools, Tool{
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

// CallTool 调用工具
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*ToolsCallResult, error) {
	switch c.config.Transport {
	case MCPTransportStdio:
		return c.callToolStdio(ctx, name, arguments)
	case MCPTransportSSE:
		return c.callToolSSE(ctx, name, arguments)
	default:
		return nil, fmt.Errorf("unsupported transport: %s", c.config.Transport)
	}
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
