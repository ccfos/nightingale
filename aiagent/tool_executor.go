package aiagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"text/template"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/toolkits/pkg/logger"
)

// executeToolI 执行工具并透出人在环中断：内置工具返回
// *ToolInterrupt 时原样上交给调用方（chat 的两个主循环据此停轮、交路由层持久化
// Pending）。tools 为本次 Run 的可见工具表（runCtx.tools 快照），不再从 a.cfg.Tools 读取。
func (a *Agent) executeToolI(ctx context.Context, toolName string, input string, req *AgentRequest, tools []AgentTool) (string, *ToolInterrupt) {
	// 1. 优先检查并执行内置工具
	if result, handled, err := ExecuteBuiltinTool(ctx, a.toolDeps, toolName, req.Params, input); handled {
		if err != nil {
			var ti *ToolInterrupt
			if errors.As(err, &ti) {
				return "", ti
			}
			return fmt.Sprintf("Error: %v", err), nil
		}
		return result, nil
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
		return fmt.Sprintf("Error: tool '%s' not found. Available tools: %v. Please use one of these exact tool names.", toolName, available), nil
	}

	// 解析输入参数
	var args map[string]interface{}
	if input != "" {
		if err := json.Unmarshal([]byte(input), &args); err != nil {
			args = map[string]interface{}{"input": input}
		}
	}

	// 3. 根据工具类型执行（HTTP/外部工具源/外部处理器不支持中断——中断是内置工具专属能力）
	switch tool.Type {
	case ToolTypeHTTP:
		return a.executeHTTPTool(ctx, tool, args, req), nil
	case ToolTypeExternal:
		return a.executeExternalTool(ctx, tool, args), nil
	case ToolTypeProcessor, ToolTypeSkill:
		// 委托给外部工具处理器（由适配层注入）
		if a.externalToolHandler != nil {
			result, err := a.externalToolHandler(ctx, tool, args, req)
			if err != nil {
				return fmt.Sprintf("Error: %v", err), nil
			}
			return result, nil
		}
		return fmt.Sprintf("Error: tool type '%s' requires ExternalToolHandler (not configured)", tool.Type), nil
	default:
		return fmt.Sprintf("Error: unsupported tool type '%s'", tool.Type), nil
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

// executeExternalTool 执行外部工具源提供的工具
func (a *Agent) executeExternalTool(ctx context.Context, tool *AgentTool, args map[string]interface{}) string {
	if tool.source == nil {
		return fmt.Sprintf("Error: external tool '%s' has no source bound", tool.Name)
	}

	result, err := tool.source.CallTool(ctx, tool, args)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	if len(result) > ToolOutputMaxBytes {
		result = result[:ToolOutputMaxBytes] + "\n... (truncated)"
	}
	return result
}

// appendSourceTools 逐源发现外部工具并追加到 base：工具统一置为 ToolTypeExternal
// 并绑定回产出它的源；Name 与已有工具冲突时先到先得。单源失败只降级跳过该源。
// 纯函数：不写 a.cfg，返回新切片（供 runCtx 使用）
func (a *Agent) appendSourceTools(ctx context.Context, base []AgentTool) []AgentTool {
	seen := make(map[string]bool, len(base))
	for _, tool := range base {
		seen[tool.Name] = true
	}
	result := base

	for _, source := range a.cfg.ToolSources {
		tools, err := source.DiscoverTools(ctx)
		if err != nil {
			logger.Warningf("Failed to discover tools from external source: %v", err)
			continue
		}

		added := 0
		for _, tool := range tools {
			if seen[tool.Name] {
				continue
			}
			tool.Type = ToolTypeExternal
			tool.source = source
			result = append(result, tool)
			seen[tool.Name] = true
			added++
		}
		logger.Infof("Discovered %d tools from external source (added=%d)", len(tools), added)
	}

	return result
}
