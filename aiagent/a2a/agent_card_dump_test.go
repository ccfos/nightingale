package a2a

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentCardListsBuiltinSkills(t *testing.T) {
	h := AgentCardHandler(AgentCardOptions{
		BaseURL:         "https://example.com",
		A2APath:         "/a2a",
		TokenHeaderName: "X-User-Token",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	var card map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &card); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	skills, _ := card["skills"].([]any)
	if len(skills) == 0 {
		t.Fatalf("expected non-empty skills list")
	}
	for _, s := range skills {
		m := s.(map[string]any)
		if m["id"] == "" || m["name"] == "" || m["description"] == "" {
			t.Errorf("skill missing required field: %+v", m)
		}
		if tags, _ := m["tags"].([]any); len(tags) == 0 {
			t.Errorf("skill %v has empty tags", m["id"])
		}
	}
	t.Logf("AgentCard advertises %d builtin skills", len(skills))
}
