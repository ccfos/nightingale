package router

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
)

func msg(role, content string) aiagent.ChatMessage {
	return aiagent.ChatMessage{Role: role, Content: content}
}

func TestAssembleTurnHistory(t *testing.T) {
	prev := []aiagent.ChatMessage{
		msg("user", "q0"),
		msg("assistant", "a0"),
	}
	turnMsgs := []aiagent.ChatMessage{
		msg("assistant", "Thought: x\nAction: update_dashboard\nAction Input: {\"id\":5}"),
		msg("user", "Observation: {\"proposal_id\":\"dbprop_X\"}"),
	}

	got := assembleTurnHistory(prev, "确认", turnMsgs, "已写回")

	want := []aiagent.ChatMessage{
		msg("user", "q0"),
		msg("assistant", "a0"),
		msg("user", "确认"),
		msg("assistant", "Thought: x\nAction: update_dashboard\nAction Input: {\"id\":5}"),
		msg("user", "Observation: {\"proposal_id\":\"dbprop_X\"}"),
		msg("assistant", "已写回"),
	}
	assertMsgsEqual(t, got, want)

	// Exactly one terminal assistant turn equal to fullContent.
	if last := got[len(got)-1]; last.Role != "assistant" || last.Content != "已写回" {
		t.Fatalf("terminal turn = %+v, want assistant/已写回", last)
	}

	// Must not alias prev (mutating result must not corrupt prev).
	got[0] = msg("user", "MUTATED")
	if prev[0].Content != "q0" {
		t.Fatalf("assembleTurnHistory aliased prev: prev[0]=%+v", prev[0])
	}
}

func TestAssembleTurnHistory_DirectMode(t *testing.T) {
	// No tool turns (Direct / no-tool path): prev + user + assistant(answer).
	got := assembleTurnHistory(nil, "什么是 P99", nil, "P99 是…")
	want := []aiagent.ChatMessage{
		msg("user", "什么是 P99"),
		msg("assistant", "P99 是…"),
	}
	assertMsgsEqual(t, got, want)
}

func TestAssembleTurnHistory_EmptyFinal(t *testing.T) {
	// Halted / no-answer turn: no trailing assistant message.
	got := assembleTurnHistory([]aiagent.ChatMessage{msg("user", "q0")}, "选业务组", nil, "")
	want := []aiagent.ChatMessage{
		msg("user", "q0"),
		msg("user", "选业务组"),
	}
	assertMsgsEqual(t, got, want)
}

// TestProposalIDSurvivesToConfirmTurn is the headline correctness proof for Step 1
// (design doc bug #2): a proposal_id returned in a tool Observation on turn N must
// be visible to the model on the confirm turn N+1, while staying OUT of the intent
// classifier's view.
func TestProposalIDSurvivesToConfirmTurn(t *testing.T) {
	// Turn N: the agent proposed a dashboard change; the tool Observation carries
	// the proposal_id. This is exactly what the ReAct loop emits as turnMsgs.
	turnMsgs := []aiagent.ChatMessage{
		msg("assistant", "Thought: 生成提案\nAction: update_dashboard\nAction Input: {\"id\":5,\"panels\":[...]}"),
		msg("user", "Observation: {\"proposal_id\":\"dbprop_e5c0147422b95ec0\",\"applied\":false,\"changes\":[\"panel-0 PromQL 包裹 max()\"]}"),
	}
	turnN := assembleTurnHistory(nil, "把大盘5第一行改为显示最大值", turnMsgs, "已生成提案：第一行包裹 max()，确认吗？")

	// Persist exactly as the router does, then reload exactly as the next turn does.
	blob, err := json.Marshal(aiagent.TranscriptEnvelope{
		SchemaVersion: aiagent.TranscriptSchemaVersion,
		Messages:      turnN,
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var env aiagent.TranscriptEnvelope
	if err := json.Unmarshal(blob, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	reloaded := env.Messages // == AgentRequest.History on the confirm turn

	// (1) The proposal_id reaches the model next turn.
	if !containsContent(reloaded, "dbprop_e5c0147422b95ec0") {
		t.Fatalf("proposal_id did not survive into next-turn history: %+v", reloaded)
	}

	// (2) But the intent classifier's view stays clean (no Observation scaffolding).
	vis := aiagent.VisibleConversation(reloaded)
	if containsContent(vis, "dbprop_e5c0147422b95ec0") {
		t.Fatalf("classifier view leaked the Observation/proposal_id: %+v", vis)
	}
	if containsContent(vis, "Observation:") {
		t.Fatalf("classifier view contains an Observation turn: %+v", vis)
	}
}

func assertMsgsEqual(t *testing.T, got, want []aiagent.ChatMessage) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d; got=%+v", len(got), len(want), got)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Fatalf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func containsContent(msgs []aiagent.ChatMessage, sub string) bool {
	for _, m := range msgs {
		if strings.Contains(m.Content, sub) {
			return true
		}
	}
	return false
}
