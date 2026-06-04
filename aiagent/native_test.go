package aiagent

import (
	"context"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
)

// scriptedNativeLLM is a fake llm.LLM scripted per round with raw stream chunks,
// so tests control native tool-call deltas (including fragmented arguments).
type scriptedNativeLLM struct {
	rounds [][]llm.StreamChunk
	call   int
}

func (s *scriptedNativeLLM) Name() string { return "scripted-native" }

func (s *scriptedNativeLLM) take() []llm.StreamChunk {
	var chunks []llm.StreamChunk
	if s.call < len(s.rounds) {
		chunks = s.rounds[s.call]
	}
	s.call++
	return chunks
}

func (s *scriptedNativeLLM) Generate(ctx context.Context, req *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	resp := &llm.GenerateResponse{}
	for _, c := range s.take() {
		resp.Content += c.Content
		for _, tc := range c.ToolCalls {
			if tc.ID == "" && tc.Name == "" && len(resp.ToolCalls) > 0 {
				resp.ToolCalls[len(resp.ToolCalls)-1].Arguments += tc.Arguments
			} else {
				resp.ToolCalls = append(resp.ToolCalls, tc)
			}
		}
	}
	return resp, nil
}

func (s *scriptedNativeLLM) GenerateStream(ctx context.Context, req *llm.GenerateRequest) (<-chan llm.StreamChunk, error) {
	chunks := s.take()
	out := make(chan llm.StreamChunk, len(chunks)+1)
	for _, c := range chunks {
		out <- c
	}
	out <- llm.StreamChunk{Done: true}
	close(out)
	return out, nil
}

// TestRunNativeLoop_ContractAndTranscript drives one tool round (with OpenAI-style
// fragmented arguments) + a final text round, and asserts the native loop honors
// the established stream contract (same ToolCall/ToolResult shapes as ReAct) and
// emits a STRUCTURED transcript pair.
func TestRunNativeLoop_ContractAndTranscript(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{ // round 0: a tool call whose arguments arrive in two fragments
			{ToolCalls: []llm.ToolCall{{ID: "call_a", Name: "noop_lookup", Arguments: `{"id":`}}},
			{ToolCalls: []llm.ToolCall{{Arguments: `5}`}}},
		},
		{ // round 1: final answer as plain text
			{Content: "已生成提案，确认吗？"},
		},
	}}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 5, Timeout: 30000}, llmClient: fake}

	streamChan := make(chan *StreamChunk, 100)
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "改大盘"}}, nil, &ReActLoopConfig{
		MaxIterations:  5,
		StreamChan:     streamChan,
		EmitTranscript: true,
	})
	close(streamChan)

	if !resp.Success || resp.Content != "已生成提案，确认吗？" {
		t.Fatalf("resp = %+v, want success with final text", resp)
	}
	if len(resp.Steps) != 1 || resp.Steps[0].Action != "noop_lookup" {
		t.Fatalf("steps = %+v, want one tool step", resp.Steps)
	}

	var types []string
	var toolCallChunk, toolResultChunk *StreamChunk
	var transcripts [][]ChatMessage
	sawText := false
	for ch := range streamChan {
		types = append(types, ch.Type)
		switch ch.Type {
		case StreamTypeToolCall:
			toolCallChunk = ch
		case StreamTypeToolResult:
			toolResultChunk = ch
		case StreamTypeTranscript:
			transcripts = append(transcripts, ch.Transcript)
		case StreamTypeText:
			sawText = true
		}
	}

	// Contract: same chunk shapes as the ReAct loop, so router card-extraction
	// and the A2A bridge need zero changes.
	if toolCallChunk == nil || toolCallChunk.Content != "noop_lookup" ||
		toolCallChunk.Metadata["input"] != `{"id":5}` {
		t.Fatalf("ToolCall chunk = %+v, want Content=name + merged input args", toolCallChunk)
	}
	if toolResultChunk == nil || toolResultChunk.Metadata["tool"] != "noop_lookup" ||
		toolResultChunk.Content == "" {
		t.Fatalf("ToolResult chunk = %+v, want Metadata.tool=name + observation content", toolResultChunk)
	}
	if sawText {
		t.Fatalf("native loop must not emit StreamTypeText (ReAct-only channel); got %v", types)
	}

	// Structured transcript: assistant{ToolCalls} + tool{ToolCallID/ToolName}.
	if len(transcripts) != 1 || len(transcripts[0]) != 2 {
		t.Fatalf("transcripts = %+v, want one pair", transcripts)
	}
	asst, tool := transcripts[0][0], transcripts[0][1]
	if asst.Role != "assistant" || len(asst.ToolCalls) != 1 ||
		asst.ToolCalls[0].ID != "call_a" || asst.ToolCalls[0].Arguments != `{"id":5}` {
		t.Fatalf("transcript assistant turn = %+v, want structured merged tool call", asst)
	}
	if tool.Role != llm.RoleTool || tool.ToolCallID != "call_a" || tool.ToolName != "noop_lookup" ||
		!strings.Contains(tool.Content, "not found") {
		t.Fatalf("transcript tool turn = %+v, want structured result turn", tool)
	}
}

func TestResolveToolProtocol(t *testing.T) {
	for in, want := range map[string]string{
		"":        ToolProtocolNative, // 空值 = 默认 native
		"native":  ToolProtocolNative,
		"react":   ToolProtocolReAct, // 只有显式 react 走文本协议降级
		"unknown": ToolProtocolNative,
	} {
		if got := ResolveToolProtocol(in); got != want {
			t.Fatalf("ResolveToolProtocol(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestExecuteReAct_DefaultsToNative：未配置 ToolProtocol（空值）即走原生 loop——
// 证据是工具从原生 tool_calls 被执行（文本协议永远不会执行原生形态的调用）。
func TestExecuteReAct_DefaultsToNative(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "noop_lookup", Arguments: `{}`}}}},
		{{Content: "done"}},
	}}
	a := &Agent{
		cfg:       &AgentConfig{MaxIterations: 3, Timeout: 30000, UserPromptRendered: "改大盘"}, // ToolProtocol 留空
		llmClient: fake,
	}
	resp := a.executeReAct(context.Background(), &AgentRequest{}, &runCtx{})
	if !resp.Success || resp.Content != "done" || len(resp.Steps) != 1 {
		t.Fatalf("resp = %+v, want native execution by default (1 tool step + final)", resp)
	}
}

// TestExecuteReAct_AutoFallbackToText：默认 native 下，端点不支持原生 FC（首轮
// 零 tool_calls、正文是文本形态的工具调用）→ 当轮自动降级为 ReAct 文本协议重跑，
// 而不是把那坨文本当最终答案返回。
func TestExecuteReAct_AutoFallbackToText(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		// 第 1 次调用（native 尝试）：端点无视 tools 参数，把工具调用吐进正文
		{{Content: `{"name":"noop_lookup","arguments":{"id":5}}`}},
		// 第 2 次调用（react 重跑的首轮）：规范 ReAct 最终答案
		{{Content: "Thought: ok\nAction: Final Answer\nAction Input: 答案"}},
	}}
	a := &Agent{
		cfg:       &AgentConfig{MaxIterations: 3, Timeout: 30000, UserPromptRendered: "查一下"}, // ToolProtocol 留空 = native 默认
		llmClient: fake,
	}
	rc := &runCtx{tools: []AgentTool{{Name: "noop_lookup", Type: ToolTypeBuiltin}}} // 有工具表才会触发探测

	resp := a.executeReAct(context.Background(), &AgentRequest{}, rc)
	if !resp.Success {
		t.Fatalf("resp = %+v", resp)
	}
	if strings.TrimSpace(resp.Content) != "答案" {
		t.Fatalf("content = %q, want the ReAct rerun's final answer (not the raw text-form tool call)", resp.Content)
	}
	if fake.call != 2 {
		t.Fatalf("LLM called %d times, want 2 (native attempt + react rerun)", fake.call)
	}
}

// TestExecuteReAct_DispatchesNative proves the single protocol seam: with
// ToolProtocol=native, executeReAct hands the whole run to the native loop
// (which executes tools from native tool_calls — the text loop never would).
func TestExecuteReAct_DispatchesNative(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "noop_lookup", Arguments: `{}`}}}},
		{{Content: "done"}},
	}}
	a := &Agent{
		cfg: &AgentConfig{
			ToolProtocol:       ToolProtocolNative,
			MaxIterations:      3,
			Timeout:            30000,
			UserPromptRendered: "改大盘",
		},
		llmClient: fake,
	}
	resp := a.executeReAct(context.Background(), &AgentRequest{}, &runCtx{})
	if !resp.Success || resp.Content != "done" || len(resp.Steps) != 1 {
		t.Fatalf("resp = %+v, want native execution (1 tool step + final 'done')", resp)
	}
}

// TestRunNativeLoop_ThinkingBlocksPersist：Anthropic 系思考块（含签名）随
// assistant 轮进入 transcript——下一轮回放时 provider 适配层据此回填（开启
// thinking 后工具续轮的协议硬性要求）。
func TestRunNativeLoop_ThinkingBlocksPersist(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{
			{Reasoning: "先读配置"},
			{ThinkingBlock: &llm.ThinkingBlock{Type: "thinking", Thinking: "先读配置", Signature: "sig-1"}},
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "noop_lookup", Arguments: `{}`}}},
		},
		{{Content: "done"}},
	}}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 5, Timeout: 30000}, llmClient: fake}

	streamChan := make(chan *StreamChunk, 100)
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "改大盘"}}, nil, &ReActLoopConfig{
		MaxIterations:  5,
		StreamChan:     streamChan,
		EmitTranscript: true,
	})
	close(streamChan)

	if !resp.Success {
		t.Fatalf("resp = %+v", resp)
	}
	var transcript []ChatMessage
	thinkingStreamed := false
	for ch := range streamChan {
		if ch.Type == StreamTypeTranscript {
			transcript = append(transcript, ch.Transcript...)
		}
		if ch.Type == StreamTypeThinking && ch.Delta == "先读配置" {
			thinkingStreamed = true
		}
	}
	if !thinkingStreamed {
		t.Fatal("reasoning delta must stream to the thinking channel")
	}
	if len(transcript) == 0 || transcript[0].Role != "assistant" {
		t.Fatalf("transcript = %+v", transcript)
	}
	tb := transcript[0].ThinkingBlocks
	if len(tb) != 1 || tb[0].Signature != "sig-1" || tb[0].Thinking != "先读配置" {
		t.Fatalf("thinking blocks must persist on the assistant turn with signature: %+v", tb)
	}
}

// TestRunNativeLoop_Interrupt proves the human-in-the-loop seam (Step 4): when a
// builtin tool returns a *ToolInterrupt, the loop stops immediately, the prompt
// becomes this turn's answer, an interrupt chunk carries the resume payload, and
// NO tool-result/transcript is emitted for the interrupted call.
func TestRunNativeLoop_Interrupt(t *testing.T) {
	RegisterBuiltinTool("test_interrupting_tool", &BuiltinTool{
		Definition: AgentTool{Name: "test_interrupting_tool", Type: ToolTypeBuiltin},
		Handler: func(ctx context.Context, deps *ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
			return "", &ToolInterrupt{
				Kind:       InterruptKindApproval,
				Prompt:     "即将修改仪表盘，确认吗？",
				ResumeArgs: `{"id":5,"proposal_id":"dbprop_x","confirmed":true}`,
			}
		},
	})

	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "test_interrupting_tool", Arguments: `{}`}}}},
	}}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 5, Timeout: 30000}, llmClient: fake}

	streamChan := make(chan *StreamChunk, 100)
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "改大盘"}}, nil, &ReActLoopConfig{
		MaxIterations:  5,
		StreamChan:     streamChan,
		EmitTranscript: true,
	})
	close(streamChan)

	if !resp.Success || resp.Content != "即将修改仪表盘，确认吗？" {
		t.Fatalf("resp = %+v, want prompt as the turn's answer", resp)
	}

	var interruptChunk *StreamChunk
	for ch := range streamChan {
		switch ch.Type {
		case StreamTypeInterrupt:
			interruptChunk = ch
		case StreamTypeToolResult, StreamTypeTranscript:
			t.Fatalf("interrupted call must not emit %s", ch.Type)
		}
	}
	if interruptChunk == nil {
		t.Fatal("missing interrupt chunk")
	}
	if interruptChunk.Metadata["tool"] != "test_interrupting_tool" ||
		interruptChunk.Metadata["kind"] != InterruptKindApproval ||
		interruptChunk.Metadata["resume_args"] != `{"id":5,"proposal_id":"dbprop_x","confirmed":true}` {
		t.Fatalf("interrupt chunk metadata = %+v", interruptChunk.Metadata)
	}
}

func TestBuildNativeToolDefs(t *testing.T) {
	defs := buildNativeToolDefs([]AgentTool{{
		Name:        "update_dashboard",
		Description: "two-phase update",
		Parameters: []ToolParameter{
			{Name: "id", Type: "int", Description: "dashboard id", Required: true},
			{Name: "confirmed", Type: "boolean", Description: "confirm flag"},
			{Name: "panels", Type: "", Description: "panels json"},
		},
	}})
	if len(defs) != 1 || defs[0].Name != "update_dashboard" {
		t.Fatalf("defs = %+v", defs)
	}
	params := defs[0].Parameters
	if params["type"] != "object" {
		t.Fatalf("schema type = %v", params["type"])
	}
	props := params["properties"].(map[string]interface{})
	if props["id"].(map[string]interface{})["type"] != "integer" {
		t.Fatalf("id type = %+v, want int→integer normalization", props["id"])
	}
	if props["confirmed"].(map[string]interface{})["type"] != "boolean" {
		t.Fatalf("confirmed type = %+v", props["confirmed"])
	}
	if props["panels"].(map[string]interface{})["type"] != "string" {
		t.Fatalf("panels type = %+v, want empty→string fallback", props["panels"])
	}
	req := params["required"].([]string)
	if len(req) != 1 || req[0] != "id" {
		t.Fatalf("required = %v", req)
	}
}

func TestNormalizeJSONSchemaType(t *testing.T) {
	cases := map[string]string{
		"int": "integer", "Integer": "integer", "int64": "integer",
		"bool": "boolean", "boolean": "boolean",
		"float": "number", "number": "number",
		"object": "object", "array": "array", "list": "array",
		"": "string", "string": "string", "whatever": "string",
	}
	for in, want := range cases {
		if got := normalizeJSONSchemaType(in); got != want {
			t.Fatalf("normalizeJSONSchemaType(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRunNativeLoop_ContentChannelExclusive：字段独占路由的回归测试——推理走
// StreamTypeThinking、正文走 StreamTypeContent，互斥不双发。
// 这是"思考面板与回答内容完全相同"问题的防回归锚点。
func TestRunNativeLoop_ContentChannelExclusive(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{
			{Reasoning: "想一想"},
			{Content: "答案"},
			{Content: "正文"},
		},
	}}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 3, Timeout: 30000}, llmClient: fake}

	streamChan := make(chan *StreamChunk, 100)
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "问"}}, nil, &ReActLoopConfig{
		MaxIterations: 3,
		StreamChan:    streamChan,
	})
	close(streamChan)

	if !resp.Success || resp.Content != "答案正文" {
		t.Fatalf("resp = %+v", resp)
	}
	if !resp.contentStreamed {
		t.Fatal("contentStreamed must be set in streaming mode so the Done chunk gets the content_streamed mark")
	}
	var thinkingDeltas, contentDeltas string
	for ch := range streamChan {
		switch ch.Type {
		case StreamTypeThinking:
			thinkingDeltas += ch.Delta
		case StreamTypeContent:
			contentDeltas += ch.Delta
		}
	}
	if thinkingDeltas != "想一想" {
		t.Fatalf("thinking channel = %q, want reasoning only (no content duplication)", thinkingDeltas)
	}
	if contentDeltas != "答案正文" {
		t.Fatalf("content channel = %q, want the body streamed delta by delta", contentDeltas)
	}
}

// TestRunNativeLoop_FallbackHoldsContent：首轮疑似文本形态工具调用的正文被押住，
// fallback 触发时绝不进 content 通道——ReAct 重跑的答案才是唯一正文，不重复。
func TestRunNativeLoop_FallbackHoldsContent(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{
			{Content: `{"name":"noop_lookup",`},
			{Content: `"arguments":{"id":5}}`},
		},
	}}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 3, Timeout: 30000}, llmClient: fake}

	tools := []AgentTool{{Name: "noop_lookup", Type: ToolTypeBuiltin}}
	streamChan := make(chan *StreamChunk, 100)
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "查"}}, buildNativeToolDefs(tools), &ReActLoopConfig{
		MaxIterations: 3,
		StreamChan:    streamChan,
		Tools:         tools,
	})
	close(streamChan)

	if !resp.fallbackToReAct {
		t.Fatalf("resp = %+v, want fallbackToReAct", resp)
	}
	for ch := range streamChan {
		if ch.Type == StreamTypeContent {
			t.Fatalf("suspected text-form tool call leaked to content channel: %q", ch.Delta)
		}
	}
}

// TestRunNativeLoop_HeldInnocentContentFlushes：首轮带工具、正文以普通文字开头
// → 押后判定立即证伪，正文照常进 content 通道（整段补发 + 后续逐 token）。
func TestRunNativeLoop_HeldInnocentContentFlushes(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{
			{Content: "好的，"},
			{Content: "这是答案"},
		},
	}}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 3, Timeout: 30000}, llmClient: fake}

	tools := []AgentTool{{Name: "noop_lookup", Type: ToolTypeBuiltin}}
	streamChan := make(chan *StreamChunk, 100)
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "问"}}, buildNativeToolDefs(tools), &ReActLoopConfig{
		MaxIterations: 3,
		StreamChan:    streamChan,
		Tools:         tools,
	})
	close(streamChan)

	if !resp.Success || resp.Content != "好的，这是答案" {
		t.Fatalf("resp = %+v", resp)
	}
	var contentDeltas string
	for ch := range streamChan {
		if ch.Type == StreamTypeContent {
			contentDeltas += ch.Delta
		}
	}
	if contentDeltas != "好的，这是答案" {
		t.Fatalf("content channel = %q, want full body", contentDeltas)
	}
}

// TestLooksLikeNativeFallback：降级判定与正文押后同口径——只认"裸 JSON/标签/
// 围栏起头"的文本形态工具调用；普通文字起头（即使中段嵌了工具 JSON）不降级。
func TestLooksLikeNativeFallback(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{`{"name":"t","arguments":{}}`, true},
		{"  <tool_call>{\"name\":\"t\"}</tool_call>", true},
		{"```json\n{\"name\":\"t\",\"arguments\":{}}\n```", true},
		{`[{"name":"t","arguments":{}}]`, true},
		{"正常的 markdown 答案", false},
		{`先解释一下，然后 {"name":"t","arguments":{}}`, false},
		{"", false},
	}
	for _, c := range cases {
		if got := looksLikeNativeFallback(c.in); got != c.want {
			t.Fatalf("looksLikeNativeFallback(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
