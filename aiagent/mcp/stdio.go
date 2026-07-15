package mcp

import (
	"context"
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
