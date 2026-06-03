package chat

import (
	"context"
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
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := looksLikeDashboardEdit(c.req); got != c.want {
				t.Fatalf("looksLikeDashboardEdit(%q) = %v, want %v", c.req.UserInput, got, c.want)
			}
		})
	}
}

func TestPreflightEdit_ResolvesDashboardURL(t *testing.T) {
	req := &AIChatRequest{UserInput: "请把 http://n9e/dashboards/88 这个大盘的变量修一下"}
	if _, _, err := PreflightEdit(context.Background(), nil, req, nil); err != nil {
		t.Fatalf("PreflightEdit err: %v", err)
	}
	if got := ctxInt64(req.Context, "dashboard_id"); got != 88 {
		t.Fatalf("dashboard_id = %d, want 88", got)
	}
}

// A pasted dashboard URL must override a stale page-context dashboard_id so the
// user's explicit target wins (otherwise the wrong board gets edited).
func TestPreflightEdit_DashboardURLOverridesContext(t *testing.T) {
	req := &AIChatRequest{
		UserInput: "改 http://n9e/dashboards/99 这个大盘",
		Context:   map[string]interface{}{"dashboard_id": float64(7)},
	}
	if _, _, err := PreflightEdit(context.Background(), nil, req, nil); err != nil {
		t.Fatalf("PreflightEdit err: %v", err)
	}
	if got := ctxInt64(req.Context, "dashboard_id"); got != 99 {
		t.Fatalf("dashboard_id = %d, want 99 (pasted URL must override page context)", got)
	}
}

// An alert-rules edit URL must NOT be misread as a dashboard id.
func TestPreflightEdit_AlertURLNotDashboard(t *testing.T) {
	req := &AIChatRequest{UserInput: "把 http://n9e/alert-rules/edit/178 改成20"}
	if _, _, err := PreflightEdit(context.Background(), nil, req, nil); err != nil {
		t.Fatalf("PreflightEdit err: %v", err)
	}
	if got := ctxInt64(req.Context, "rule_id"); got != 178 {
		t.Fatalf("rule_id = %d, want 178", got)
	}
	if got := ctxInt64(req.Context, "dashboard_id"); got != 0 {
		t.Fatalf("dashboard_id = %d, want 0 (alert URL must not set it)", got)
	}
}
