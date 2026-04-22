package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/toolkits/pkg/logger"
)

// connectStdio 通过 stdio 连接（使用官方 SDK）
func (c *Client) connectStdio(ctx context.Context) error {
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
	transport := &sdkmcp.CommandTransport{
		Command: cmd,
	}

	// 创建 MCP 客户端
	c.client = sdkmcp.NewClient(
		&sdkmcp.Implementation{
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

// callToolStdio 通过 stdio 调用工具（使用官方 SDK）
func (c *Client) callToolStdio(ctx context.Context, name string, arguments map[string]interface{}) (*ToolsCallResult, error) {
	if c.session == nil {
		return nil, fmt.Errorf("MCP session not initialized")
	}

	// 调用工具
	result, err := c.session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("tool call failed: %v", err)
	}

	// 转换结果
	mcpResult := &ToolsCallResult{
		IsError: result.IsError,
	}

	for _, content := range result.Content {
		mc := Content{}

		// 根据具体类型提取内容
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
