package aiagent

import (
	"encoding/json"
	"testing"
)

// TestBuildCreationFormVisibilityField locks the visibility field contract shared
// with the frontend: key "visibility", single-select, two candidates whose IDs
// mirror the models.Private flag (0=public, 1=team-scoped), team-scoped default.
func TestBuildCreationFormVisibilityField(t *testing.T) {
	// deps/user are unused for the "visibility" case (fixed enum, no DB lookup),
	// so nil is safe here.
	raw := BuildCreationForm(nil, nil, "zh_CN", "mcp-server", []string{"visibility"}, FormPreselect{})

	var payload FormPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal form payload: %v", err)
	}
	if len(payload.Fields) != 1 {
		t.Fatalf("want 1 field, got %d", len(payload.Fields))
	}
	f := payload.Fields[0]
	if f.Key != "visibility" || f.Type != "single" {
		t.Fatalf("unexpected field key/type: %+v", f)
	}
	if len(f.Candidates) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(f.Candidates))
	}
	if f.Candidates[0].ID != VisibilityPublic {
		t.Fatalf("first candidate should be public(%d), got %d", VisibilityPublic, f.Candidates[0].ID)
	}
	if f.Candidates[1].ID != VisibilityTeamScope || !f.Candidates[1].IsDefault {
		t.Fatalf("second candidate should be team-scoped(%d) and default, got %+v", VisibilityTeamScope, f.Candidates[1])
	}
}
