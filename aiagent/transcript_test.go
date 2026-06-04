package aiagent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestVisibleConversation(t *testing.T) {
	h := []ChatMessage{
		{Role: "user", Content: "把 http://x/dashboards/5 第一行图表改为显示最大值"},
		{Role: "assistant", Content: "Thought: 我需要先读取大盘\nAction: get_dashboard_detail\nAction Input: {\"id\":5}"},
		{Role: "user", Content: "Observation: {\"panels\":[...]}"},
		{Role: "assistant", Content: "Thought: 生成提案\nAction: update_dashboard\nAction Input: {\"id\":5,...}"},
		{Role: "user", Content: "Observation: {\"proposal_id\":\"dbprop_abc\",\"applied\":false}"},
		{Role: "assistant", Content: "已生成提案：第一行 Stat 图表外层包裹 max()，确认吗？"},
		{Role: "user", Content: "Observation: ⚠️ Format error: your previous response was NOT in the required ReAct format."},
		{Role: "user", Content: "确认"},
	}

	got := VisibleConversation(h)

	// Expect only: real user turns + the final (clean) assistant answer, in order.
	want := []ChatMessage{
		{Role: "user", Content: "把 http://x/dashboards/5 第一行图表改为显示最大值"},
		{Role: "assistant", Content: "已生成提案：第一行 Stat 图表外层包裹 max()，确认吗？"},
		{Role: "user", Content: "确认"},
	}
	if len(got) != len(want) {
		t.Fatalf("VisibleConversation len = %d, want %d; got=%+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Role != want[i].Role || got[i].Content != want[i].Content {
			t.Fatalf("VisibleConversation[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestVisibleConversation_KeepsFinalAnswerActionForm(t *testing.T) {
	// An assistant turn whose Action IS "Final Answer" is a terminal answer, kept.
	h := []ChatMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "Thought: done\nAction: Final Answer\nAction Input: hello"},
	}
	got := VisibleConversation(h)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (Final Answer action turn must be kept); got=%+v", len(got), got)
	}
}

func TestIsReActToolTurn(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"tool call", "Thought: x\nAction: update_dashboard\nAction Input: {}", true},
		{"final answer action", "Thought: x\nAction: Final Answer\nAction Input: ok", false},
		{"clean answer", "已生成提案，确认吗？", false},
		{"answer with json no action", "结果：{\"name\":\"x\",\"arguments\":{}}", false},
		{"action inline final", "Action: Final Answer", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isReActToolTurn(c.content); got != c.want {
				t.Fatalf("isReActToolTurn(%q) = %v, want %v", c.content, got, c.want)
			}
		})
	}
}

func TestTranscriptEnvelopeRoundTrip(t *testing.T) {
	env := TranscriptEnvelope{
		SchemaVersion: TranscriptSchemaVersion,
		Messages: []ChatMessage{
			{Role: "user", Content: "q1"},
			{Role: "assistant", Content: "Action: update_dashboard\nAction Input: {}"},
			{Role: "user", Content: "Observation: ok"},
			{Role: "assistant", Content: "done"},
		},
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back TranscriptEnvelope
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.SchemaVersion != TranscriptSchemaVersion {
		t.Fatalf("schema_version = %d, want %d", back.SchemaVersion, TranscriptSchemaVersion)
	}
	if len(back.Messages) != len(env.Messages) {
		t.Fatalf("messages len = %d, want %d", len(back.Messages), len(env.Messages))
	}
	for i := range env.Messages {
		if !reflect.DeepEqual(back.Messages[i], env.Messages[i]) {
			t.Fatalf("messages[%d] = %+v, want %+v", i, back.Messages[i], env.Messages[i])
		}
	}
	// schema_version must actually be present in the JSON (forward-evolution hook).
	if !strings.Contains(string(b), "\"schema_version\"") {
		t.Fatalf("serialized envelope missing schema_version: %s", b)
	}
}
