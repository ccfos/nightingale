package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/toolkits/pkg/logger"
)

const (
	DefaultGeminiURL = "https://generativelanguage.googleapis.com/v1beta/models"
)

// toInt 把 JSON 反序列化出来的"数字"（可能是 int / float64 / json.Number）
// 收敛成 int。NormalizeThinkingParams 内部直接构造 map，会得到 int；用户
// 自己填的 CustomParams 经过 gorm json serializer 后会变 float64，所以两者都要兼容。
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err == nil {
			return int(i), true
		}
	}
	return 0, false
}

// Gemini implements the LLM interface for Google Gemini API
type Gemini struct {
	config *Config
	client *http.Client
}

// NewGemini creates a new Gemini provider
func NewGemini(cfg *Config, client *http.Client) (*Gemini, error) {
	cfg.BaseURL = NormalizeGeminiBase(cfg.BaseURL)
	return &Gemini{
		config: cfg,
		client: client,
	}, nil
}

func (g *Gemini) Name() string {
	return ProviderGemini
}

// Gemini API request/response structures
type geminiRequest struct {
	Contents          []geminiContent         `json:"contents"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	Tools             []geminiTool            `json:"tools,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`

	// 思考模型字段：Thought 标记该 part 是思考摘要（路由到 reasoning 通道，
	// 不算答案）；ThoughtSignature 附在 functionCall part 上，续轮请求必须
	// 原样回传（Gemini 3 缺失会 4xx）——native 下放开 thinking 的前提。
	Thought          bool   `json:"thought,omitempty"`
	ThoughtSignature string `json:"thoughtSignature,omitempty"`
}

type geminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type geminiFunctionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type geminiFunctionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature     float64               `json:"temperature,omitempty"`
	TopP            float64               `json:"topP,omitempty"`
	MaxOutputTokens int                   `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

// geminiThinkingConfig 对应 Gemini 2.5+ 的 thinking 控制字段。
// ThinkingBudget=0 关闭思考（仅 Flash 系列支持，Pro 关不掉）；
// ThinkingLevel 是 Gemini 3 引入的替代字段，取值如 "minimal"。
// 用 *int 区分"没设置"和"显式设为 0"——后者要序列化为 thinkingBudget:0。
type geminiThinkingConfig struct {
	ThinkingBudget *int   `json:"thinkingBudget,omitempty"`
	ThinkingLevel  string `json:"thinkingLevel,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content       geminiContent `json:"content"`
		FinishReason  string        `json:"finishReason"`
		SafetyRatings []struct {
			Category    string `json:"category"`
			Probability string `json:"probability"`
		} `json:"safetyRatings,omitempty"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (g *Gemini) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	geminiReq := g.convertRequest(req)

	url := g.buildURL(false)
	respBody, err := g.doRequest(ctx, url, geminiReq)
	if err != nil {
		return nil, err
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if geminiResp.Error != nil {
		return nil, fmt.Errorf("Gemini API error: %s", geminiResp.Error.Message)
	}

	return g.convertResponse(&geminiResp), nil
}

func (g *Gemini) GenerateStream(ctx context.Context, req *GenerateRequest) (<-chan StreamChunk, error) {
	geminiReq := g.convertRequest(req)

	jsonData, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := g.buildURL(true)
	resp, err := doHTTPStreamWithRetry(ctx, g.client, "Gemini",
		func() (*http.Request, error) {
			return http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		},
		g.setHeaders,
	)
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamChunk, 100)
	go g.streamResponse(ctx, resp, ch)
	return ch, nil
}

func (g *Gemini) streamResponse(ctx context.Context, resp *http.Response, ch chan<- StreamChunk) {
	defer close(ch)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	var buffer strings.Builder

	for {
		select {
		case <-ctx.Done():
			ch <- StreamChunk{Done: true, Error: ctx.Err()}
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				ch <- StreamChunk{Done: true, Error: err}
			} else {
				ch <- StreamChunk{Done: true}
			}
			return
		}

		line = strings.TrimSpace(line)

		// Gemini streams JSON objects, accumulate until we have a complete one
		if line == "" {
			continue
		}

		// Handle SSE format if present
		if strings.HasPrefix(line, "data: ") {
			line = strings.TrimPrefix(line, "data: ")
		}

		buffer.WriteString(line)

		// Try to parse accumulated JSON
		var geminiResp geminiResponse
		if err := json.Unmarshal([]byte(buffer.String()), &geminiResp); err != nil {
			// Not complete yet, continue accumulating
			continue
		}

		// Reset buffer for next response
		buffer.Reset()

		if len(geminiResp.Candidates) > 0 {
			candidate := geminiResp.Candidates[0]
			chunk := StreamChunk{
				FinishReason: candidate.FinishReason,
			}

			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					if part.Thought {
						chunk.Reasoning += part.Text // 思考摘要走 reasoning 通道，不是答案
					} else {
						chunk.Content += part.Text
					}
				}
				if part.FunctionCall != nil {
					argsJSON, _ := json.Marshal(part.FunctionCall.Args)
					chunk.ToolCalls = append(chunk.ToolCalls, ToolCall{
						Name:             part.FunctionCall.Name,
						Arguments:        string(argsJSON),
						ThoughtSignature: part.ThoughtSignature,
					})
				}
			}

			ch <- chunk

			if candidate.FinishReason != "" && candidate.FinishReason != "STOP" {
				ch <- StreamChunk{Done: true}
				return
			}
		}
	}
}

func (g *Gemini) convertRequest(req *GenerateRequest) *geminiRequest {
	genCfg := &geminiGenerationConfig{
		TopP: req.TopP,
	}

	switch {
	case req.Temperature != nil:
		genCfg.Temperature = *req.Temperature
	case g.config.Temperature != nil:
		genCfg.Temperature = *g.config.Temperature
	}

	switch {
	case req.MaxTokens != nil:
		genCfg.MaxOutputTokens = *req.MaxTokens
	case g.config.MaxTokens != nil:
		genCfg.MaxOutputTokens = *g.config.MaxTokens
	}

	// Gemini 没有 OpenAI 风格的 extra_body 平铺，thinking 控制必须落进
	// generationConfig.thinkingConfig 里。这里把 NormalizeThinkingParams 注入的
	// map 桥接成强类型字段。容错地读 budget / level，转不出来就静默忽略——
	// extra 里可能还混着别的厂商字段，不能因为格式不符直接报错。
	//
	// 同时尝试 snake_case 和 camelCase：前者是 NormalizeThinkingParams 注入的写法，
	// 后者是 Gemini 官方文档里的写法（用户可能照官方文档拷过来）。两种都收，避免静默失效。
	tc, ok := g.config.ExtraBody["thinking_config"].(map[string]any)
	if !ok {
		tc, ok = g.config.ExtraBody["thinkingConfig"].(map[string]any)
	}
	if ok {
		gtc := &geminiThinkingConfig{}
		// budget 也同样两种 key 都收
		budget, hasBudget := tc["thinking_budget"]
		if !hasBudget {
			budget, hasBudget = tc["thinkingBudget"]
		}
		if hasBudget {
			if n, ok := toInt(budget); ok {
				gtc.ThinkingBudget = &n
			}
		}
		level, _ := tc["thinking_level"].(string)
		if level == "" {
			level, _ = tc["thinkingLevel"].(string)
		}
		gtc.ThinkingLevel = level
		if gtc.ThinkingBudget != nil || gtc.ThinkingLevel != "" {
			genCfg.ThinkingConfig = gtc
		}
	}

	// Gemini provider 只消费 thinking_config / thinkingConfig，其它 ExtraBody 键无效。
	// 这是 Gemini API 形态本身决定的（请求字段都嵌在 generationConfig 等强类型结构里，
	// 没有 OpenAI 那种顶层平铺的逃生口）。这里给个 debug 日志，避免用户在 CustomParams
	// 里塞了别的字段却无声无息地不生效、查不出原因。
	if len(g.config.ExtraBody) > 0 {
		var unused []string
		for k := range g.config.ExtraBody {
			if k == "thinking_config" || k == "thinkingConfig" {
				continue
			}
			unused = append(unused, k)
		}
		if len(unused) > 0 {
			logger.Debugf("[Gemini] ignored ExtraBody keys (provider only consumes thinking_config): %v", unused)
		}
	}

	geminiReq := &geminiRequest{
		GenerationConfig: genCfg,
	}

	// Convert messages
	for _, msg := range req.Messages {
		if msg.Role == RoleSystem {
			geminiReq.SystemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: msg.Content}},
			}
			continue
		}

		// 工具结果轮：functionResponse part。Gemini 的
		// role 只允许 user|model，结果归 user；按 ToolName 匹配（不用 id），
		// response 要求对象形态。连续多条结果（并行调用）合并进同一个 user
		// content——Gemini 同样要求 user/model 交替。
		if msg.Role == RoleTool {
			part := geminiPart{FunctionResponse: &geminiFunctionResponse{
				Name:     msg.ToolName,
				Response: parseToolJSONObject(msg.Content, "result"),
			}}
			if n := len(geminiReq.Contents); n > 0 && geminiReq.Contents[n-1].Role == "user" &&
				len(geminiReq.Contents[n-1].Parts) > 0 && geminiReq.Contents[n-1].Parts[0].FunctionResponse != nil {
				geminiReq.Contents[n-1].Parts = append(geminiReq.Contents[n-1].Parts, part)
			} else {
				geminiReq.Contents = append(geminiReq.Contents, geminiContent{
					Role:  "user",
					Parts: []geminiPart{part},
				})
			}
			continue
		}

		// Map roles
		role := msg.Role
		if role == RoleAssistant {
			role = "model"
		}

		// assistant 工具调用轮：text part（如有）+ functionCall parts；
		// 纯文本轮保持原有单 text part 形态。
		var parts []geminiPart
		if msg.Content != "" || len(msg.ToolCalls) == 0 {
			parts = append(parts, geminiPart{Text: msg.Content})
		}
		for _, tc := range msg.ToolCalls {
			parts = append(parts, geminiPart{
				FunctionCall: &geminiFunctionCall{
					Name: tc.Name,
					Args: parseToolJSONObject(tc.Arguments, "input"),
				},
				// 回传思考签名（Gemini 3 工具续轮硬性要求）。
				ThoughtSignature: tc.ThoughtSignature,
			})
		}

		geminiReq.Contents = append(geminiReq.Contents, geminiContent{
			Role:  role,
			Parts: parts,
		})
	}

	// Convert tools
	if len(req.Tools) > 0 {
		var declarations []geminiFunctionDeclaration
		for _, tool := range req.Tools {
			declarations = append(declarations, geminiFunctionDeclaration{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			})
		}
		geminiReq.Tools = []geminiTool{{FunctionDeclarations: declarations}}
	}

	return geminiReq
}

func (g *Gemini) convertResponse(resp *geminiResponse) *GenerateResponse {
	result := &GenerateResponse{}

	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]
		result.FinishReason = candidate.FinishReason

		var textParts []string
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				if part.Thought {
					result.ReasoningContent += part.Text // 思考摘要不进答案
				} else {
					textParts = append(textParts, part.Text)
				}
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				result.ToolCalls = append(result.ToolCalls, ToolCall{
					Name:             part.FunctionCall.Name,
					Arguments:        string(argsJSON),
					ThoughtSignature: part.ThoughtSignature,
				})
			}
		}
		result.Content = strings.Join(textParts, "")
	}

	if resp.UsageMetadata != nil {
		result.Usage = &Usage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		}
	}

	return result
}

func (g *Gemini) buildURL(stream bool) string {
	action := "generateContent"
	if stream {
		action = "streamGenerateContent"
	}

	// Check if baseURL already contains the full path
	if strings.Contains(g.config.BaseURL, ":generateContent") ||
		strings.Contains(g.config.BaseURL, ":streamGenerateContent") {
		return fmt.Sprintf("%s?key=%s", g.config.BaseURL, g.config.APIKey)
	}

	return fmt.Sprintf("%s/%s:%s?key=%s",
		g.config.BaseURL,
		g.config.Model,
		action,
		g.config.APIKey,
	)
}

func (g *Gemini) doRequest(ctx context.Context, url string, req *geminiRequest) ([]byte, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	return doHTTPWithRetry(ctx, g.client, "Gemini",
		func() (*http.Request, error) {
			return http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		},
		g.setHeaders,
	)
}

func (g *Gemini) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")

	ApplyCustomHeaders(req, g.config.Headers)
}
