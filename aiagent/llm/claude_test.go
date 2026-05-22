package llm

import (
	"encoding/json"
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
			"model":    "evil-override",      // 必须被显式字段挡掉
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
