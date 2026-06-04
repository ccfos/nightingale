package aiagent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
)

// scriptedLLM is a fake llm.LLM that returns a pre-scripted response per call,
// one per ReAct iteration. Enough to drive runReActLoop in tests.
type scriptedLLM struct {
	responses []string
	call      int
}

func (s *scriptedLLM) Name() string { return "scripted" }

func (s *scriptedLLM) Generate(ctx context.Context, req *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	return nil, errors.New("scriptedLLM: Generate not used")
}

func (s *scriptedLLM) GenerateStream(ctx context.Context, req *llm.GenerateRequest) (<-chan llm.StreamChunk, error) {
	resp := ""
	if s.call < len(s.responses) {
		resp = s.responses[s.call]
	}
	s.call++
	out := make(chan llm.StreamChunk, 2)
	out <- llm.StreamChunk{Content: resp}
	out <- llm.StreamChunk{Done: true}
	close(out)
	return out, nil
}

// TestRunReActLoop_EmitsTranscript drives one tool step + a final answer and
// asserts the loop emits exactly the intermediate tool-call/observation pair as a
// StreamTypeTranscript chunk, and NOT the final answer (the router appends that).
func TestRunReActLoop_EmitsTranscript(t *testing.T) {
	fake := &scriptedLLM{responses: []string{
		// iteration 0: a valid ReAct tool call (unknown tool → error observation,
		// but the transcript pair is still emitted, which is what we assert).
		"Thought: 先读配置\nAction: noop_lookup\nAction Input: {\"id\":5}",
		// iteration 1: final answer → loop returns, no transcript emitted here.
		"Thought: 完成\nAction: Final Answer\nAction Input: 已生成提案，确认吗？",
	}}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 5, Timeout: 30000}, llmClient: fake}

	streamChan := make(chan *StreamChunk, 100)
	config := &ReActLoopConfig{
		MaxIterations:  5,
		StreamChan:     streamChan,
		IsComplete:     func(action string) bool { return action == ActionFinalAnswer },
		EmitTranscript: true,
	}
	resp := a.runReActLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "改大盘"}}, config)
	close(streamChan)

	if !resp.Success {
		t.Fatalf("loop did not succeed: %+v", resp)
	}
	if strings.TrimSpace(resp.Content) != "已生成提案，确认吗？" {
		t.Fatalf("final content = %q, want the Final Answer body", resp.Content)
	}

	var transcripts [][]ChatMessage
	for ch := range streamChan {
		if ch.Type == StreamTypeTranscript {
			transcripts = append(transcripts, ch.Transcript)
		}
	}

	if len(transcripts) != 1 {
		t.Fatalf("got %d transcript chunks, want exactly 1 (one tool step, no final)", len(transcripts))
	}
	pair := transcripts[0]
	if len(pair) != 2 {
		t.Fatalf("transcript pair len = %d, want 2 (assistant tool turn + user observation)", len(pair))
	}
	if pair[0].Role != "assistant" || !strings.Contains(pair[0].Content, "Action: noop_lookup") {
		t.Fatalf("transcript[0] = %+v, want assistant turn containing the tool call", pair[0])
	}
	if pair[1].Role != "user" || !strings.HasPrefix(strings.TrimSpace(pair[1].Content), "Observation:") {
		t.Fatalf("transcript[1] = %+v, want user Observation turn", pair[1])
	}
}

// TestEmitTranscript_Gating verifies the helper only emits when streaming AND
// EmitTranscript is on, and never panics on a nil channel.
func TestEmitTranscript_Gating(t *testing.T) {
	msgs := []ChatMessage{{Role: "assistant", Content: "a"}, {Role: "user", Content: "Observation: b"}}

	// on
	ch := make(chan *StreamChunk, 1)
	emitTranscript(&ReActLoopConfig{StreamChan: ch, EmitTranscript: true}, msgs...)
	select {
	case got := <-ch:
		if got.Type != StreamTypeTranscript || len(got.Transcript) != 2 {
			t.Fatalf("emitted chunk = %+v, want StreamTypeTranscript with 2 msgs", got)
		}
	default:
		t.Fatal("expected a transcript chunk, got none")
	}

	// off
	ch2 := make(chan *StreamChunk, 1)
	emitTranscript(&ReActLoopConfig{StreamChan: ch2, EmitTranscript: false}, msgs...)
	if len(ch2) != 0 {
		t.Fatal("EmitTranscript=false must not emit")
	}

	// nil channel: must not panic
	emitTranscript(&ReActLoopConfig{StreamChan: nil, EmitTranscript: true}, msgs...)
}
