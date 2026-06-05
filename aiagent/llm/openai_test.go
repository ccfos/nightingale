package llm

import (
	"context"
	"io"
	"net/http"
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
