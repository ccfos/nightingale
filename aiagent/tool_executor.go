package aiagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/mcp"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/toolkits/pkg/logger"
)

// executeTool 执行工具
// tools 为本次 Run 的可见工具表（runCtx.tools 快照），不再从 a.cfg.Tools 读取
func (a *Agent) executeTool(ctx context.Context, toolName string, input string, req *AgentRequest, tools []AgentTool) string {
	// 1. 优先检查并执行内置工具
	if result, handled, err := ExecuteBuiltinTool(ctx, a.toolDeps, toolName, req.Params, input); handled {
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return result
	}

	// 2. 查找配置的工具定义
	var tool *AgentTool
	for i := range tools {
		if tools[i].Name == toolName {
			tool = &tools[i]
			break
		}
	}

	if tool == nil {
		available := make([]string, len(tools))
		for i, t := range tools {
			available[i] = t.Name
		}
		return fmt.Sprintf("Error: tool '%s' not found. Available tools: %v. Please use one of these exact tool names.", toolName, available)
	}

	// 解析输入参数
	var args map[string]interface{}
	if input != "" {
		if err := json.Unmarshal([]byte(input), &args); err != nil {
			args = map[string]interface{}{"input": input}
		}
	}

	// 3. 根据工具类型执行
	switch tool.Type {
	case ToolTypeHTTP:
		return a.executeHTTPTool(ctx, tool, args, req)
	case ToolTypeMCP:
		return a.executeMCPTool(ctx, tool, args)
	case ToolTypeProcessor, ToolTypeSkill:
		// 委托给外部工具处理器（由适配层注入）
		if a.externalToolHandler != nil {
			result, err := a.externalToolHandler(ctx, tool, args, req)
			if err != nil {
				return fmt.Sprintf("Error: %v", err)
			}
			return result
		}
		return fmt.Sprintf("Error: tool type '%s' requires ExternalToolHandler (not configured)", tool.Type)
	default:
		return fmt.Sprintf("Error: unsupported tool type '%s'", tool.Type)
	}
}

// executeHTTPTool 执行 HTTP 工具
func (a *Agent) executeHTTPTool(ctx context.Context, tool *AgentTool, args map[string]interface{}, req *AgentRequest) string {
	if tool.URL == "" {
		return "Error: tool URL not configured"
	}

	method := tool.Method
	if method == "" {
		method = "GET"
	}

	// 构建请求体
	var bodyReader io.Reader
	if tool.BodyTemplate != "" {
		templateData := map[string]interface{}{
			"args":   args,
			"params": req.Params,
			"vars":   req.Vars,
		}
		// 合并 adapter 注入的兼容字段（event, inputs 等），保证旧模板继续工作
		for k, v := range req.TemplateExtra {
			templateData[k] = v
		}

		t, err := template.New("tool_body").Funcs(template.FuncMap(tplx.TemplateFuncMap)).Parse(tool.BodyTemplate)
		if err != nil {
			return fmt.Sprintf("Error: failed to parse body template: %v", err)
		}

		var body bytes.Buffer
		if err = t.Execute(&body, templateData); err != nil {
			return fmt.Sprintf("Error: failed to execute body template: %v", err)
		}
		bodyReader = &body
	} else if method == "POST" || method == "PUT" {
		jsonBody, _ := json.Marshal(args)
		bodyReader = bytes.NewBuffer(jsonBody)
	}

	// 创建 HTTP 客户端
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: tool.SkipSSLVerify},
	}

	timeout := tool.Timeout
	if timeout <= 0 {
		timeout = 30000
	}

	client := &http.Client{
		Timeout:   time.Duration(timeout) * time.Millisecond,
		Transport: transport,
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, tool.URL, bodyReader)
	if err != nil {
		return fmt.Sprintf("Error: failed to create request: %v", err)
	}

	if bodyReader != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	for k, v := range tool.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Sprintf("Error: HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("Error: failed to read response: %v", err)
	}

	if resp.StatusCode > HTTPStatusSuccessMax {
		return fmt.Sprintf("HTTP error (status %d): %s", resp.StatusCode, string(body))
	}

	result := string(body)
	if len(result) > ToolOutputMaxBytes {
		result = result[:ToolOutputMaxBytes] + "\n... (truncated)"
	}

	return result
}

// executeMCPTool 执行 MCP 工具
func (a *Agent) executeMCPTool(ctx context.Context, tool *AgentTool, args map[string]interface{}) string {
	if tool.MCPConfig == nil {
		return "Error: mcp_config not configured"
	}

	if a.mcpClientManager == nil {
		return "Error: MCP client manager not initialized"
	}

	serverConfig, ok := a.mcpServers[tool.MCPConfig.ServerName]
	if !ok {
		return fmt.Sprintf("Error: MCP server '%s' not found", tool.MCPConfig.ServerName)
	}

	client, err := a.mcpClientManager.GetOrCreateClient(ctx, serverConfig)
	if err != nil {
		return fmt.Sprintf("Error: failed to connect to MCP server '%s': %v", tool.MCPConfig.ServerName, err)
	}

	result, err := client.CallTool(ctx, tool.MCPConfig.ToolName, args)
	if err != nil {
		return fmt.Sprintf("Error: MCP tool call failed: %v", err)
	}

	return a.formatMCPResult(result)
}

// appendMCPTools 自动发现 MCP 服务器提供的工具并追加到 base
// 纯函数：不写 a.cfg，返回新切片（供 runCtx 使用）
func (a *Agent) appendMCPTools(ctx context.Context, base []AgentTool) []AgentTool {
	if a.mcpClientManager == nil || len(a.mcpServers) == 0 {
		return base
	}

	seen := make(map[string]bool, len(base))
	for _, tool := range base {
		seen[tool.Name] = true
	}
	result := base

	for serverName, serverConfig := range a.mcpServers {
		client, err := a.mcpClientManager.GetOrCreateClient(ctx, serverConfig)
		if err != nil {
			logger.Warningf("Failed to connect to MCP server '%s': %v", serverName, err)
			continue
		}

		tools, err := client.ListTools(ctx)
		if err != nil {
			logger.Warningf("Failed to list tools from MCP server '%s': %v", serverName, err)
			continue
		}

		for _, mcpTool := range tools {
			if seen[mcpTool.Name] {
				continue
			}

			var params []ToolParameter
			if mcpTool.InputSchema != nil {
				params = a.convertMCPSchemaToParams(mcpTool.InputSchema)
			}

			result = append(result, AgentTool{
				Name:        mcpTool.Name,
				Description: mcpTool.Description,
				Type:        ToolTypeMCP,
				MCPConfig: &mcp.ToolConfig{
					ServerName: serverName,
					ToolName:   mcpTool.Name,
				},
				Parameters: params,
			})
			seen[mcpTool.Name] = true

			logger.Debugf("Discovered MCP tool: %s (from server: %s)", mcpTool.Name, serverName)
		}

		logger.Infof("Discovered %d tools from MCP server '%s'", len(tools), serverName)
	}

	return result
}

func (a *Agent) convertMCPSchemaToParams(schema map[string]interface{}) []ToolParameter {
	var params []ToolParameter

	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return params
	}

	requiredFields := make(map[string]bool)
	if required, ok := schema["required"].([]interface{}); ok {
		for _, r := range required {
			if name, ok := r.(string); ok {
				requiredFields[name] = true
			}
		}
	}

	for name, propRaw := range properties {
		prop, ok := propRaw.(map[string]interface{})
		if !ok {
			continue
		}

		param := ToolParameter{
			Name:     name,
			Required: requiredFields[name],
		}

		if t, ok := prop["type"].(string); ok {
			param.Type = t
		}
		if desc, ok := prop["description"].(string); ok {
			param.Description = desc
		}

		params = append(params, param)
	}

	return params
}

func (a *Agent) formatMCPResult(result *mcp.ToolsCallResult) string {
	if result == nil {
		return "No result returned"
	}

	if result.IsError {
		var errMsg strings.Builder
		errMsg.WriteString("Error: ")
		for _, content := range result.Content {
			if content.Text != "" {
				errMsg.WriteString(content.Text)
			}
		}
		return errMsg.String()
	}

	var output strings.Builder
	for _, content := range result.Content {
		switch content.Type {
		case "text":
			output.WriteString(content.Text)
		case "image":
			output.WriteString(fmt.Sprintf("[Image: %s]", content.MimeType))
		case "resource":
			output.WriteString(fmt.Sprintf("[Resource: %s]", content.MimeType))
		default:
			if content.Text != "" {
				output.WriteString(content.Text)
			}
		}
		output.WriteString("\n")
	}

	outputStr := strings.TrimSpace(output.String())
	if len(outputStr) > ToolOutputMaxBytes {
		outputStr = outputStr[:ToolOutputMaxBytes] + "\n... (truncated)"
	}

	return outputStr
}
