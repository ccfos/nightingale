package aiagent

import (
	"strings"
	"testing"
)

// 复现线上 chat 4123af5c / n9e contextId 019e81f3 的实际输出：deepseek-v4-pro
// 把 "Action:" 黏在 Thought 行尾，导致工具从未执行、仪表盘没生成。
func TestParseReActResponse_InlineActionMarker(t *testing.T) {
	resp := "Thought: 用户要求创建仪表盘。先调用 list_datasources 获取 Prometheus 数据源列表。Action: list_datasources\n" +
		`Action Input: {"plugin_type": "prometheus", "limit": 10}`

	step := (&Agent{}).parseReActResponse(resp)

	if step.Action != "list_datasources" {
		t.Fatalf("Action = %q, want list_datasources", step.Action)
	}
	if step.ActionInput != `{"plugin_type": "prometheus", "limit": 10}` {
		t.Fatalf("ActionInput = %q", step.ActionInput)
	}
	if strings.Contains(step.Thought, "Action:") {
		t.Fatalf("Thought 不应再含 Action 标记: %q", step.Thought)
	}
}

// 告警规则那轮同样的形态：Action: list_metrics 黏在 Thought 行尾。
func TestParseReActResponse_InlineActionMarker_ListMetrics(t *testing.T) {
	resp := "Thought: 先确认数据源类型。用 list_metrics 探测一下即可。Action: list_metrics\n" +
		`Action Input: {"datasource_id": 10249, "keyword": "cpu_usage_idle", "limit": 5}`

	step := (&Agent{}).parseReActResponse(resp)
	if step.Action != "list_metrics" {
		t.Fatalf("Action = %q, want list_metrics", step.Action)
	}
}

// 规范三行格式不得被改动（无回归）。
func TestParseReActResponse_CanonicalUnchanged(t *testing.T) {
	resp := "Thought: reasoning\nAction: list_datasources\nAction Input: {\"plugin_type\":\"prometheus\"}"
	step := (&Agent{}).parseReActResponse(resp)
	if step.Action != "list_datasources" || step.ActionInput != `{"plugin_type":"prometheus"}` {
		t.Fatalf("regression: action=%q input=%q", step.Action, step.ActionInput)
	}
	if strings.TrimSpace(step.Thought) != "reasoning" {
		t.Fatalf("thought = %q, want reasoning", step.Thought)
	}
}

// Final Answer 的 markdown 正文里出现 "Action:" 不得被截断/改写。
func TestParseReActResponse_FinalAnswerBodyPreserved(t *testing.T) {
	resp := "Thought: done\nAction: Final Answer\nAction Input:\n## 结论\n下一步 Action: 部署服务"
	step := (&Agent{}).parseReActResponse(resp)
	if step.Action != ActionFinalAnswer {
		t.Fatalf("Action = %q, want %q", step.Action, ActionFinalAnswer)
	}
	if !strings.Contains(step.ActionInput, "下一步 Action: 部署服务") {
		t.Fatalf("正文被破坏: %q", step.ActionInput)
	}
}

// Final Answer: 简写形式（黏在 Thought 行尾）也应被识别为最终答案。
func TestParseReActResponse_InlineFinalAnswerShorthand(t *testing.T) {
	resp := "Thought: 我已经有足够信息了。Final Answer: 仪表盘创建完成"
	step := (&Agent{}).parseReActResponse(resp)
	if step.Action != ActionFinalAnswer {
		t.Fatalf("Action = %q, want %q", step.Action, ActionFinalAnswer)
	}
	if strings.TrimSpace(step.ActionInput) != "仪表盘创建完成" {
		t.Fatalf("ActionInput = %q", step.ActionInput)
	}
}

// 已是行首标记时归一化应幂等。
func TestNormalizeReActMarkers_Idempotent(t *testing.T) {
	s := "Thought: a\nAction: b\nAction Input: {}"
	if got := normalizeReActMarkers(s); got != s {
		t.Fatalf("not idempotent:\n got=%q\nwant=%q", got, s)
	}
}
