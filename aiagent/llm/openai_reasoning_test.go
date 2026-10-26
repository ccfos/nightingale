package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newOpenAIFixtureServer(t *testing.T, body string, sse bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sse {
			w.Header().Set("Content-Type", "text/event-stream")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestOpenAIReasoningAlias 覆盖 vLLM 的 reasoning 字段名（与 reasoning_content
// 同义）：非流式响应不能丢推理（issue #3440）。
func TestOpenAIReasoningAlias(t *testing.T) {
	cases := map[string]string{
		"reasoning":         `{"choices":[{"message":{"role":"assistant","content":"answer","reasoning":"diagnostic thought"},"finish_reason":"stop"}]}`,
		"reasoning_content": `{"choices":[{"message":{"role":"assistant","content":"answer","reasoning_content":"diagnostic thought"},"finish_reason":"stop"}]}`,
		// 两个字段同时出现只取一份，不拼接重复
		"both": `{"choices":[{"message":{"role":"assistant","content":"answer","reasoning_content":"diagnostic thought","reasoning":"diagnostic thought"},"finish_reason":"stop"}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newOpenAIFixtureServer(t, body, false)
			client, err := NewOpenAI(&Config{Model: "diagnostic", BaseURL: srv.URL + "/v1/chat/completions"}, srv.Client())
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.Generate(context.Background(), &GenerateRequest{
				Messages: []Message{{Role: RoleUser, Content: "diagnostic"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if resp.ReasoningContent != "diagnostic thought" || resp.Content != "answer" {
				t.Fatalf("reasoning=%q content=%q", resp.ReasoningContent, resp.Content)
			}
		})
	}
}

// TestOpenAIStream_ReasoningAlias 流式 delta.reasoning 同样进入 Reasoning 通道。
func TestOpenAIStream_ReasoningAlias(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning":"diagnostic "},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"reasoning_content":"thought","reasoning":"thought"},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"content":"answer"},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"

	var reasoning, content string
	for _, c := range runOpenAIStream(t, sse) {
		reasoning += c.Reasoning
		content += c.Content
	}
	if reasoning != "diagnostic thought" || content != "answer" {
		t.Fatalf("reasoning=%q content=%q", reasoning, content)
	}
}

// TestConvertRequest_ReasoningPassback 推理文本只在 assistant 工具调用轮回填，
// 字段名沿用端点响应所用的那个；纯文本 assistant 轮不回填，两个字段从不同时发。
func TestConvertRequest_ReasoningPassback(t *testing.T) {
	o := &OpenAI{config: &Config{Model: "deepseek-flash"}}
	req := o.convertRequest(&GenerateRequest{Messages: []Message{
		{Role: RoleUser, Content: "q1"},
		{Role: RoleAssistant, Content: "a1", ReasoningContent: "old final reasoning"},
		{Role: RoleUser, Content: "q2"},
		{Role: RoleAssistant, ReasoningContent: "need metrics", ToolCalls: []ToolCall{{ID: "c1", Name: "query", Arguments: "{}"}}},
		{Role: RoleTool, ToolCallID: "c1", ToolName: "query", Content: "ok"},
		{Role: RoleAssistant, ReasoningContent: "vllm thought", ReasoningField: ReasoningFieldAlias, ToolCalls: []ToolCall{{ID: "c2", Name: "query", Arguments: "{}"}}},
		{Role: RoleTool, ToolCallID: "c2", ToolName: "query", Content: "ok"},
	}})

	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	m := body.Messages
	if _, ok := m[1]["reasoning_content"]; ok {
		t.Errorf("plain assistant turn must not carry reasoning: %v", m[1])
	}
	// 默认（DeepSeek 官方等）走 reasoning_content
	if m[3]["reasoning_content"] != "need metrics" || m[3]["reasoning"] != nil {
		t.Errorf("default passback must use reasoning_content only: %v", m[3])
	}
	// 端点用 reasoning（vLLM）时原样回 reasoning
	if m[5]["reasoning"] != "vllm thought" || m[5]["reasoning_content"] != nil {
		t.Errorf("alias passback must use reasoning only: %v", m[5])
	}
}

// TestOpenAIReasoningField 响应带出端点所用的推理字段名，供回填沿用。
func TestOpenAIReasoningField(t *testing.T) {
	for body, want := range map[string]string{
		`{"choices":[{"message":{"role":"assistant","content":"a","reasoning":"r"},"finish_reason":"stop"}]}`:         ReasoningFieldAlias,
		`{"choices":[{"message":{"role":"assistant","content":"a","reasoning_content":"r"},"finish_reason":"stop"}]}`: ReasoningFieldContent,
	} {
		srv := newOpenAIFixtureServer(t, body, false)
		client, _ := NewOpenAI(&Config{Model: "m", BaseURL: srv.URL + "/v1/chat/completions"}, srv.Client())
		resp, err := client.Generate(context.Background(), &GenerateRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}})
		if err != nil || resp.ReasoningField != want {
			t.Fatalf("field=%q want %q err=%v", resp.ReasoningField, want, err)
		}
	}
	var fields []string
	for _, c := range runOpenAIStream(t, "data: {\"choices\":[{\"delta\":{\"reasoning\":\"r\"}}]}\n\ndata: [DONE]\n\n") {
		if c.Reasoning != "" {
			fields = append(fields, c.ReasoningField)
		}
	}
	if len(fields) != 1 || fields[0] != ReasoningFieldAlias {
		t.Fatalf("stream reasoning field = %v", fields)
	}
}
