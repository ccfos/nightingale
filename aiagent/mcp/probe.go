package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ListToolsHTTP 通过 HTTP JSON-RPC 协议探测 MCP 服务器的工具列表。
// 用于 HTTP 处理器做连通性测试、在 DB 中预览 MCP 工具列表等场景。
func ListToolsHTTP(serverURL string, headers map[string]string) ([]Tool, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	// Step 1: Initialize
	initResp, initSessionID, err := sendRPC(client, serverURL, headers, "", 1, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "nightingale", "version": "1.0.0"},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize: %v", err)
	}
	_ = initResp

	// Send initialized notification
	sendRPC(client, serverURL, headers, initSessionID, 0, "notifications/initialized", map[string]interface{}{})

	// Step 2: List tools
	toolsResp, _, err := sendRPC(client, serverURL, headers, initSessionID, 2, "tools/list", map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("tools/list: %v", err)
	}

	if toolsResp == nil || toolsResp.Result == nil {
		return []Tool{}, nil
	}

	toolsRaw, ok := toolsResp.Result["tools"]
	if !ok {
		return []Tool{}, nil
	}

	toolsJSON, _ := json.Marshal(toolsRaw)
	var tools []Tool
	json.Unmarshal(toolsJSON, &tools)
	return tools, nil
}

// sendRPC 发送一个 JSON-RPC 请求并解析响应（HTTP/SSE 混合响应）
func sendRPC(client *http.Client, serverURL string, hdrs map[string]string, sessionID string, id int, method string, params interface{}) (*jsonRPCResponse, string, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	if id > 0 {
		body["id"] = id
	}

	reqBody, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", serverURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	newSessionID := resp.Header.Get("Mcp-Session-Id")
	if newSessionID == "" {
		newSessionID = sessionID
	}

	// Notification (no id) - no response body expected
	if id <= 0 {
		return nil, newSessionID, nil
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		if len(respBody) > 500 {
			respBody = respBody[:500]
		}
		return nil, newSessionID, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, newSessionID, err
	}

	// Handle SSE response
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") {
		for _, line := range strings.Split(string(respBody), "\n") {
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				var rpcResp jsonRPCResponse
				if json.Unmarshal([]byte(data), &rpcResp) == nil && (rpcResp.Result != nil || rpcResp.Error != nil) {
					if rpcResp.Error != nil {
						return &rpcResp, newSessionID, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
					}
					return &rpcResp, newSessionID, nil
				}
			}
		}
		return nil, newSessionID, fmt.Errorf("no valid JSON-RPC response in SSE stream")
	}

	// Handle JSON response
	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		if len(respBody) > 200 {
			respBody = respBody[:200]
		}
		return nil, newSessionID, fmt.Errorf("invalid response: %s", string(respBody))
	}

	if rpcResp.Error != nil {
		return &rpcResp, newSessionID, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return &rpcResp, newSessionID, nil
}
