package llm

import (
	"encoding/json"
	"reflect"
	"testing"
)

// nativeToolMsgs is a canonical structured-tool-turn conversation:
// user ask → assistant issues two parallel tool calls (one with valid
// JSON args, one with junk) → two tool results → assistant final answer.
func nativeToolMsgs() []Message {
	return []Message{
		{Role: RoleSystem, Content: "sys"},
		{Role: RoleUser, Content: "改大盘"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "call_1", Name: "get_dashboard_detail", Arguments: `{"id":5}`},
			{ID: "call_2", Name: "list_dashboards", Arguments: `not-json`},
		}},
		{Role: RoleTool, ToolCallID: "call_1", ToolName: "get_dashboard_detail", Content: `{"panels":[]}`},
		{Role: RoleTool, ToolCallID: "call_2", ToolName: "list_dashboards", Content: "plain text"},
		{Role: RoleAssistant, Content: "done"},
	}
}

func TestOpenAIConvertRequest_ToolTurns(t *testing.T) {
	o := &OpenAI{config: &Config{Model: "m"}}
	req := o.convertRequest(&GenerateRequest{Messages: nativeToolMsgs()})

	if len(req.Messages) != 6 {
		t.Fatalf("messages = %d, want 6 (OpenAI keeps turns 1:1)", len(req.Messages))
	}
	asst := req.Messages[2]
	if len(asst.ToolCalls) != 2 || asst.ToolCalls[0].ID != "call_1" ||
		asst.ToolCalls[0].Type != "function" ||
		asst.ToolCalls[0].Function.Name != "get_dashboard_detail" ||
		asst.ToolCalls[0].Function.Arguments != `{"id":5}` {
		t.Fatalf("assistant tool_calls malformed: %+v", asst.ToolCalls)
	}
	tool := req.Messages[3]
	if tool.Role != "tool" || tool.ToolCallID != "call_1" || tool.Content != `{"panels":[]}` {
		t.Fatalf("tool message malformed: %+v", tool)
	}
}

func TestClaudeConvertRequest_ToolTurns(t *testing.T) {
	c := &Claude{config: &Config{Model: "m"}}
	req := c.convertRequest(&GenerateRequest{Messages: nativeToolMsgs()})

	if req.System != "sys" {
		t.Fatalf("system = %q", req.System)
	}
	// Strict alternation: user / assistant(tool_use) / user(merged tool_results) / assistant
	roles := make([]string, 0, len(req.Messages))
	for _, m := range req.Messages {
		roles = append(roles, m.Role)
	}
	if !reflect.DeepEqual(roles, []string{"user", "assistant", "user", "assistant"}) {
		t.Fatalf("roles = %v, want strict alternation with merged tool results", roles)
	}

	asst := req.Messages[1]
	// Empty-content tool-call turn must NOT carry an empty text block.
	if len(asst.Content) != 2 || asst.Content[0].Type != "tool_use" || asst.Content[1].Type != "tool_use" {
		t.Fatalf("assistant blocks = %+v, want exactly two tool_use blocks", asst.Content)
	}
	if asst.Content[0].ID != "call_1" || asst.Content[0].Name != "get_dashboard_detail" {
		t.Fatalf("tool_use[0] = %+v", asst.Content[0])
	}
	if in, ok := asst.Content[0].Input.(map[string]interface{}); !ok || in["id"] != float64(5) {
		t.Fatalf("tool_use[0].input = %#v, want parsed object {id:5}", asst.Content[0].Input)
	}
	// Junk arguments degrade to {"input": raw}, never an invalid non-object.
	if in, ok := asst.Content[1].Input.(map[string]interface{}); !ok || in["input"] != "not-json" {
		t.Fatalf("tool_use[1].input = %#v, want {input: not-json}", asst.Content[1].Input)
	}

	results := req.Messages[2]
	if len(results.Content) != 2 ||
		results.Content[0].Type != "tool_result" || results.Content[0].ToolUseID != "call_1" ||
		results.Content[1].Type != "tool_result" || results.Content[1].ToolUseID != "call_2" {
		t.Fatalf("merged tool_result turn malformed: %+v", results.Content)
	}
}

func TestGeminiConvertRequest_ToolTurns(t *testing.T) {
	g := &Gemini{config: &Config{Model: "m"}}
	req := g.convertRequest(&GenerateRequest{Messages: nativeToolMsgs()})

	if req.SystemInstruction == nil || req.SystemInstruction.Parts[0].Text != "sys" {
		t.Fatalf("systemInstruction = %+v", req.SystemInstruction)
	}
	roles := make([]string, 0, len(req.Contents))
	for _, c := range req.Contents {
		roles = append(roles, c.Role)
	}
	if !reflect.DeepEqual(roles, []string{"user", "model", "user", "model"}) {
		t.Fatalf("roles = %v, want user/model alternation with merged functionResponses", roles)
	}

	model := req.Contents[1]
	if len(model.Parts) != 2 || model.Parts[0].FunctionCall == nil || model.Parts[1].FunctionCall == nil {
		t.Fatalf("model parts = %+v, want exactly two functionCall parts (no empty text part)", model.Parts)
	}
	if model.Parts[0].FunctionCall.Name != "get_dashboard_detail" ||
		model.Parts[0].FunctionCall.Args["id"] != float64(5) {
		t.Fatalf("functionCall[0] = %+v", model.Parts[0].FunctionCall)
	}
	if model.Parts[1].FunctionCall.Args["input"] != "not-json" {
		t.Fatalf("functionCall[1].args = %+v, want junk degraded to {input: raw}", model.Parts[1].FunctionCall.Args)
	}

	frTurn := req.Contents[2]
	if len(frTurn.Parts) != 2 || frTurn.Parts[0].FunctionResponse == nil || frTurn.Parts[1].FunctionResponse == nil {
		t.Fatalf("functionResponse turn = %+v, want two merged parts", frTurn.Parts)
	}
	// Gemini matches by NAME (it has no call ids).
	if frTurn.Parts[0].FunctionResponse.Name != "get_dashboard_detail" {
		t.Fatalf("functionResponse[0].name = %q", frTurn.Parts[0].FunctionResponse.Name)
	}
	// Object-shaped response: JSON object kept, plain text wrapped as {result: raw}.
	if _, ok := frTurn.Parts[0].FunctionResponse.Response["panels"]; !ok {
		t.Fatalf("functionResponse[0].response = %+v, want parsed object", frTurn.Parts[0].FunctionResponse.Response)
	}
	if frTurn.Parts[1].FunctionResponse.Response["result"] != "plain text" {
		t.Fatalf("functionResponse[1].response = %+v, want {result: plain text}", frTurn.Parts[1].FunctionResponse.Response)
	}
}

// Plain-text conversations (no tool turns) must be wire-identical to the
// pre-Step-2 behavior across all three providers.
func TestConvertRequest_PlainTextUnchanged(t *testing.T) {
	msgs := []Message{
		{Role: RoleSystem, Content: "sys"},
		{Role: RoleUser, Content: "hi"},
		{Role: RoleAssistant, Content: "hello"},
	}

	o := &OpenAI{config: &Config{Model: "m"}}
	oReq := o.convertRequest(&GenerateRequest{Messages: msgs})
	if len(oReq.Messages) != 3 || oReq.Messages[2].Content != "hello" || oReq.Messages[2].ToolCalls != nil {
		t.Fatalf("openai plain-text drifted: %+v", oReq.Messages)
	}

	c := &Claude{config: &Config{Model: "m"}}
	cReq := c.convertRequest(&GenerateRequest{Messages: msgs})
	if len(cReq.Messages) != 2 || len(cReq.Messages[1].Content) != 1 ||
		cReq.Messages[1].Content[0].Type != "text" || cReq.Messages[1].Content[0].Text != "hello" {
		t.Fatalf("claude plain-text drifted: %+v", cReq.Messages)
	}

	g := &Gemini{config: &Config{Model: "m"}}
	gReq := g.convertRequest(&GenerateRequest{Messages: msgs})
	if len(gReq.Contents) != 2 || gReq.Contents[1].Role != "model" ||
		len(gReq.Contents[1].Parts) != 1 || gReq.Contents[1].Parts[0].Text != "hello" {
		t.Fatalf("gemini plain-text drifted: %+v", gReq.Contents)
	}
}

// TestClaudeConvertRequest_ThinkingReplay：Anthropic 扩展思考的协议正确性——
// assistant 轮的思考块（含签名/redacted）必须回填，且排在 text/tool_use 之前。
func TestClaudeConvertRequest_ThinkingReplay(t *testing.T) {
	c := &Claude{config: &Config{Model: "m"}}
	req := c.convertRequest(&GenerateRequest{Messages: []Message{
		{Role: RoleUser, Content: "改大盘"},
		{Role: RoleAssistant,
			ThinkingBlocks: []ThinkingBlock{
				{Type: "thinking", Thinking: "先读配置", Signature: "sig-abc"},
				{Type: "redacted_thinking", Data: "opaque-bytes"},
			},
			ToolCalls: []ToolCall{{ID: "c1", Name: "get_dashboard_detail", Arguments: `{"id":5}`}},
		},
		{Role: RoleTool, ToolCallID: "c1", ToolName: "get_dashboard_detail", Content: "{}"},
	}})

	asst := req.Messages[1]
	if len(asst.Content) != 3 {
		t.Fatalf("blocks = %d, want 3 (thinking + redacted + tool_use, no empty text)", len(asst.Content))
	}
	if asst.Content[0].Type != "thinking" || asst.Content[0].Thinking != "先读配置" || asst.Content[0].Signature != "sig-abc" {
		t.Fatalf("thinking block must lead with signature intact: %+v", asst.Content[0])
	}
	if asst.Content[1].Type != "redacted_thinking" || asst.Content[1].Data != "opaque-bytes" {
		t.Fatalf("redacted block must pass through: %+v", asst.Content[1])
	}
	if asst.Content[2].Type != "tool_use" {
		t.Fatalf("tool_use must follow thinking blocks: %+v", asst.Content[2])
	}
}

// TestClaudeConvertResponse_CapturesThinking：非流式响应里思考块（含签名）被捕获，
// 思考文本同时进入 ReasoningContent。
func TestClaudeConvertResponse_CapturesThinking(t *testing.T) {
	c := &Claude{config: &Config{Model: "m"}}
	resp := c.convertResponse(&claudeResponse{
		Content: []claudeContentBlock{
			{Type: "thinking", Thinking: "推理过程", Signature: "sig-1"},
			{Type: "tool_use", ID: "c1", Name: "get_x", Input: map[string]interface{}{"id": float64(1)}},
		},
	})
	if len(resp.ThinkingBlocks) != 1 || resp.ThinkingBlocks[0].Signature != "sig-1" {
		t.Fatalf("thinking blocks = %+v", resp.ThinkingBlocks)
	}
	if resp.ReasoningContent != "推理过程" {
		t.Fatalf("reasoning = %q", resp.ReasoningContent)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v", resp.ToolCalls)
	}
}

// TestGeminiThoughtSignatureRoundTrip：functionCall part 上的 thoughtSignature
// 捕获→回传（Gemini 3 续轮硬性要求）；thought 文本归 reasoning 不算答案。
func TestGeminiThoughtSignatureRoundTrip(t *testing.T) {
	g := &Gemini{config: &Config{Model: "m"}}

	// Candidates 是匿名结构体，经 JSON 构造（与真实 API 响应同形态）。
	var gResp geminiResponse
	if err := json.Unmarshal([]byte(`{"candidates":[{"content":{"role":"model","parts":[
		{"text":"思考摘要","thought":true},
		{"functionCall":{"name":"get_x","args":{"id":1}},"thoughtSignature":"tsig-1"},
		{"text":"正文"}
	]}}]}`), &gResp); err != nil {
		t.Fatal(err)
	}
	resp := g.convertResponse(&gResp)
	if resp.ReasoningContent != "思考摘要" || resp.Content != "正文" {
		t.Fatalf("thought text must go to reasoning: content=%q reasoning=%q", resp.Content, resp.ReasoningContent)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ThoughtSignature != "tsig-1" {
		t.Fatalf("thoughtSignature not captured: %+v", resp.ToolCalls)
	}

	// 回传：带签名的 ToolCall 重建请求时必须附回 functionCall part。
	req := g.convertRequest(&GenerateRequest{Messages: []Message{
		{Role: RoleUser, Content: "q"},
		{Role: RoleAssistant, ToolCalls: resp.ToolCalls},
		{Role: RoleTool, ToolCallID: "x", ToolName: "get_x", Content: "{}"},
	}})
	model := req.Contents[1]
	if len(model.Parts) != 1 || model.Parts[0].FunctionCall == nil || model.Parts[0].ThoughtSignature != "tsig-1" {
		t.Fatalf("thoughtSignature not replayed: %+v", model.Parts)
	}
}

func TestParseToolJSONObject(t *testing.T) {
	if m := parseToolJSONObject(`{"a":1}`, "input"); m["a"] != float64(1) {
		t.Fatalf("object: %+v", m)
	}
	if m := parseToolJSONObject(``, "input"); len(m) != 0 {
		t.Fatalf("empty: %+v", m)
	}
	if m := parseToolJSONObject(`[1,2]`, "input"); m["input"] != "[1,2]" {
		t.Fatalf("array degrades: %+v", m)
	}
	if m := parseToolJSONObject(`oops`, "result"); m["result"] != "oops" {
		t.Fatalf("junk degrades: %+v", m)
	}
}
