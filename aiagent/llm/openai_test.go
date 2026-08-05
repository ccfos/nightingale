package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// runOpenAIStream 把一段 SSE 文本灌进 streamResponse，收齐所有 chunk。
func runOpenAIStream(t *testing.T, sse string) []StreamChunk {
	t.Helper()
	o := &OpenAI{config: &Config{}}
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(sse))}
	ch := make(chan StreamChunk, 100)
	go o.streamResponse(context.Background(), resp, ch)

	var chunks []StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}
	return chunks
}

func collectToolCalls(chunks []StreamChunk) []ToolCall {
	var calls []ToolCall
	for _, c := range chunks {
		calls = append(calls, c.ToolCalls...)
	}
	return calls
}

// TestOpenAIStream_ParallelToolCallsInterleaved 验证两个并行 tool_call 的参数
// 片段交错下发（合法 SSE 行为）时按 index 正确归槽，不会把 A 的参数拼进 B。
func TestOpenAIStream_ParallelToolCallsInterleaved(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"query_metrics","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"list_hosts","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"promql\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"group\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"up\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"\"prod\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"

	chunks := runOpenAIStream(t, sse)
	calls := collectToolCalls(chunks)

	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d: %+v", len(calls), calls)
	}
	if calls[0].ID != "call_a" || calls[0].Name != "query_metrics" || calls[0].Arguments != `{"promql":"up"}` {
		t.Errorf("call 0 mis-aggregated: %+v", calls[0])
	}
	if calls[1].ID != "call_b" || calls[1].Name != "list_hosts" || calls[1].Arguments != `{"group":"prod"}` {
		t.Errorf("call 1 mis-aggregated: %+v", calls[1])
	}
	if !chunks[len(chunks)-1].Done {
		t.Errorf("stream should end with Done chunk")
	}
}

// TestOpenAIStream_GatewayResendsIDName 验证兼容网关（qwen/deepseek 等）在每个
// delta 重发 id+name 时不会被拆成多个参数残缺的重复调用。
func TestOpenAIStream_GatewayResendsIDName(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_x","type":"function","function":{"name":"get_alert","arguments":"{\"id\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_x","type":"function","function":{"name":"get_alert","arguments":"123}"}}]}}]}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"

	calls := collectToolCalls(runOpenAIStream(t, sse))

	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d: %+v", len(calls), calls)
	}
	if calls[0].ID != "call_x" || calls[0].Name != "get_alert" || calls[0].Arguments != `{"id":123}` {
		t.Errorf("call mis-aggregated: %+v", calls[0])
	}
}

// TestOpenAIStream_NoIndexFallback 验证不带 index 的网关退回旧启发式：
// 纯参数片段续接最近一个调用，带 id/name 的片段开新调用。
func TestOpenAIStream_NoIndexFallback(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"f1","arguments":"{\"a\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"1}"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"id":"call_2","type":"function","function":{"name":"f2","arguments":"{}"}}]}}]}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"

	calls := collectToolCalls(runOpenAIStream(t, sse))

	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d: %+v", len(calls), calls)
	}
	if calls[0].ID != "call_1" || calls[0].Arguments != `{"a":1}` {
		t.Errorf("call 0 mis-aggregated: %+v", calls[0])
	}
	if calls[1].ID != "call_2" || calls[1].Arguments != `{}` {
		t.Errorf("call 1 mis-aggregated: %+v", calls[1])
	}
}

// TestOpenAIStream_FlushOnEOF 验证上游没发 [DONE] 直接断流时（EOF），
// 已聚合完的 tool_call 仍会整块抛出，不会丢调用。
func TestOpenAIStream_FlushOnEOF(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_e","type":"function","function":{"name":"fe","arguments":"{}"}}]}}]}`,
	}, "\n\n") + "\n\n"

	chunks := runOpenAIStream(t, sse)
	calls := collectToolCalls(chunks)

	if len(calls) != 1 || calls[0].ID != "call_e" {
		t.Fatalf("expected call_e flushed on EOF, got: %+v", calls)
	}
	if !chunks[len(chunks)-1].Done {
		t.Errorf("stream should end with Done chunk")
	}
}

// TestOpenAIStream_EOFDropsTruncatedArgs 验证 EOF 兜底路径（没发 [DONE]）下，
// 连接停在 arguments 片段中途的调用被丢弃，已聚合完整的调用照常下发——
// 截断参数吐给下游会被包成 {"input": raw} 真执行。空 arguments 视为完整。
func TestOpenAIStream_EOFDropsTruncatedArgs(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_ok","type":"function","function":{"name":"f_ok","arguments":"{\"a\":1}"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_noargs","type":"function","function":{"name":"f_noargs","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":2,"id":"call_cut","type":"function","function":{"name":"f_cut","arguments":"{\"promql\":\"up"}}]}}]}`,
	}, "\n\n") + "\n\n"

	chunks := runOpenAIStream(t, sse)
	calls := collectToolCalls(chunks)

	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls (truncated one dropped), got %d: %+v", len(calls), calls)
	}
	if calls[0].ID != "call_ok" || calls[0].Arguments != `{"a":1}` {
		t.Errorf("complete call should survive EOF flush: %+v", calls[0])
	}
	if calls[1].ID != "call_noargs" || calls[1].Arguments != "" {
		t.Errorf("empty-args call should survive EOF flush: %+v", calls[1])
	}
	if !chunks[len(chunks)-1].Done {
		t.Errorf("stream should end with Done chunk")
	}
}

// TestOpenAIStream_DoneKeepsInvalidArgs 验证 [DONE] 路径不做 JSON 校验：协议
// 走完后的坏 JSON 是模型自己产出的，应原样进工具循环，靠错误观测喂回模型重试，
// 而不是在传输层静默丢掉。
func TestOpenAIStream_DoneKeepsInvalidArgs(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bad","type":"function","function":{"name":"f_bad","arguments":"{\"a\":"}}]}}]}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"

	calls := collectToolCalls(runOpenAIStream(t, sse))

	if len(calls) != 1 || calls[0].ID != "call_bad" || calls[0].Arguments != `{"a":` {
		t.Fatalf("invalid args after [DONE] should pass through untouched, got: %+v", calls)
	}
}

// TestOpenAIStream_ContentPassthrough 验证正文/finish_reason 增量行为不受
// 聚合器改动影响。
func TestOpenAIStream_ContentPassthrough(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hello "}}]}`,
		`data: {"choices":[{"delta":{"content":"world"},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"

	chunks := runOpenAIStream(t, sse)

	var content strings.Builder
	var finish string
	for _, c := range chunks {
		content.WriteString(c.Content)
		if c.FinishReason != "" {
			finish = c.FinishReason
		}
	}
	if content.String() != "hello world" {
		t.Errorf("content mis-streamed: %q", content.String())
	}
	if finish != "stop" {
		t.Errorf("finish_reason lost: %q", finish)
	}
}

// TestConvertRequest_ToolTurnEmptyContent 验证工具返回空串时 tool 结果轮被填
// 占位符——content 经 omitempty 整个丢字段会被严格端点 400 拒绝；assistant
// tool-call 轮的空 content 则必须保持可省略。
func TestConvertRequest_ToolTurnEmptyContent(t *testing.T) {
	o := &OpenAI{config: &Config{Model: "gpt-x"}}
	out := o.convertRequest(&GenerateRequest{Messages: []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "noop", Arguments: "{}"}}},
		{Role: RoleTool, ToolCallID: "c1", Content: ""},
		{Role: RoleTool, ToolCallID: "c1", Content: "real result"},
	}})

	if out.Messages[0].Content != "" {
		t.Errorf("assistant tool-call turn content should stay empty, got %q", out.Messages[0].Content)
	}
	if out.Messages[1].Content == "" {
		t.Errorf("empty tool result must get a placeholder, got empty (field would be dropped by omitempty)")
	}
	if out.Messages[2].Content != "real result" {
		t.Errorf("non-empty tool result must pass through, got %q", out.Messages[2].Content)
	}
}

func TestOpenAIRequest_MaxTokensFieldByModel(t *testing.T) {
	tests := []struct {
		model     string
		wantField string
	}{
		{model: "gpt-5-mini", wantField: "max_completion_tokens"},
		{model: "o1", wantField: "max_completion_tokens"},
		{model: "o1-preview", wantField: "max_completion_tokens"},
		{model: "o3", wantField: "max_completion_tokens"},
		{model: "o3-mini", wantField: "max_completion_tokens"},
		// gpt-5.1 是点号不是横杠，o4 是新家族成员：两者都必须走新字段
		{model: "gpt-5.1", wantField: "max_completion_tokens"},
		{model: "gpt-5.1-mini", wantField: "max_completion_tokens"},
		{model: "o4-mini", wantField: "max_completion_tokens"},
		{model: "GPT-5-Nano", wantField: "max_completion_tokens"},
		{model: " o1 ", wantField: "max_completion_tokens"},
		{model: "gpt-4o", wantField: "max_tokens"},
		// o1x / o 不能被 o1 前缀误伤
		{model: "o1x", wantField: "max_tokens"},
		{model: "o", wantField: "max_tokens"},
		{model: "deepseek-chat", wantField: "max_tokens"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			tokenLimit := 123
			otherField := "max_tokens"
			if tt.wantField == "max_tokens" {
				otherField = "max_completion_tokens"
			}

			o := &OpenAI{config: &Config{
				Model:     tt.model,
				MaxTokens: &tokenLimit,
				ExtraBody: map[string]any{otherField: 456},
			}}
			data, err := json.Marshal(o.convertRequest(&GenerateRequest{}))
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}

			var body map[string]json.RawMessage
			if err := json.Unmarshal(data, &body); err != nil {
				t.Fatalf("unmarshal request: %v", err)
			}
			if _, ok := body[otherField]; ok {
				t.Fatalf("request contains both token fields: %s", data)
			}

			var got int
			if err := json.Unmarshal(body[tt.wantField], &got); err != nil {
				t.Fatalf("unmarshal %s: %v; body: %s", tt.wantField, err, data)
			}
			if got != tokenLimit {
				t.Errorf("%s = %d, want %d; body: %s", tt.wantField, got, tokenLimit, data)
			}
		})
	}
}

func TestOpenAIRequest_RequestMaxTokensForGPT5Probe(t *testing.T) {
	requestMaxTokens := 5
	o := &OpenAI{config: &Config{Model: "gpt-5-mini"}}
	data, err := json.Marshal(o.convertRequest(&GenerateRequest{MaxTokens: &requestMaxTokens}))
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if _, ok := body["max_tokens"]; ok {
		t.Fatalf("request unexpectedly contains max_tokens: %s", data)
	}

	var got int
	if err := json.Unmarshal(body["max_completion_tokens"], &got); err != nil {
		t.Fatalf("unmarshal max_completion_tokens: %v; body: %s", err, data)
	}
	if got != requestMaxTokens {
		t.Errorf("max_completion_tokens = %d, want %d; body: %s", got, requestMaxTokens, data)
	}
}

// TestSwapToMaxCompletionTokens 覆盖模型名漏判时的 400 兜底改名：命中提示就把
// max_tokens 迁到 max_completion_tokens，无关错误或已经改过名时保持请求不变。
func TestSwapToMaxCompletionTokens(t *testing.T) {
	err400 := fmt.Errorf("OpenAI API error (status 400): Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead.")

	req := &openAIRequest{MaxTokens: 100}
	if !swapToMaxCompletionTokens(req, err400) {
		t.Fatal("expected swap to trigger on max_completion_tokens hint")
	}
	if req.MaxTokens != 0 || req.MaxCompletionTokens != 100 {
		t.Fatalf("token should move to max_completion_tokens, got max_tokens=%d max_completion_tokens=%d",
			req.MaxTokens, req.MaxCompletionTokens)
	}
	// 幂等：改完之后再撞同样的错也不该重试，否则会无限循环
	if swapToMaxCompletionTokens(req, err400) {
		t.Fatal("second swap must not trigger")
	}

	unrelated := &openAIRequest{MaxTokens: 100}
	if swapToMaxCompletionTokens(unrelated, fmt.Errorf("OpenAI API error (status 401): invalid api key")) {
		t.Fatal("unrelated error must not trigger swap")
	}
	if unrelated.MaxTokens != 100 || unrelated.MaxCompletionTokens != 0 {
		t.Fatal("unrelated error must leave request untouched")
	}

	// 用户在 CustomParams 里手填的 max_tokens 同样会触发 400，兜底要一并摘掉
	viaExtra := &openAIRequest{extraBody: map[string]any{"max_tokens": 456, "enable_thinking": false}}
	if !swapToMaxCompletionTokens(viaExtra, err400) {
		t.Fatal("expected swap to strip max_tokens from extra body")
	}
	if _, ok := viaExtra.extraBody["max_tokens"]; ok {
		t.Fatal("extra body max_tokens must be removed")
	}
	if _, ok := viaExtra.extraBody["enable_thinking"]; !ok {
		t.Fatal("unrelated extra body keys must survive")
	}
}

// TestConvertRequest_ExtraBodyNotShared 确认每次请求拿到的是 extra body 的副本：
// 兜底改名会就地删 key，若直接引用 config 里的 map 会写坏并发共享的缓存配置。
func TestConvertRequest_ExtraBodyNotShared(t *testing.T) {
	cfg := &Config{Model: "my-azure-deploy", ExtraBody: map[string]any{"max_tokens": 456}}
	o := &OpenAI{config: cfg}

	req := o.convertRequest(&GenerateRequest{})
	delete(req.extraBody, "max_tokens")

	if _, ok := cfg.ExtraBody["max_tokens"]; !ok {
		t.Fatal("config extra body must not be mutated by a single request")
	}
}

// TestOpenAIRequest_MarshalKeepsSwappedField 确认改名兜底之后序列化仍然正确：
// 模型名（Azure 自定义部署名）命不中家族前缀，但显式字段已经是 max_completion_tokens，
// MarshalJSON 不能按模型名把它抹掉，也不能让 extraBody 把 max_tokens 塞回来。
func TestOpenAIRequest_MarshalKeepsSwappedField(t *testing.T) {
	req := &openAIRequest{
		Model:               "my-azure-deploy",
		MaxCompletionTokens: 100,
		extraBody:           map[string]any{"max_tokens": 456},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if _, ok := body["max_tokens"]; ok {
		t.Fatalf("max_tokens must not be reintroduced by extra body: %s", data)
	}
	var got int
	if err := json.Unmarshal(body["max_completion_tokens"], &got); err != nil {
		t.Fatalf("unmarshal max_completion_tokens: %v; body: %s", err, data)
	}
	if got != 100 {
		t.Errorf("max_completion_tokens = %d, want 100; body: %s", got, data)
	}
}

const unsupportedMaxTokensBody = `{"error":{"message":"Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead.","type":"invalid_request_error","param":"max_tokens","code":"unsupported_parameter"}}`

// newSwapProbeServer 起一个先拒后收的假上游：第一次请求回 400（提示改用
// max_completion_tokens），第二次回 okBody。返回收集到的两次请求体。
func newSwapProbeServer(t *testing.T, okBody string, sse bool) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		bodies = append(bodies, m)

		if len(bodies) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(unsupportedMaxTokensBody))
			return
		}
		if sse {
			w.Header().Set("Content-Type", "text/event-stream")
		}
		_, _ = w.Write([]byte(okBody))
	}))
	t.Cleanup(srv.Close)
	return srv, &bodies
}

// assertSwapped 校验兜底重试的两次请求体：第一次发 max_tokens，第二次改发
// max_completion_tokens 且不再带 max_tokens。
func assertSwapped(t *testing.T, bodies []map[string]any) {
	t.Helper()
	if len(bodies) != 2 {
		t.Fatalf("expected exactly 2 upstream requests (reject + retry), got %d", len(bodies))
	}
	if _, ok := bodies[0]["max_tokens"]; !ok {
		t.Errorf("first request should carry max_tokens, got %v", bodies[0])
	}
	if _, ok := bodies[1]["max_tokens"]; ok {
		t.Errorf("retry must not carry max_tokens, got %v", bodies[1])
	}
	if got := bodies[1]["max_completion_tokens"]; got != float64(5) {
		t.Errorf("retry max_completion_tokens = %v, want 5; body %v", got, bodies[1])
	}
}

// TestGenerate_RetriesWithMaxCompletionTokens 端到端验证兜底：模型名是 Azure 自定义
// 部署名（命不中家族前缀），首次请求被 400 拒绝后自动改名重试并成功（issue #3297）。
func TestGenerate_RetriesWithMaxCompletionTokens(t *testing.T) {
	srv, bodies := newSwapProbeServer(t, `{"choices":[{"message":{"role":"assistant","content":"Hello"}}]}`, false)

	maxTokens := 5
	o := &OpenAI{config: &Config{Model: "my-azure-deploy", BaseURL: srv.URL}, client: srv.Client()}
	resp, err := o.Generate(context.Background(), &GenerateRequest{
		Messages:  []Message{{Role: RoleUser, Content: "Hi"}},
		MaxTokens: &maxTokens,
	})
	if err != nil {
		t.Fatalf("Generate should recover after renaming the token field: %v", err)
	}
	if resp.Content != "Hello" {
		t.Errorf("content = %q, want %q", resp.Content, "Hello")
	}
	assertSwapped(t, *bodies)
}

// TestGenerateStream_RetriesWithMaxCompletionTokens 同上，覆盖流式路径——重试必须
// 重新序列化请求体，否则会把改名前的 body 再发一遍。
func TestGenerateStream_RetriesWithMaxCompletionTokens(t *testing.T) {
	sse := `data: {"choices":[{"delta":{"content":"Hello"}}]}` + "\n\n" + `data: [DONE]` + "\n\n"
	srv, bodies := newSwapProbeServer(t, sse, true)

	maxTokens := 5
	o := &OpenAI{config: &Config{Model: "my-azure-deploy", BaseURL: srv.URL}, client: srv.Client()}
	ch, err := o.GenerateStream(context.Background(), &GenerateRequest{
		Messages:  []Message{{Role: RoleUser, Content: "Hi"}},
		MaxTokens: &maxTokens,
	})
	if err != nil {
		t.Fatalf("GenerateStream should recover after renaming the token field: %v", err)
	}

	var content strings.Builder
	for c := range ch {
		content.WriteString(c.Content)
	}
	if content.String() != "Hello" {
		t.Errorf("streamed content = %q, want %q", content.String(), "Hello")
	}
	assertSwapped(t, *bodies)
}

// TestGenerate_DoesNotRetryUnrelated400 确认兜底不会退化成"遇 400 就重试"。
func TestGenerate_DoesNotRetryUnrelated400(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid model"}}`))
	}))
	defer srv.Close()

	maxTokens := 5
	o := &OpenAI{config: &Config{Model: "gpt-4o", BaseURL: srv.URL}, client: srv.Client()}
	if _, err := o.Generate(context.Background(), &GenerateRequest{
		Messages:  []Message{{Role: RoleUser, Content: "Hi"}},
		MaxTokens: &maxTokens,
	}); err == nil {
		t.Fatal("expected the unrelated 400 to surface")
	}
	if calls != 1 {
		t.Errorf("unrelated 400 must not be retried, got %d upstream calls", calls)
	}
}
