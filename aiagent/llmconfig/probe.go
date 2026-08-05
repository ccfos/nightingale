// Package llmconfig 承载 LLM 配置相关的业务逻辑，例如探针式连通性测试。
// 把这些与 HTTP 协议细节强相关、但与 gin handler 无关的代码放在本子包里，
// router 只负责 "解析请求参数 → 调 llmconfig.Test → 渲染响应"。
package llmconfig

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/ccfos/nightingale/v6/models"
)

type ProbeErrorKind string

const (
	ProbeErrorAuth               ProbeErrorKind = "auth"
	ProbeErrorEndpointNotFound   ProbeErrorKind = "endpoint_not_found"
	ProbeErrorRateLimited        ProbeErrorKind = "rate_limited"
	ProbeErrorRequestFailed      ProbeErrorKind = "request_failed"
	ProbeErrorUnexpectedResponse ProbeErrorKind = "unexpected_response"
	ProbeErrorModel              ProbeErrorKind = "model"
	ProbeErrorNoContent          ProbeErrorKind = "no_content"
)

// ProbeError is the structured domain error returned by LLM connectivity tests.
// It keeps provider and transport details separate from any i18n presentation logic.
type ProbeError struct {
	Kind       ProbeErrorKind
	StatusCode int
	APIURL     string
	Model      string
	Detail     string
	Cause      error
}

func (e *ProbeError) Error() string {
	parts := []string{"llm probe failed", "kind=" + string(e.Kind)}
	if e.StatusCode > 0 {
		parts = append(parts, fmt.Sprintf("status=%d", e.StatusCode))
	}
	if e.Model != "" {
		parts = append(parts, "model="+e.Model)
	}
	if e.Detail != "" {
		parts = append(parts, "detail="+e.Detail)
	}
	if e.Cause != nil {
		parts = append(parts, "cause="+e.Cause.Error())
	}
	return strings.Join(parts, ", ")
}

func (e *ProbeError) Unwrap() error {
	return e.Cause
}

// Test sends a real chat request to the target LLM provider to verify connectivity,
// credentials, and model availability. Returns a ProbeError for classified failures.
func Test(p *models.AILLMConfig) error {
	client, err := llm.New(BuildLLMConfig(p))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), ProbeTimeout(p.ExtraConfig))
	defer cancel()

	// 统一给足预算：推理/思考模型（o1 系列、gpt-5、deepseek-r1、gemini-2.5-pro 等）会先把
	// token 花在思考上，预算过小会让 content 为空而误报「无内容」。普通模型对 "Hi" 会
	// finish_reason=stop 提前收尾，抬高上限并不增加实际消耗，故不按模型名分档、不再多
	// 维护一张表。512 仍可能被推理模型烧光，那种情况由下面的截断判定兜底。
	maxTokens := 512
	resp, err := client.Generate(ctx, &llm.GenerateRequest{
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
		MaxTokens: &maxTokens,
	})
	if err != nil {
		return classifyProbeError(p, err)
	}
	if resp == nil {
		return &ProbeError{Kind: ProbeErrorNoContent, Model: p.Model}
	}
	// 预算被 reasoning 吃光时正文为空、finish_reason=length，但这轮请求本身是成功的
	// ——端点、鉴权、模型都验证过了，探测目的已达成，不能判失败。o1 系列关不掉思考，
	// Azure 自定义部署名下的 gpt-5 也命不中前缀、注入不了 reasoning_effort，都会走到这里。
	if llm.ClassifyFinish(resp.FinishReason) == llm.FinishTruncated {
		return nil
	}
	if strings.TrimSpace(resp.Content) == "" && strings.TrimSpace(resp.ReasoningContent) == "" {
		return &ProbeError{Kind: ProbeErrorNoContent, Model: p.Model}
	}
	return nil
}

var providerStatusErrPattern = regexp.MustCompile(`(?i)^(?:[a-z0-9_ -]+) API error \(status (\d+)\):\s*(.*)$`)

// BuildLLMConfig maps a stored AILLMConfig into an llm.Config usable by the
// unified aiagent/llm client. Exported so other consumers (e.g. the workflow
// AI summary processor) can reuse a centrally-managed LLM config instead of
// re-implementing the mapping.
func BuildLLMConfig(p *models.AILLMConfig) *llm.Config {
	return &llm.Config{
		Provider:      p.APIType,
		BaseURL:       p.APIURL,
		APIKey:        p.APIKey,
		Model:         p.Model,
		Headers:       cloneStringMap(p.ExtraConfig.CustomHeaders),
		Timeout:       int(ProbeTimeout(p.ExtraConfig) / time.Millisecond),
		SkipSSLVerify: p.ExtraConfig.SkipTLSVerify,
		Proxy:         p.ExtraConfig.Proxy,
		Temperature:   p.ExtraConfig.Temperature,
		MaxTokens:     p.ExtraConfig.MaxTokens,
		ExtraBody:     llm.NormalizeThinkingParams(p.APIType, p.APIURL, p.Model, p.ExtraConfig.CustomParams),
	}
}

// ProbeTimeout resolves the request timeout from a config's extra settings,
// defaulting to 30s when unset.
func ProbeTimeout(extra models.LLMExtraConfig) time.Duration {
	timeout := 30 * time.Second
	if extra.TimeoutSeconds > 0 {
		timeout = time.Duration(extra.TimeoutSeconds) * time.Second
	}
	return timeout
}

func classifyProbeError(p *models.AILLMConfig, err error) error {
	msg := err.Error()
	if statusCode, raw, ok := parseProviderStatusError(msg); ok {
		return formatHTTPError(statusCode, p.APIURL, raw)
	}

	if strings.Contains(msg, "failed to parse response") {
		return &ProbeError{
			Kind:   ProbeErrorUnexpectedResponse,
			APIURL: p.APIURL,
			Detail: truncate(msg, 200),
			Cause:  err,
		}
	}

	if strings.Contains(msg, "no response from OpenAI") {
		return &ProbeError{Kind: ProbeErrorNoContent, Model: p.Model, Cause: err}
	}

	if providerMsg, ok := extractProviderMessage(msg); ok {
		return &ProbeError{
			Kind:   ProbeErrorModel,
			Model:  p.Model,
			Detail: providerMsg,
			Cause:  err,
		}
	}

	return err
}

func parseProviderStatusError(msg string) (int, string, bool) {
	matches := providerStatusErrPattern.FindStringSubmatch(msg)
	if len(matches) != 3 {
		return 0, "", false
	}
	var statusCode int
	if _, err := fmt.Sscanf(matches[1], "%d", &statusCode); err != nil {
		return 0, "", false
	}
	return statusCode, matches[2], true
}

func extractProviderMessage(msg string) (string, bool) {
	for _, prefix := range []string{"OpenAI API error: ", "Claude API error: ", "Gemini API error: "} {
		if strings.HasPrefix(msg, prefix) {
			providerMsg := strings.TrimSpace(strings.TrimPrefix(msg, prefix))
			if providerMsg != "" {
				return providerMsg, true
			}
		}
	}
	return "", false
}

// formatHTTPError converts an HTTP error status into a structured ProbeError.
func formatHTTPError(statusCode int, apiURL, raw string) *ProbeError {
	raw = truncate(raw, 300)
	switch statusCode {
	case 401, 403:
		return &ProbeError{Kind: ProbeErrorAuth, StatusCode: statusCode, APIURL: apiURL, Detail: raw}
	case 404:
		return &ProbeError{Kind: ProbeErrorEndpointNotFound, StatusCode: statusCode, APIURL: apiURL, Detail: raw}
	case 429:
		return &ProbeError{Kind: ProbeErrorRateLimited, StatusCode: statusCode, APIURL: apiURL, Detail: raw}
	default:
		return &ProbeError{Kind: ProbeErrorRequestFailed, StatusCode: statusCode, APIURL: apiURL, Detail: raw}
	}
}

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
