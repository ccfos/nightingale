// Package llmconfig 承载 LLM 配置相关的业务逻辑，例如探针式连通性测试。
// 把这些与 HTTP 协议细节强相关、但与 gin handler 无关的代码放在本子包里，
// router 只负责 "解析请求参数 → 调 llmconfig.Test → 渲染响应"。
package llmconfig

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/ccfos/nightingale/v6/models"
)

// Test 向目标 LLM 提供方发送一次最小探针请求，用于验证连通性与凭证是否可用。
// 出错时返回具体错误信息（例如 HTTP 状态码 + 截断后的响应体），成功则返回 nil。
func Test(p *models.AILLMConfig) error {
	extra := p.ExtraConfig

	// Build HTTP client with ExtraConfig settings
	timeout := 30 * time.Second
	if extra.TimeoutSeconds > 0 {
		timeout = time.Duration(extra.TimeoutSeconds) * time.Second
	}

	transport := &http.Transport{}
	if extra.SkipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	if extra.Proxy != "" {
		if proxyURL, err := url.Parse(extra.Proxy); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	client := &http.Client{Timeout: timeout, Transport: transport}

	var reqURL string
	var reqBody []byte
	hdrs := map[string]string{"Content-Type": "application/json"}

	switch p.APIType {
	case "openai", "ollama":
		base := strings.TrimRight(p.APIURL, "/")
		if strings.HasSuffix(base, "/chat/completions") {
			reqURL = base
		} else {
			reqURL = base + "/chat/completions"
		}
		reqBody, _ = json.Marshal(map[string]interface{}{
			"model":      p.Model,
			"messages":   []map[string]string{{"role": "user", "content": "Hi"}},
			"max_tokens": 5,
		})
		if p.APIKey != "" {
			hdrs["Authorization"] = "Bearer " + p.APIKey
		}
	case "kimi":
		// Kimi Code uses Anthropic Claude-compatible Messages API
		base := strings.TrimRight(p.APIURL, "/")
		if strings.HasSuffix(base, "/v1/messages") {
			reqURL = base
		} else if strings.HasSuffix(base, "/v1") {
			reqURL = base + "/messages"
		} else {
			reqURL = base + "/v1/messages"
		}
		reqBody, _ = json.Marshal(map[string]interface{}{
			"model":      p.Model,
			"messages":   []map[string]interface{}{{"role": "user", "content": []map[string]string{{"type": "text", "text": "Hi"}}}},
			"max_tokens": 5,
		})
		hdrs["x-api-key"] = p.APIKey
		hdrs["anthropic-version"] = "2023-06-01"
	case "claude":
		reqURL = llm.NormalizeClaudeURL(p.APIURL)
		reqBody, _ = json.Marshal(map[string]interface{}{
			"model":      p.Model,
			"messages":   []map[string]string{{"role": "user", "content": "Hi"}},
			"max_tokens": 5,
		})
		hdrs["x-api-key"] = p.APIKey
		hdrs["anthropic-version"] = "2023-06-01"
	case "gemini":
		base := llm.NormalizeGeminiBase(p.APIURL)
		if strings.Contains(base, ":generateContent") || strings.Contains(base, ":streamGenerateContent") {
			reqURL = base + "?key=" + p.APIKey
		} else {
			reqURL = base + "/" + p.Model + ":generateContent?key=" + p.APIKey
		}
		reqBody, _ = json.Marshal(map[string]interface{}{
			"contents": []map[string]interface{}{
				{"parts": []map[string]string{{"text": "Hi"}}},
			},
		})
	default:
		return fmt.Errorf("unsupported api_type: %s", p.APIType)
	}

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	// Apply custom headers from ExtraConfig
	for k, v := range extra.CustomHeaders {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 500 {
			body = body[:500]
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
