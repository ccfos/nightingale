package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
)

// scriptedNativeLLM is a fake llm.LLM scripted per round with raw stream chunks,
// so tests control native tool-call deltas (including fragmented arguments).
type scriptedNativeLLM struct {
	rounds [][]llm.StreamChunk
	call   int
	reqs   []*llm.GenerateRequest // 每轮收到的请求，供断言回填内容
}

func (s *scriptedNativeLLM) Name() string { return "scripted-native" }

func (s *scriptedNativeLLM) take(req *llm.GenerateRequest) []llm.StreamChunk {
	s.reqs = append(s.reqs, req)
	var chunks []llm.StreamChunk
	if s.call < len(s.rounds) {
		chunks = s.rounds[s.call]
	}
	s.call++
	return chunks
}

func (s *scriptedNativeLLM) Generate(ctx context.Context, req *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	resp := &llm.GenerateResponse{}
	for _, c := range s.take(req) {
		resp.Content += c.Content
		resp.ReasoningContent += c.Reasoning
		resp.ToolCalls = append(resp.ToolCalls, c.ToolCalls...)
		if c.FinishReason != "" {
			resp.FinishReason = c.FinishReason
		}
	}
	return resp, nil
}

func (s *scriptedNativeLLM) GenerateStream(ctx context.Context, req *llm.GenerateRequest) (<-chan llm.StreamChunk, error) {
	chunks := s.take(req)
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

// TestRunNativeLoop_ReasoningPassback 推理文本（OpenAI 兼容端点）随 assistant
// 工具调用轮回填到本回合下一次请求，但不随 transcript 持久化（issue #3440）。
// 流式与非流式两条路径都覆盖。
func TestRunNativeLoop_ReasoningPassback(t *testing.T) {
	for _, streaming := range []bool{true, false} {
		fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
			{
				{Reasoning: "需要先查 "},
				{Reasoning: "CPU 指标"},
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "noop_lookup", Arguments: `{}`}}},
			},
			{{Content: "done"}},
		}}
		a := &Agent{cfg: &AgentConfig{MaxIterations: 5, Timeout: 30000}, llmClient: fake}

		var streamChan chan *StreamChunk
		if streaming {
			streamChan = make(chan *StreamChunk, 100)
		}
		resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "分析大盘"}}, nil, &ToolLoopConfig{
			MaxIterations:  5,
			StreamChan:     streamChan,
			EmitTranscript: true,
		})
		if !resp.Success || len(fake.reqs) != 2 {
			t.Fatalf("streaming=%v resp=%+v reqs=%d", streaming, resp, len(fake.reqs))
		}
		second := fake.reqs[1].Messages
		var asst *llm.Message
		for i := range second {
			if second[i].Role == llm.RoleAssistant && len(second[i].ToolCalls) > 0 {
				asst = &second[i]
			}
		}
		if asst == nil || asst.ReasoningContent != "需要先查 CPU 指标" {
			t.Fatalf("streaming=%v: tool-call turn must carry reasoning into the next request: %+v", streaming, asst)
		}

		if !streaming {
			continue
		}
		close(streamChan)
		for ch := range streamChan {
			if ch.Type != StreamTypeTranscript {
				continue
			}
			raw, _ := json.Marshal(ch.Transcript)
			if strings.Contains(string(raw), "CPU 指标") {
				t.Fatalf("reasoning must not be persisted in transcript: %s", raw)
			}
		}
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
// 带部分产出时走 Done 而非 Error，并打 incomplete 标。收尾调用拿不到正文时
// 退化为确定性进度摘要（含最后一段非空评论），不再出现空白 "Last thought:"。
func TestExecuteNativeWithDone_MaxIterationPartialGoesDone(t *testing.T) {
	// 每轮都发工具调用，永不收敛 → 必然打到 MaxIterations；收尾轮无输出
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{{Content: "查一下", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "noop_lookup", Arguments: `{}`}}}},
		{{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "noop_lookup", Arguments: `{}`}}}},
	}}
	a := &Agent{
		cfg:       &AgentConfig{MaxIterations: 2, Timeout: 30000, UserPromptRendered: "排查问题"},
		llmClient: fake,
	}

	streamChan := make(chan *StreamChunk, 100)
	a.executeNativeWithDone(context.Background(), &AgentRequest{StreamChan: streamChan}, &runCtx{})
	close(streamChan)

	var doneChunk *StreamChunk
	var streamed strings.Builder
	for ch := range streamChan {
		switch ch.Type {
		case StreamTypeError:
			t.Fatalf("max-iteration partial must not surface as Error chunk: %+v", ch)
		case StreamTypeContent:
			streamed.WriteString(ch.Delta)
		case StreamTypeDone:
			doneChunk = ch
		}
	}
	if doneChunk == nil {
		t.Fatal("missing Done chunk")
	}
	for _, want := range []string{"分析未完成", "未能生成最终结论", "noop_lookup×2", "查一下"} {
		if !strings.Contains(doneChunk.Content, want) {
			t.Fatalf("Done content missing %q: %q", want, doneChunk.Content)
		}
	}
	if strings.Contains(doneChunk.Content, "Last thought:") {
		t.Fatalf("blank Last thought must be gone: %q", doneChunk.Content)
	}
	if inc, _ := doneChunk.Metadata["incomplete"].(bool); !inc {
		t.Fatalf("Done must carry incomplete=true: %+v", doneChunk.Metadata)
	}
	// header 与兜底摘要都已经推流：Done 打 content_streamed，路由层不再二次推流
	if s, _ := doneChunk.Metadata["content_streamed"].(bool); !s {
		t.Fatalf("finalize body is streamed, content_streamed must be set: %+v", doneChunk.Metadata)
	}
	if !strings.Contains(streamed.String(), "未能生成最终结论") {
		t.Fatalf("fallback summary must be streamed: %q", streamed.String())
	}
	if len(fake.reqs) != 3 {
		t.Fatalf("expected 2 loop rounds + 1 finalize call, got %d", len(fake.reqs))
	}
}

// TestRunNativeLoop_FinalizeAfterMaxIterations 预算用尽后的收尾调用：最后一条
// 观测尾部带收尾指令（不新开 user 轮），模型的收尾正文成为答案并前置
// 确定性的未完成提示；收尾轮里的工具调用被忽略，transcript 不含收尾指令。
func TestRunNativeLoop_FinalizeAfterMaxIterations(t *testing.T) {
	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "noop_lookup", Arguments: `{"id":1}`}}}},
		{{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "noop_lookup", Arguments: `{"id":2}`}}}},
		{{Reasoning: "汇总已有观测"}, {Content: "CPU 正常；内存待确认", ToolCalls: []llm.ToolCall{{ID: "c3", Name: "noop_lookup", Arguments: `{}`}}}},
	}}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 2, Timeout: 30000}, llmClient: fake}

	streamChan := make(chan *StreamChunk, 100)
	resp := a.runNativeLoop(context.Background(), &AgentRequest{Params: map[string]string{"lang": "en_US"}}, []ChatMessage{{Role: "user", Content: "analyze"}}, nil, &ToolLoopConfig{
		MaxIterations:        2,
		StreamChan:           streamChan,
		ExtractPartialResult: true,
		EmitTranscript:       true,
	})
	close(streamChan)

	if !resp.Incomplete || resp.Success || resp.Error == "" {
		t.Fatalf("exhausted loop must be incomplete with error: %+v", resp)
	}
	if !strings.HasPrefix(resp.Content, "> ⚠️ Analysis incomplete") || !strings.HasSuffix(resp.Content, "CPU 正常；内存待确认") {
		t.Fatalf("content = %q", resp.Content)
	}
	if len(resp.Steps) != 2 {
		t.Fatalf("finalize-round tool calls must not execute: steps=%d", len(resp.Steps))
	}
	finalReq := fake.reqs[2].Messages
	last := finalReq[len(finalReq)-1]
	if last.Role != llm.RoleTool || !strings.Contains(last.Content, "budget is exhausted") {
		t.Fatalf("finalize instruction must ride on the last tool observation: %+v", last)
	}
	// 下发顺序：收尾轮思考 → header → 收尾正文。header 必须与正文相邻（同一
	// 正文段），否则路由层会把它切成独立段、与权威正文里的 header 重复展示。
	var order []string
	for ch := range streamChan {
		switch ch.Type {
		case StreamTypeThinking:
			order = append(order, "thinking")
		case StreamTypeContent:
			if strings.Contains(ch.Delta, "Analysis incomplete") {
				order = append(order, "header")
			} else {
				order = append(order, "content")
			}
		case StreamTypeTranscript:
			raw, _ := json.Marshal(ch.Transcript)
			if strings.Contains(string(raw), "budget is exhausted") {
				t.Fatalf("finalize instruction must not be persisted: %s", raw)
			}
		}
	}
	if got := strings.Join(order, ","); got != "thinking,header,content" {
		t.Fatalf("stream order = %s, want thinking,header,content", got)
	}
}

// TestRunNativeLoop_BudgetWarningAndRepeatNote 剩余预算降到阈值时在观测尾部
// 提醒收敛；读类工具同名同参重复调用仍执行，但观测尾部提示复用结果。
func TestRunNativeLoop_BudgetWarningAndRepeatNote(t *testing.T) {
	same := llm.ToolCall{Name: "noop_lookup", Arguments: `{"id":7}`}
	rounds := [][]llm.StreamChunk{}
	for i := 0; i < 4; i++ {
		tc := same
		tc.ID = "c" + string(rune('a'+i))
		rounds = append(rounds, []llm.StreamChunk{{ToolCalls: []llm.ToolCall{tc}}})
	}
	rounds = append(rounds, []llm.StreamChunk{{Content: "done"}})
	fake := &scriptedNativeLLM{rounds: rounds}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 5, Timeout: 30000}, llmClient: fake}

	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "q"}}, nil, &ToolLoopConfig{MaxIterations: 5})
	if !resp.Success || resp.Incomplete {
		t.Fatalf("resp = %+v", resp)
	}
	if len(resp.Steps) != 4 {
		t.Fatalf("read tool repeats must still execute: steps=%d", len(resp.Steps))
	}
	// 重复调用提示只进喂模型的上下文：第 3 次调用后的请求里最后一条观测带
	// 提示，第 2 次之后的不带；steps（→ ToolResult/transcript）里永远不带。
	lastObs := func(req *llm.GenerateRequest) string { return req.Messages[len(req.Messages)-1].Content }
	if strings.Contains(lastObs(fake.reqs[2]), "完全相同的参数") || !strings.Contains(lastObs(fake.reqs[3]), "调用该工具 3 次") {
		t.Fatalf("repeat note: after 2nd=%q after 3rd=%q", lastObs(fake.reqs[2]), lastObs(fake.reqs[3]))
	}
	for i, st := range resp.Steps {
		if strings.Contains(st.Observation, "完全相同的参数") {
			t.Fatalf("step %d observation must stay clean: %q", i, st.Observation)
		}
	}
	// 第 3 轮（iteration=2）执行完后剩 2 轮 → 第 4 次请求的最后一条观测带提醒
	warned := func(req *llm.GenerateRequest) bool {
		m := req.Messages[len(req.Messages)-1]
		return strings.Contains(m.Content, "工具调用预算仅剩")
	}
	if warned(fake.reqs[2]) || !warned(fake.reqs[3]) || strings.Contains(resp.Steps[2].Observation, "工具调用预算仅剩") {
		t.Fatalf("budget warning must hit exactly the request after iteration 2 and stay out of steps")
	}
}

// TestRunNativeLoop_RepeatNoteSkipsPollingAndWriteTools 轮询类工具同参重复是
// 正常轮询、写类工具已由 dedup 提示，二者调用多少次都不追加重复调用提醒。
func TestRunNativeLoop_RepeatNoteSkipsPollingAndWriteTools(t *testing.T) {
	for _, name := range []string{"get_task_status", "create_noop_rule"} {
		t.Run(name, func(t *testing.T) {
			rounds := [][]llm.StreamChunk{}
			for i := 0; i < 4; i++ {
				tc := llm.ToolCall{ID: "c" + string(rune('a'+i)), Name: name, Arguments: `{"task_id":1}`}
				rounds = append(rounds, []llm.StreamChunk{{ToolCalls: []llm.ToolCall{tc}}})
			}
			rounds = append(rounds, []llm.StreamChunk{{Content: "done"}})
			fake := &scriptedNativeLLM{rounds: rounds}
			a := &Agent{cfg: &AgentConfig{MaxIterations: 10, Timeout: 30000}, llmClient: fake}

			resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "q"}}, nil, &ToolLoopConfig{MaxIterations: 10})
			if !resp.Success {
				t.Fatalf("resp = %+v", resp)
			}
			for i, req := range fake.reqs {
				if last := req.Messages[len(req.Messages)-1].Content; strings.Contains(last, "完全相同的参数") {
					t.Fatalf("request %d must not carry repeat note: %q", i, last)
				}
			}
		})
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

// TestRunNativeLoop_VLLMReasoningRoundTrip 真实 OpenAI 客户端对接模拟的 vLLM
// SSE 端点（推理走 delta.reasoning）：工具续轮的请求体里推理以 reasoning 回填，
// 不发 reasoning_content——vLLM Python 前端只读 reasoning（issue #3440）。
func TestRunNativeLoop_VLLMReasoningRoundTrip(t *testing.T) {
	var bodies []map[string]any
	round := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "text/event-stream")
		if round == 0 {
			fmt.Fprint(w, `data: {"choices":[{"delta":{"reasoning":"check cpu first"}}]}`+"\n\n"+
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"noop_lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n"+
				"data: [DONE]\n\n")
		} else {
			fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"done"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
		}
		round++
	}))
	defer srv.Close()

	client, err := llm.NewOpenAI(&llm.Config{Model: "deepseek-v41", BaseURL: srv.URL + "/v1/chat/completions"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 5, Timeout: 30000}, llmClient: client}
	streamChan := make(chan *StreamChunk, 100)
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "q"}}, nil, &ToolLoopConfig{
		MaxIterations: 5,
		StreamChan:    streamChan,
	})
	close(streamChan)
	if !resp.Success || len(bodies) != 2 {
		t.Fatalf("resp=%+v requests=%d", resp, len(bodies))
	}
	msgs, _ := bodies[1]["messages"].([]any)
	var asst map[string]any
	for _, m := range msgs {
		if mm, _ := m.(map[string]any); mm["role"] == "assistant" {
			asst = mm
		}
	}
	if asst["reasoning"] != "check cpu first" || asst["reasoning_content"] != nil {
		t.Fatalf("assistant tool turn on the wire = %v", asst)
	}
}
