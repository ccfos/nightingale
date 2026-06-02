package aiagent

import (
	"strings"
	"testing"
)

// 复现线上 chat 465952ac seq3 / n9e contextId 019e82a3-1f03 seq2：deepseek-v4-pro
// 吐 XML 批量工具调用（畸形闭合标签 tool_call/tool_calls 混用），导致 MySQL 告警
// 规则创建卡死、executed_tools 始终 false。
func TestParseReActResponse_XMLToolCallBatch(t *testing.T) {
	resp := "好的，我需要先了解 MySQL 数据源的真实 schema，然后参考模板来构建规则。\n\n" +
		"首先，让我同时读取参考文档，并探测数据库列表。\n\n" +
		"<tool_calls>\n" +
		"<tool_call name=\"read_file\">\n" +
		"{\"base\": \"n9e-create-alert-rule\", \"path\": \"datasources/mysql.md\"}\n" +
		"</tool_call>\n" +
		"<tool_calls name=\"list_databases\">\n" +
		"{\"datasource_id\": 10248}\n" +
		"</tool_call>\n" +
		"</tool_calls>"

	step := (&Agent{}).parseReActResponse(resp)
	if step.Action != "read_file" {
		t.Fatalf("Action = %q, want read_file", step.Action)
	}
	if step.ActionInput != `{"base": "n9e-create-alert-rule", "path": "datasources/mysql.md"}` {
		t.Fatalf("ActionInput = %q", step.ActionInput)
	}
	if !strings.Contains(step.Thought, "MySQL 数据源") {
		t.Fatalf("Thought 丢失: %q", step.Thought)
	}
}

// 单个 XML 工具调用、无外层包裹、无前置散文。
func TestParseReActResponse_XMLToolCallSingle(t *testing.T) {
	resp := "<tool_call name=\"list_metrics\">{\"datasource_id\": 10249, \"keyword\": \"cpu\"}</tool_call>"
	step := (&Agent{}).parseReActResponse(resp)
	if step.Action != "list_metrics" {
		t.Fatalf("Action = %q, want list_metrics", step.Action)
	}
	if step.ActionInput != `{"datasource_id": 10249, "keyword": "cpu"}` {
		t.Fatalf("ActionInput = %q", step.ActionInput)
	}
}

// 已有规范 Action 时，XML 归一化不得改写。
func TestNormalizeXMLToolCall_RespectsExistingAction(t *testing.T) {
	resp := "Thought: ok\nAction: list_datasources\nAction Input: {\"plugin_type\":\"prometheus\"}"
	if got := normalizeXMLToolCall(resp); got != resp {
		t.Fatalf("should be unchanged, got %q", got)
	}
}

// 无 XML 工具调用的普通最终回答原样返回。
func TestNormalizeXMLToolCall_NoXMLUnchanged(t *testing.T) {
	resp := "就是一段普通的最终回答，没有工具调用。"
	if got := normalizeXMLToolCall(resp); got != resp {
		t.Fatalf("should be unchanged, got %q", got)
	}
}

// 文档承诺：本函数只认 name 作为 XML 属性的形态；name 写在 JSON 内的 Nous/Hermes
// 形态、以及 Anthropic 的 <function_calls><invoke> 形态均不匹配、原样返回。
// 把这条承诺变成可回归断言，防止注释与正则再次漂移。
func TestNormalizeXMLToolCall_UnsupportedFormsUnchanged(t *testing.T) {
	hermes := `<tool_call>{"name":"list_metrics","arguments":{"datasource_id":10249}}</tool_call>`
	if got := normalizeXMLToolCall(hermes); got != hermes {
		t.Fatalf("Hermes 形态应原样返回，got %q", got)
	}
	anthropic := `<function_calls><invoke name="list_metrics">` +
		`<parameter name="datasource_id">10249</parameter></invoke></function_calls>`
	if got := normalizeXMLToolCall(anthropic); got != anthropic {
		t.Fatalf("Anthropic 形态应原样返回，got %q", got)
	}
}

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

// looksLikeToolCall 在 step.Action=="" 时区分"模型把工具调用以非 ReAct 形态泄漏到正文"
// （应纠正后重试）与"真正的最终答案"（应直接返回）。误判任一方向都有害：
// 漏判 → 工具调用被当最终答案静默终止；误判 → 正常答案被反复要求"改格式"。
func TestLooksLikeToolCall(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		// 复现线上 a3558c6f：裸 JSON 并行工具调用数组（截图原始形态）。
		{"parallel json array", "Thought: 先做两件事并行\n```json\n[ { \"name\": \"list_files\", \"arguments\": { \"base\": \"integrations/Linux\", \"path\": \"alerts\" } }, { \"name\": \"list_metrics\", \"arguments\": { \"datasource_id\": 10246, \"keyword\": \"cpu_usage\" } } ]\n```", true},
		{"single json object", `{"name": "list_metrics", "arguments": {"datasource_id": 10246}}`, true},
		{"parameters key variant", `{"name": "read_file", "parameters": {"path": "x"}}`, true},
		{"nous hermes xml tag", "<tool_call>{\"name\":\"list_files\",\"arguments\":{}}</tool_call>", true},
		{"anthropic native tags", "<function_calls><invoke name=\"list_files\"></invoke></function_calls>", true},
		{"openai envelope", `{"tool_calls": [{"id": "call_1"}]}`, true},

		// 真正的最终答案：不得误判。
		{"plain markdown answer", "## 结论\n已为业务组 60 创建 Linux CPU 告警规则。\n## 证据\n- cpu_usage_idle 在数据源 10246 有数据", false},
		{"answer mentioning the word arguments", "你的论点（arguments）已记录，但这不是工具调用。", false},
		{"answer with only a name key", `配置示例：{"name": "my-rule"}，无需调用工具。`, false},
		{"empty", "", false},
	}
	for _, c := range cases {
		if got := looksLikeToolCall(c.in); got != c.want {
			t.Errorf("%s: looksLikeToolCall = %v, want %v", c.name, got, c.want)
		}
	}
}
