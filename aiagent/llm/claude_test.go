package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestClaudeRequest_MarshalExtraBody 确保 extraBody 被平铺到顶层 JSON，
// 且显式字段（model/messages/...）不会被同名 key 偷偷覆盖。
func TestClaudeRequest_MarshalExtraBody(t *testing.T) {
	req := claudeRequest{
		Model:     "kimi-k2.5",
		MaxTokens: 1024,
		Messages: []claudeMessage{
			{Role: "user", Content: []claudeContentBlock{{Type: "text", Text: "hi"}}},
		},
		extraBody: map[string]any{
			"thinking": map[string]any{"type": "disabled"},
			"model":    "evil-override", // 必须被显式字段挡掉
			"foo":      "bar",
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal back failed: %v", err)
	}

	if m["model"] != "kimi-k2.5" {
		t.Errorf("model was overwritten by extraBody: got %v", m["model"])
	}
	if _, ok := m["thinking"]; !ok {
		t.Errorf("extraBody.thinking missing from request body: %s", data)
	}
	if m["foo"] != "bar" {
		t.Errorf("extraBody.foo missing or wrong: %v", m["foo"])
	}

	// Sanity: 字段串里应该明确出现 thinking 和 disabled
	s := string(data)
	if !strings.Contains(s, `"thinking"`) || !strings.Contains(s, `"disabled"`) {
		t.Errorf("expected thinking.disabled in body, got: %s", s)
	}
}

// TestClaudeRequest_MarshalNoExtraBody 确认不带 extraBody 时行为不变。
func TestClaudeRequest_MarshalNoExtraBody(t *testing.T) {
	req := claudeRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 1024,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !strings.Contains(string(data), `"model":"claude-sonnet-4-5"`) {
		t.Errorf("baseline marshal broke: %s", data)
	}
}

// TestClaudeContentBlock_EmptyThinkingKept 空 thinking + signature 回放时
// 不能被 omitempty 丢掉，否则 Anthropic 续轮 400（#3327）。
func TestClaudeContentBlock_EmptyThinkingKept(t *testing.T) {
	block := claudeContentBlock{
		Type:      "thinking",
		Thinking:  "",
		Signature: "sig",
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"thinking":""`) {
		t.Errorf("empty thinking dropped from replay payload: %s", s)
	}
	if !strings.Contains(s, `"signature":"sig"`) {
		t.Errorf("signature missing: %s", s)
	}
}

func TestClaudeContentBlock_ThinkingOmittedFromOtherTypes(t *testing.T) {
	tests := []struct {
		block claudeContentBlock
		want  map[string]any
	}{
		{
			block: claudeContentBlock{Type: "text", Text: "hello"},
			want:  map[string]any{"type": "text", "text": "hello"},
		},
		{
			block: claudeContentBlock{Type: "tool_use", ID: "call-1", Name: "lookup", Input: map[string]any{}},
			want:  map[string]any{"type": "tool_use", "id": "call-1", "name": "lookup", "input": map[string]any{}},
		},
		{
			block: claudeContentBlock{Type: "tool_result", ToolUseID: "call-1", Content: "done"},
			want:  map[string]any{"type": "tool_result", "tool_use_id": "call-1", "content": "done"},
		},
		{
			block: claudeContentBlock{Type: "redacted_thinking", Data: "encrypted"},
			want:  map[string]any{"type": "redacted_thinking", "data": "encrypted"},
		},
	}

	for _, test := range tests {
		t.Run(test.block.Type, func(t *testing.T) {
			data, err := json.Marshal(test.block)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatalf("unmarshal back failed: %v", err)
			}
			if !reflect.DeepEqual(payload, test.want) {
				t.Errorf("payload = %#v, want %#v", payload, test.want)
			}
		})
	}
}
