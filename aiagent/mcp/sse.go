package mcp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
)

// connectSSE 通过 SSE 连接（保留自定义实现，SDK 暂不支持 SSE 客户端）
func (c *Client) connectSSE(ctx context.Context) error {
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

// listToolsSSE 通过 SSE 获取工具列表
func (c *Client) listToolsSSE(ctx context.Context) ([]Tool, error) {
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
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse tools list: %v", err)
	}

	return result.Tools, nil
}

// callToolSSE 通过 SSE 调用工具
func (c *Client) callToolSSE(ctx context.Context, name string, arguments map[string]interface{}) (*ToolsCallResult, error) {
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
	var result ToolsCallResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse tool call result: %v", err)
	}

	return &result, nil
}

// setAuthHeaders 设置鉴权请求头
func (c *Client) setAuthHeaders(req *http.Request) {
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
func (c *Client) sendSSERequest(ctx context.Context, req map[string]interface{}) (map[string]interface{}, error) {
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
