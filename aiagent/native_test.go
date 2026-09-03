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
		resp.ToolCalls = append(resp.ToolCalls, c.ToolCalls...)
		if c.FinishReason != "" {
			resp.FinishReason = c.FinishReason
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

// TestRunNativeLoop_ContractAndTranscript drives one tool round + a final text
// round, and asserts the native loop honors the established stream contract
// (ToolCall/ToolResult chunk shapes) and emits a STRUCTURED transcript pair.
// Provider 在流内聚合分片后整块抛出（见 llm/openai.go 的 aggregator），
// 所以这里 mock 直接给完整调用。
func TestRunNativeLoop_ContractAndTranscript(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{ // round 0: a complete tool call (provider-side aggregation already done)
			{ToolCalls: []llm.ToolCall{{ID: "call_a", Name: "noop_lookup", Arguments: `{"id":5}`}}},
		},
		{ // round 1: final answer as plain text
			{Content: "已生成提案，确认吗？"},
		},
	}}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 5, Timeout: 30000}, llmClient: fake}

	streamChan := make(chan *StreamChunk, 100)
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "改大盘"}}, nil, &ToolLoopConfig{
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

	var toolCallChunk, toolResultChunk *StreamChunk
	var transcripts [][]ChatMessage
	for ch := range streamChan {
		switch ch.Type {
		case StreamTypeToolCall:
			toolCallChunk = ch
		case StreamTypeToolResult:
			toolResultChunk = ch
		case StreamTypeTranscript:
			transcripts = append(transcripts, ch.Transcript)
		}
	}

	// Contract: the chunk shapes router card-extraction and the A2A bridge
	// rely on.
	if toolCallChunk == nil || toolCallChunk.Content != "noop_lookup" ||
		toolCallChunk.Metadata["input"] != `{"id":5}` {
		t.Fatalf("ToolCall chunk = %+v, want Content=name + merged input args", toolCallChunk)
	}
	if toolResultChunk == nil || toolResultChunk.Metadata["tool"] != "noop_lookup" ||
		toolResultChunk.Content == "" {
		t.Fatalf("ToolResult chunk = %+v, want Metadata.tool=name + observation content", toolResultChunk)
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

// TestExecuteNative_EndToEnd：executeNative 端到端（非流式）——组装系统提示词
// 与历史后，工具从原生 tool_calls 被执行，最终文本轮收敛为答案。
func TestExecuteNative_EndToEnd(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "noop_lookup", Arguments: `{}`}}}},
		{{Content: "done"}},
	}}
	a := &Agent{
		cfg:       &AgentConfig{MaxIterations: 3, Timeout: 30000, UserPromptRendered: "改大盘"},
		llmClient: fake,
	}
	resp := a.executeNative(context.Background(), &AgentRequest{}, &runCtx{})
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
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "改大盘"}}, nil, &ToolLoopConfig{
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
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "改大盘"}}, nil, &ToolLoopConfig{
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
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "问"}}, nil, &ToolLoopConfig{
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

// TestRunNativeLoop_ContentStreamsWithToolsOffered：首轮带工具、模型直接出正文
// → 正文不被押住、不丢失，逐 token 进 content 通道。
func TestRunNativeLoop_ContentStreamsWithToolsOffered(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{
			{Content: "好的，"},
			{Content: "这是答案"},
		},
	}}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 3, Timeout: 30000}, llmClient: fake}

	tools := []AgentTool{{Name: "noop_lookup", Type: ToolTypeBuiltin}}
	streamChan := make(chan *StreamChunk, 100)
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "问"}}, buildNativeToolDefs(tools), &ToolLoopConfig{
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

// TestExecuteNativeWithDone_MaxIterationPartialGoesDone：max-iteration 截断且
// 带部分产出时走 Done 而非 Error——路由层把 Done.Content 当唯一正文推流并
// 持久化，用户能看到已有分析。
func TestExecuteNativeWithDone_MaxIterationPartialGoesDone(t *testing.T) {
	// 每轮都发工具调用，永不收敛 → 必然打到 MaxIterations
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{{Content: "查一下", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "noop_lookup", Arguments: `{}`}}}},
		{{Content: "再查", ToolCalls: []llm.ToolCall{{ID: "c2", Name: "noop_lookup", Arguments: `{}`}}}},
	}}
	a := &Agent{
		cfg:       &AgentConfig{MaxIterations: 2, Timeout: 30000, UserPromptRendered: "排查问题"},
		llmClient: fake,
	}

	streamChan := make(chan *StreamChunk, 100)
	a.executeNativeWithDone(context.Background(), &AgentRequest{StreamChan: streamChan}, &runCtx{})
	close(streamChan)

	var doneChunk *StreamChunk
	for ch := range streamChan {
		switch ch.Type {
		case StreamTypeError:
			t.Fatalf("max-iteration partial must not surface as Error chunk: %+v", ch)
		case StreamTypeDone:
			doneChunk = ch
		}
	}
	if doneChunk == nil || !strings.Contains(doneChunk.Content, "Analysis incomplete") {
		t.Fatalf("Done chunk = %+v, want partial content from ExtractPartialResult", doneChunk)
	}
	if streamed, _ := doneChunk.Metadata["content_streamed"].(bool); streamed {
		t.Fatalf("partial body never streamed as final answer, content_streamed must be unset")
	}
}

// TestRunNativeLoop_TruncatedFinishAppendsNote：finish_reason 表明输出被长度
// 上限截断时，正文尾部追加可见提示（流式同步推一条 content 增量），
// 不把半截回答伪装成完整答案。
func TestRunNativeLoop_TruncatedFinishAppendsNote(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{{Content: "前半截回答", FinishReason: "MAX_TOKENS"}},
	}}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 3, Timeout: 30000}, llmClient: fake}

	streamChan := make(chan *StreamChunk, 100)
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "问"}}, nil, &ToolLoopConfig{
		MaxIterations: 3,
		StreamChan:    streamChan,
	})
	close(streamChan)

	if !resp.Success || !strings.HasPrefix(resp.Content, "前半截回答") || !strings.Contains(resp.Content, "截断") {
		t.Fatalf("resp = %+v, want partial content with truncation note", resp)
	}
	var contentDeltas string
	for ch := range streamChan {
		if ch.Type == StreamTypeContent {
			contentDeltas += ch.Delta
		}
	}
	if !strings.Contains(contentDeltas, "截断") {
		t.Fatalf("content channel = %q, truncation note must stream too", contentDeltas)
	}
}

// TestRunNativeLoop_BlockedFinishFails：被安全策略拦截（SAFETY/content_filter）
// 的输出不可信，判错而非伪装成正常回答。
func TestRunNativeLoop_BlockedFinishFails(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{{Content: "被拦截的半截", FinishReason: "SAFETY"}},
	}}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 3, Timeout: 30000}, llmClient: fake}

	streamChan := make(chan *StreamChunk, 100)
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "问"}}, nil, &ToolLoopConfig{
		MaxIterations: 3,
		StreamChan:    streamChan,
	})
	close(streamChan)

	if resp.Success || !strings.Contains(resp.Error, "SAFETY") {
		t.Fatalf("resp = %+v, want failure carrying finish_reason", resp)
	}
}

// TestRunNativeLoop_InterruptAfterExecutedWrite：同一 assistant 轮"先行工具已
// 执行 + 后续工具中断"时，已执行部分必须配对持久化进 transcript（assistant 轮
// 裁剪到有结果的调用 + 对应结果轮）——否则下一轮模型对已落库的副作用失忆、
// 可能重复执行；同时被中断的调用不得留下孤儿 tool_use（回放链路没有配对修复，
// provider 会整单拒绝）。
func TestRunNativeLoop_InterruptAfterExecutedWrite(t *testing.T) {
	RegisterBuiltinTool("test_interrupt_after_write", &BuiltinTool{
		Definition: AgentTool{Name: "test_interrupt_after_write", Type: ToolTypeBuiltin},
		Handler: func(ctx context.Context, deps *ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
			return "", &ToolInterrupt{
				Kind:       InterruptKindApproval,
				Prompt:     "确认修改吗？",
				ResumeArgs: `{"confirmed":true}`,
			}
		},
	})

	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{{ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "noop_lookup", Arguments: `{}`},                // 先行工具：正常执行出结果
			{ID: "c2", Name: "test_interrupt_after_write", Arguments: `{}`}, // 后续工具：中断
		}}},
	}}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 5, Timeout: 30000}, llmClient: fake}

	streamChan := make(chan *StreamChunk, 100)
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "改"}}, nil, &ToolLoopConfig{
		MaxIterations:  5,
		StreamChan:     streamChan,
		EmitTranscript: true,
	})
	close(streamChan)

	if !resp.Success || resp.Content != "确认修改吗？" {
		t.Fatalf("resp = %+v, want interrupt prompt as answer", resp)
	}

	var transcript []ChatMessage
	interruptSeen := false
	for ch := range streamChan {
		switch ch.Type {
		case StreamTypeTranscript:
			transcript = append(transcript, ch.Transcript...)
		case StreamTypeInterrupt:
			interruptSeen = true
		}
	}
	if !interruptSeen {
		t.Fatal("interrupt chunk must still be emitted")
	}
	if len(transcript) != 2 {
		t.Fatalf("transcript = %+v, want assistant + 1 executed tool result", transcript)
	}
	asst, toolTurn := transcript[0], transcript[1]
	if asst.Role != "assistant" || len(asst.ToolCalls) != 1 || asst.ToolCalls[0].ID != "c1" {
		t.Fatalf("assistant turn = %+v, want ToolCalls trimmed to the executed call only (no orphan tool_use)", asst)
	}
	if toolTurn.Role != llm.RoleTool || toolTurn.ToolCallID != "c1" {
		t.Fatalf("tool turn = %+v, want paired result for the executed call", toolTurn)
	}
}
