package chat

import (
	"testing"
)

func TestEnrichContext_ResolvesDashboardURL(t *testing.T) {
	req := &AIChatRequest{UserInput: "请把 http://n9e/dashboards/88 这个大盘的变量修一下"}
	EnrichContextFromText(req)
	if got := ctxInt64(req.Context, "dashboard_id"); got != 88 {
		t.Fatalf("dashboard_id = %d, want 88", got)
	}
}

// A pasted dashboard URL must override a stale page-context dashboard_id so the
// user's explicit target wins (otherwise the wrong board gets edited).
func TestEnrichContext_DashboardURLOverridesContext(t *testing.T) {
	req := &AIChatRequest{
		UserInput: "改 http://n9e/dashboards/99 这个大盘",
		Context:   map[string]interface{}{"dashboard_id": float64(7)},
	}
	EnrichContextFromText(req)
	if got := ctxInt64(req.Context, "dashboard_id"); got != 99 {
		t.Fatalf("dashboard_id = %d, want 99 (pasted URL must override page context)", got)
	}
}

// An alert-rules edit URL must NOT be misread as a dashboard id.
func TestEnrichContext_AlertURLNotDashboard(t *testing.T) {
	req := &AIChatRequest{UserInput: "把 http://n9e/alert-rules/edit/178 改成20"}
	EnrichContextFromText(req)
	if got := ctxInt64(req.Context, "rule_id"); got != 178 {
		t.Fatalf("rule_id = %d, want 178", got)
	}
	if got := ctxInt64(req.Context, "dashboard_id"); got != 0 {
		t.Fatalf("dashboard_id = %d, want 0 (alert URL must not set it)", got)
	}
}
