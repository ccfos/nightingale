package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/toolkits/pkg/logger"
)

const DefaultOpenAIURL = "https://api.openai.com/v1/chat/completions"

// maxRetries / initialRetryWait / maxRetryWait 迁移至 defaults.go

// OpenAI implements the LLM interface for OpenAI and compatible APIs
type OpenAI struct {
	config *Config
	client *http.Client
}

// NewOpenAI creates a new OpenAI provider
func NewOpenAI(cfg *Config, client *http.Client) (*OpenAI, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultOpenAIURL
	} else {
		cfg.BaseURL = NormalizeOpenAIURL(cfg.BaseURL)
	}
	return &OpenAI{
		config: cfg,
		client: client,
	}, nil
}

// versionSuffixRe 匹配版本号结尾的 base URL path，如 /v1、/api/v3、/api/paas/v4。
var versionSuffixRe = regexp.MustCompile(`(?i)/v\d+$`)

// NormalizeOpenAIURL 归一化 OpenAI 兼容端点：
// 用户常见填法是版本号结尾的 base URL（如 `https://host/v1`、智谱的 `/api/paas/v4`、
// 方舟的 `/api/v3`），对这类 URL 自动补 `/chat/completions`，避免漏路径时返回 404。
// 其余形态视为完整端点原样透传——存量配置里可能是不带标准路径的自定义网关地址，
// 不能改写；后缀判断基于 parse 后的 path，避免带 query 的完整端点被重复拼路径。
// 对于已知需要 /v1/chat/completions 路径的提供商（如 DeepSeek），
// 当用户只填写了根域名时自动补全完整路径。
func NormalizeOpenAIURL(rawURL string) string {
	u := strings.TrimRight(rawURL, "/")
	// DeepSeek 使用标准 OpenAI 兼容路径，用户填写根域名时自动补全
	if u == "https://api.deepseek.com" || u == "http://api.deepseek.com" {
		return u + "/v1/chat/completions"
	}

	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(path, "/chat/completions") || !versionSuffixRe.MatchString(path) {
		return u
	}
	parsed.Path = path + "/chat/completions"
	return parsed.String()
}

func (o *OpenAI) Name() string {
	return ProviderOpenAI
}

// OpenAI API request/response structures
type openAIRequest struct {
	Model               string          `json:"model"`
	Messages            []openAIMessage `json:"messages"`
	Tools               []openAITool    `json:"tools,omitempty"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	Temperature         float64         `json:"temperature,omitempty"`
	TopP                float64         `json:"top_p,omitempty"`
	Stream              bool            `json:"stream,omitempty"`

	// extraBody 不出现在 JSON tag 里——由 MarshalJSON 平铺到顶层。
	// 用于把 LLMConfig.ExtraBody 透传给厂商特定字段（如 dashscope 的 enable_thinking）。
	extraBody map[string]any
}

// MarshalJSON 把 extraBody 平铺到顶层 JSON。逻辑：先用别名走默认序列化（拿到所有
// 显式字段），unmarshal 回 map，再合并 extraBody，最后 marshal 出去。这种"先反射
// 再合并"的写法多绕一步，但好处是新增显式字段时不用同步改任何 reflection 代码。
// 已存在的 key 由显式字段优先（不让 extraBody 偷偷覆盖 model/messages 等关键字段）。
func (r openAIRequest) MarshalJSON() ([]byte, error) {
	type alias openAIRequest
	data, err := json.Marshal(alias(r))
	if err != nil {
		return nil, err
	}
	if len(r.extraBody) == 0 {
		return data, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	for k, v := range r.extraBody {
		// 两个 token 上限字段互斥：只要显式字段已经选定了一个，就不让 extraBody
		// 把另一个也带上（同时出现会被 OpenAI 判 400）。这里按"实际填了哪个字段"
		// 判断而不是重算模型名，否则 swapToMaxCompletionTokens 兜底改名之后
		// （模型名命不中前缀）这层剔除就会站错队。
		if (r.MaxCompletionTokens > 0 && k == "max_tokens") ||
			(r.MaxTokens > 0 && k == "max_completion_tokens") {
			continue
		}
		if _, occupied := m[k]; occupied {
			continue
		}
		m[k] = v
	}
	return json.Marshal(m)
}

// usesMaxCompletionTokens 判断模型是否属于 OpenAI 新一代推理模型家族
// （gpt-5 / o1 / o3 / o4）。这些模型废弃了 max_tokens，只认 max_completion_tokens。
//
// gpt-5 用宽前缀而不是 "gpt-5-"：官方型号里既有 gpt-5-mini 也有 gpt-5.1，
// 卡横杠会漏掉后者。o 系列反过来要卡精确值或带横杠的前缀，否则 o1x 之类会误伤。
//
// 与 thinking.go 的模型判断相互独立：那边管"是否注入关思考字段"，这里管"token
// 字段名"，成员集合不同（如 isPureThinkingModel 含 qwq/deepseek-r1/gemini 却不含
// gpt-5），故单列一个判断而非复用。
func usesMaxCompletionTokens(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(m, "gpt-5"),
		m == "o1", strings.HasPrefix(m, "o1-"),
		m == "o3", strings.HasPrefix(m, "o3-"),
		m == "o4", strings.HasPrefix(m, "o4-"):
		return true
	}
	return false
}

// cloneExtraBody 浅拷贝 extra body，让单次请求可以安全地增删自己的字段。
func cloneExtraBody(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// isMaxCompletionTokensError 命中"服务端因 max_tokens 不被支持而 400，并提示改用
// max_completion_tokens"的错误。错误串形如 `OpenAI API error (status 400): ...
// Use 'max_completion_tokens' instead.`，故以是否提到 max_completion_tokens 为判据。
func isMaxCompletionTokensError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "max_completion_tokens")
}

// swapToMaxCompletionTokens 是模型名识别的运行时兜底：当请求发了 max_tokens 却被
// 服务端要求改用 max_completion_tokens 时，就地把值迁到新字段并返回 true（供调用方
// 重试一次）；不满足条件则不动请求、返回 false。
//
// 只覆盖模型名漏判的场景——主要是 Azure 自定义部署名，那里 model 填的是部署名
// （如 my-gpt5），任何基于模型名的前缀匹配都会漏判。模型名能认出来的走
// convertRequest 里的提前迁移，不必先吃一个 400。
//
// 改完之后 MaxTokens == 0 且 extraBody 里不再有 max_tokens，第二次调用必然返回
// false，因此不会出现反复重试。
func swapToMaxCompletionTokens(req *openAIRequest, err error) bool {
	if !isMaxCompletionTokensError(err) {
		return false
	}
	changed := false
	if req.MaxTokens > 0 {
		req.MaxCompletionTokens = req.MaxTokens
		req.MaxTokens = 0
		changed = true
	}
	if takeExtraBodyMaxTokens(req) {
		changed = true
	}
	return changed
}

// takeExtraBodyMaxTokens 把 extraBody 里用户手填的 max_tokens 摘掉，值迁到
// MaxCompletionTokens，返回是否动过请求。
//
// 只删不迁的话请求会彻底没有输出上限，推理模型能跑出远超用户预期的长度和费用。
// 显式字段已有值时只删不迁，与 MarshalJSON 里"extraBody 不覆盖显式字段"的规则保持一致。
func takeExtraBodyMaxTokens(req *openAIRequest) bool {
	v, ok := req.extraBody["max_tokens"]
	if !ok {
		return false
	}
	delete(req.extraBody, "max_tokens")
	if n, ok := positiveInt(v); ok && req.MaxCompletionTokens == 0 {
		req.MaxCompletionTokens = n
	}
	return true
}

// positiveInt 从 extra body 取正整数。CustomParams 由 JSON 反序列化而来，数值落地是
// float64；程序内构造的则可能是 int/int64。取不到就返回 false，由调用方决定兜底。
func positiveInt(v any) (int, bool) {
	var n int
	switch x := v.(type) {
	case float64:
		n = int(x)
	case int:
		n = x
	case int64:
		n = int(x)
	default:
		return 0, false
	}
	if n <= 0 {
		return 0, false
	}
	return n, true
}

type openAIMessage struct {
	Role             string           `json:"role"`
	Content          string           `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	// Index 仅流式 delta 携带：同轮多个并行 tool_call 靠它区分归属。
	// 指针以区分"未携带"（部分网关）与合法的 index=0；请求侧序列化时省略。
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int           `json:"index"`
		Message      openAIMessage `json:"message"`
		Delta        openAIMessage `json:"delta"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

func (o *OpenAI) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	// Convert to OpenAI format
	openAIReq := o.convertRequest(req)
	openAIReq.Stream = false

	// Make request
	respBody, err := o.doRequest(ctx, openAIReq)
	if err != nil {
		// 模型名没命中家族前缀（如 Azure 自定义部署名）时，按服务端 400 的提示改名重试一次
		if !swapToMaxCompletionTokens(openAIReq, err) {
			return nil, err
		}
		if respBody, err = o.doRequest(ctx, openAIReq); err != nil {
			return nil, err
		}
	}

	// Parse response
	var openAIResp openAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if openAIResp.Error != nil {
		return nil, fmt.Errorf("OpenAI API error: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	// Convert to unified response
	return o.convertResponse(&openAIResp), nil
}

func (o *OpenAI) GenerateStream(ctx context.Context, req *GenerateRequest) (<-chan StreamChunk, error) {
	openAIReq := o.convertRequest(req)
	openAIReq.Stream = true

	resp, err := o.doStreamRequest(ctx, openAIReq)
	if err != nil {
		// 与 Generate 一致的兜底：名字识别漏判时按 400 提示改名重试一次
		if !swapToMaxCompletionTokens(openAIReq, err) {
			return nil, err
		}
		if resp, err = o.doStreamRequest(ctx, openAIReq); err != nil {
			return nil, err
		}
	}

	ch := make(chan StreamChunk, 100)
	go o.streamResponse(ctx, resp, ch)
	return ch, nil
}

// doStreamRequest 序列化请求并发起流式调用，成功时返回打开的响应交由调用方读取 Body。
// 序列化必须放在函数内部：兜底改名后要重新 marshal，复用旧 body 会把改名前的请求再发一遍。
func (o *OpenAI) doStreamRequest(ctx context.Context, req *openAIRequest) (*http.Response, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	// Debug 日志：确认 extraBody 等扩展字段是否真的被序列化进请求体。
	// 只在 ExtraBody 非空时打印，避免常态噪音；只打前 512 字节防止泄漏长 prompt。
	if len(o.config.ExtraBody) > 0 {
		preview := string(jsonData)
		if len(preview) > 512 {
			preview = preview[:512] + "...(truncated)"
		}
		logger.Debugf("[OpenAI] stream request body (with extra): %s", preview)
	}

	return doHTTPStreamWithRetry(ctx, o.client, "OpenAI",
		func() (*http.Request, error) {
			return http.NewRequestWithContext(ctx, "POST", o.config.BaseURL, bytes.NewBuffer(jsonData))
		},
		o.setHeaders,
	)
}

// openAIToolCallAggregator 按 index 聚合流式下发的 tool_call 分片。
// OpenAI 流式协议里同轮多个并行调用靠 index 字段区分：规范只保证每个调用首块带
// id+name，后续块只带 index+arguments 片段，且不同调用的片段可以交错下发；部分
// 兼容网关（qwen/deepseek 等）还会在每个 delta 重发 id+name。这里统一按 index
// 归槽累积，流结束时整块抛出（与 claude.go content_block_stop 的做法对齐），
// 消费端无需感知分片细节。
type openAIToolCallAggregator struct {
	slots map[int]*ToolCall
	order []int // 首见顺序，flush 时按它输出，保持调用顺序稳定
	last  int   // 最近分片归入的 index：兜底不带 index 的网关（"纯参数片段 → 续接上一个"）
}

func (a *openAIToolCallAggregator) add(tc openAIToolCall) {
	var idx int
	switch {
	case tc.Index != nil:
		idx = *tc.Index
	case tc.ID == "" && tc.Function.Name == "" && len(a.order) > 0:
		// 无 index 的纯参数片段：续接最近一个调用（旧启发式，仅作兜底）
		idx = a.last
	default:
		// 无 index 但带 id/name → 新调用，合成一个不与已有槽冲突的 index
		idx = -1
		for _, used := range a.order {
			if used >= idx {
				idx = used + 1
			}
		}
		if idx < 0 {
			idx = 0
		}
	}

	slot, ok := a.slots[idx]
	if !ok {
		if a.slots == nil {
			a.slots = map[int]*ToolCall{}
		}
		slot = &ToolCall{}
		a.slots[idx] = slot
		a.order = append(a.order, idx)
	}
	// id/name 只取首个非空值：网关重发的 id+name 不会再被误判成新调用
	if slot.ID == "" {
		slot.ID = tc.ID
	}
	if slot.Name == "" {
		slot.Name = tc.Function.Name
	}
	slot.Arguments += tc.Function.Arguments
	a.last = idx
}

// flush 按首见顺序吐出聚合完成的调用并清空状态（幂等，重复调用返回 nil）。
func (a *openAIToolCallAggregator) flush() []ToolCall {
	if len(a.order) == 0 {
		return nil
	}
	calls := make([]ToolCall, 0, len(a.order))
	for _, idx := range a.order {
		calls = append(calls, *a.slots[idx])
	}
	a.slots = nil
	a.order = nil
	return calls
}

func (o *OpenAI) streamResponse(ctx context.Context, resp *http.Response, ch chan<- StreamChunk) {
	defer close(ch)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	var toolCalls openAIToolCallAggregator

	// finish 在流正常收尾时把聚合好的完整 tool_call 整块抛出后再发 Done；
	// 出错路径不 flush——参数片段不完整，吐出去只会变成损坏的 JSON 调用。
	// complete 区分两种收尾：[DONE] 表示协议走完，调用原样下发（模型自产的坏
	// JSON 也该进工具循环，靠错误观测喂回模型重试）；EOF 兜底（网关没发 [DONE]
	// 就断流，如 idle timeout 干净 FIN）下连接可能停在 arguments 片段中途，
	// JSON 不完整的调用丢弃并告警——下游 unmarshal 失败会把残参包成
	// {"input": raw} 真执行，比丢调用更危险。空 arguments 视为完整（无参调用）。
	finish := func(complete bool) {
		calls := toolCalls.flush()
		if !complete {
			intact := calls[:0]
			for _, call := range calls {
				if call.Arguments == "" || json.Valid([]byte(call.Arguments)) {
					intact = append(intact, call)
					continue
				}
				logger.Warningf("[OpenAI] stream ended without [DONE], drop tool call with truncated arguments: name=%s id=%s args_len=%d", call.Name, call.ID, len(call.Arguments))
			}
			calls = intact
		}
		if len(calls) > 0 {
			ch <- StreamChunk{ToolCalls: calls}
		}
		ch <- StreamChunk{Done: true}
	}

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
				finish(false)
			}
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			finish(true)
			return
		}

		var streamResp openAIResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			continue
		}

		if len(streamResp.Choices) > 0 {
			delta := streamResp.Choices[0].Delta
			chunk := StreamChunk{
				Content:      delta.Content,
				Reasoning:    delta.ReasoningContent,
				FinishReason: streamResp.Choices[0].FinishReason,
			}

			// tool_call 分片只进聚合器，不随增量 chunk 下发；流收尾时整块抛出
			for _, tc := range delta.ToolCalls {
				toolCalls.add(tc)
			}

			if chunk.Content != "" || chunk.Reasoning != "" || chunk.FinishReason != "" {
				ch <- chunk
			}
		}
	}
}

func (o *OpenAI) convertRequest(req *GenerateRequest) *openAIRequest {
	openAIReq := &openAIRequest{
		Model: o.config.Model,
		TopP:  req.TopP,
		// 复制一份：config 被 client_cache 复用并发共享，而 swapToMaxCompletionTokens
		// 兜底时需要就地摘掉 extraBody 里的 max_tokens，直接引用会写坏共享 map。
		extraBody: cloneExtraBody(o.config.ExtraBody),
	}

	// Temperature: request 优先，fallback 到 config 默认值
	switch {
	case req.Temperature != nil:
		openAIReq.Temperature = *req.Temperature
	case o.config.Temperature != nil:
		openAIReq.Temperature = *o.config.Temperature
	}

	// MaxTokens: 同上
	var maxTokens int
	switch {
	case req.MaxTokens != nil:
		maxTokens = *req.MaxTokens
	case o.config.MaxTokens != nil:
		maxTokens = *o.config.MaxTokens
	}
	if usesMaxCompletionTokens(openAIReq.Model) {
		openAIReq.MaxCompletionTokens = maxTokens
		// 模型名已经确定该用新字段：CustomParams 里手填的 max_tokens 现在就迁走。
		// 留着的话（没配 MaxTokens 时 MarshalJSON 的互斥剔除不生效）会被平铺进请求体，
		// 每次调用都要先吃一个 400 再靠 swapToMaxCompletionTokens 兜底重试——agent
		// 工具循环里一轮十几次调用就是十几个白跑的往返。
		takeExtraBodyMaxTokens(openAIReq)
	} else {
		openAIReq.MaxTokens = maxTokens
	}

	// Convert messages. 结构化工具轮：assistant 的 ToolCalls 与 RoleTool 结果轮
	// 直接映射到 OpenAI 原生 tool_calls/tool 角色。
	for _, msg := range req.Messages {
		oaMsg := openAIMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		// tool 结果轮 content 必填：工具返回空串时，omitempty 会把 content 字段
		// 整个丢掉（对 assistant tool-call 轮是对的），严格端点直接 400 拒绝整单。
		if msg.Role == RoleTool && oaMsg.Content == "" {
			oaMsg.Content = "(empty result)"
		}
		for _, tc := range msg.ToolCalls {
			oaTC := openAIToolCall{ID: tc.ID, Type: "function"}
			oaTC.Function.Name = tc.Name
			oaTC.Function.Arguments = tc.Arguments
			oaMsg.ToolCalls = append(oaMsg.ToolCalls, oaTC)
		}
		openAIReq.Messages = append(openAIReq.Messages, oaMsg)
	}

	// Convert tools
	for _, tool := range req.Tools {
		openAIReq.Tools = append(openAIReq.Tools, openAITool{
			Type: "function",
			Function: openAIFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}

	return openAIReq
}

func (o *OpenAI) convertResponse(resp *openAIResponse) *GenerateResponse {
	result := &GenerateResponse{}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		result.Content = choice.Message.Content
		result.ReasoningContent = choice.Message.ReasoningContent
		result.FinishReason = choice.FinishReason

		// Convert tool calls
		for _, tc := range choice.Message.ToolCalls {
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
	}

	if resp.Usage != nil {
		result.Usage = &Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	return result
}

func (o *OpenAI) doRequest(ctx context.Context, req *openAIRequest) ([]byte, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	return doHTTPWithRetry(ctx, o.client, "OpenAI",
		func() (*http.Request, error) {
			return http.NewRequestWithContext(ctx, "POST", o.config.BaseURL, bytes.NewBuffer(jsonData))
		},
		o.setHeaders,
	)
}

func (o *OpenAI) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")

	if o.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.config.APIKey)
	}

	ApplyCustomHeaders(req, o.config.Headers)
}
