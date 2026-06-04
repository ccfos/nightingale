package chat

import (
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
)

func TestLooksLikeDashboardEdit(t *testing.T) {
	cases := []struct {
		name string
		req  *AIChatRequest
		want bool
	}{
		{"keyword 仪表盘", &AIChatRequest{UserInput: "把这个仪表盘的 ident 变量默认值改成 web01"}, true},
		{"keyword 大盘 panel", &AIChatRequest{UserInput: "给内存图表加一条曲线"}, true},
		{"english dashboard", &AIChatRequest{UserInput: "rename the panel on this dashboard"}, true},
		{"dashboard url", &AIChatRequest{UserInput: "http://x/dashboards/42 改下变量"}, true},
		{"dashboard_id ctx", &AIChatRequest{UserInput: "改一下变量", Context: map[string]interface{}{"dashboard_id": float64(7)}}, true},
		{"alert rule edit", &AIChatRequest{UserInput: "把这条告警规则阈值改成20"}, false},
		{"plain disable rule", &AIChatRequest{UserInput: "禁用这条规则"}, false},
		// Alert-rule signals must win over a stale dashboard_id page context.
		{"rule_id ctx beats dashboard_id", &AIChatRequest{UserInput: "改一下查询曲线", Context: map[string]interface{}{"dashboard_id": float64(7), "rule_id": float64(5)}}, false},
		{"alert url beats dashboard_id ctx", &AIChatRequest{UserInput: "http://x/alert-rules/edit/9 改这个图表对应的阈值", Context: map[string]interface{}{"dashboard_id": float64(7)}}, false},
		{"告警规则 keyword beats dashboard_id ctx", &AIChatRequest{UserInput: "把这条告警规则的曲线查询改一下", Context: map[string]interface{}{"dashboard_id": float64(7)}}, false},
		// An explicit dashboard URL still wins over alert keywords only when no
		// alert structured/keyword signal is present.
		{"event_id ctx is alert", &AIChatRequest{UserInput: "改这个图表", Context: map[string]interface{}{"event_id": float64(3)}}, false},
		// A bare confirmation carries no target of its own. It must continue the
		// in-flight edit inferred from history rather than defaulting to the
		// alert-rule workflow (the original "回复确认后没修复仪表盘" bug).
		{"confirm continues dashboard edit", &AIChatRequest{
			UserInput: "确认",
			History: []aiagent.ChatMessage{
				{Role: "user", Content: "把 http://x/dashboards/5 第一行图表改为显示最大值"},
				{Role: "assistant", Content: "已生成提案：第一行 Stat 图表的 PromQL 将外层包裹 max()，确认以上改动吗？"},
			},
		}, true},
		{"confirm continues alert-rule edit", &AIChatRequest{
			UserInput: "确认",
			History: []aiagent.ChatMessage{
				{Role: "user", Content: "把这条告警规则阈值改成20"},
				{Role: "assistant", Content: "已生成提案：阈值 10 → 20，确认吗？"},
			},
		}, false},
		{"confirm with no history stays default", &AIChatRequest{UserInput: "确认"}, false},
		// Persisted route state (prev_edit_target, injected by the router from
		// models.ConversationRoute) is the authoritative continuation signal —
		// stronger than the history bootstrap, weaker than explicit current-turn
		// signals.
		{"confirm inherits dashboard route", &AIChatRequest{
			UserInput: "确认",
			Context:   map[string]interface{}{"prev_edit_target": EditTargetDashboard},
		}, true},
		{"confirm inherits alert route", &AIChatRequest{
			UserInput: "确认",
			Context:   map[string]interface{}{"prev_edit_target": EditTargetAlertRule},
		}, false},
		{"explicit alert signal beats dashboard route", &AIChatRequest{
			UserInput: "把这条告警规则阈值改成20",
			Context:   map[string]interface{}{"prev_edit_target": EditTargetDashboard},
		}, false},
		{"explicit dashboard URL beats alert route", &AIChatRequest{
			UserInput: "改下 http://x/dashboards/9 的变量",
			Context:   map[string]interface{}{"prev_edit_target": EditTargetAlertRule},
		}, true},
		{"route state beats history bootstrap", &AIChatRequest{
			UserInput: "确认",
			Context:   map[string]interface{}{"prev_edit_target": EditTargetAlertRule},
			History: []aiagent.ChatMessage{
				{Role: "user", Content: "改一下这个仪表盘的图表"},
				{Role: "assistant", Content: "好的"},
			},
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := looksLikeDashboardEdit(c.req); got != c.want {
				t.Fatalf("looksLikeDashboardEdit(%q) = %v, want %v", c.req.UserInput, got, c.want)
			}
		})
	}
}

// editRequiredSkills must follow the same decision as the prompt framing, and be
// wired into the edit action so the LLM skill autoselect is bypassed for edits.
func TestEditRequiredSkills(t *testing.T) {
	dash := &AIChatRequest{
		UserInput: "确认",
		Context:   map[string]interface{}{"prev_edit_target": EditTargetDashboard},
	}
	if got := editRequiredSkills(dash); len(got) != 1 || got[0] != "n9e-modify-dashboard" {
		t.Fatalf("editRequiredSkills(dashboard continuation) = %v, want [n9e-modify-dashboard]", got)
	}
	alert := &AIChatRequest{UserInput: "把这条告警规则阈值改成20"}
	if got := editRequiredSkills(alert); got != nil {
		t.Fatalf("editRequiredSkills(alert) = %v, want nil", got)
	}
	h, ok := Lookup("edit")
	if !ok || h.RequiredSkills == nil {
		t.Fatal("edit action must declare RequiredSkills (bypasses skill autoselect)")
	}
}

func TestResolveEditTarget(t *testing.T) {
	dash := &AIChatRequest{UserInput: "把这个仪表盘的 ident 变量默认值改成 web01"}
	if got := ResolveEditTarget(dash); got != EditTargetDashboard {
		t.Fatalf("ResolveEditTarget(dashboard req) = %q, want %q", got, EditTargetDashboard)
	}
	alert := &AIChatRequest{UserInput: "把这条告警规则阈值改成20"}
	if got := ResolveEditTarget(alert); got != EditTargetAlertRule {
		t.Fatalf("ResolveEditTarget(alert req) = %q, want %q", got, EditTargetAlertRule)
	}
	// The confirm turn with inherited route resolves to the same target — this is
	// what the router persists, keeping the route sticky across turns.
	confirm := &AIChatRequest{
		UserInput: "确认",
		Context:   map[string]interface{}{"prev_edit_target": EditTargetDashboard},
	}
	if got := ResolveEditTarget(confirm); got != EditTargetDashboard {
		t.Fatalf("ResolveEditTarget(confirm w/ dashboard route) = %q, want %q", got, EditTargetDashboard)
	}
}

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
