package aiagent

import (
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
)

func TestProjectHistory_UnderBudgetUnchanged(t *testing.T) {
	h := []ChatMessage{
		{Role: "user", Content: "改大盘"},
		{Role: "assistant", Content: "好的", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "get_dashboard_detail"}}},
		{Role: llm.RoleTool, ToolCallID: "c1", ToolName: "get_dashboard_detail", Content: "{}"},
		{Role: "assistant", Content: "已生成提案"},
	}
	got := projectHistory(h, 0)
	if len(got) != len(h) {
		t.Fatalf("under budget must keep everything: %d != %d", len(got), len(h))
	}
	for i := range h {
		if got[i].Content != h[i].Content || got[i].Role != h[i].Role {
			t.Fatalf("[%d] changed: %+v", i, got[i])
		}
	}
}

func TestProjectHistory_CapsOversizedObservation(t *testing.T) {
	big := `{"proposal_id":"dbprop_head","data":"` + strings.Repeat("x", HistoryObservationCapBytes*2) + `"}`
	h := []ChatMessage{
		{Role: "user", Content: "查一下"},
		{Role: llm.RoleTool, ToolCallID: "c1", ToolName: "query_prometheus", Content: big},
		{Role: "assistant", Content: "done"},
	}
	got := projectHistory(h, 10*1024*1024) // 预算充裕，只测截断
	if len(got[1].Content) > HistoryObservationCapBytes {
		t.Fatalf("not capped: %d bytes", len(got[1].Content))
	}
	if !strings.Contains(got[1].Content, "dbprop_head") {
		t.Fatal("head (with proposal_id) must survive the cap")
	}
	if !strings.Contains(got[1].Content, "已截断") {
		t.Fatal("missing truncation note")
	}
	// 原切片不被修改（投影不回写）
	if len(h[1].Content) <= HistoryObservationCapBytes {
		t.Fatal("projection must not mutate the canonical history")
	}
}

// 截断保留首尾两段：头部承载 proposal_id 之类的关键产物，尾部承载工具自己的
// 收尾提示（search_code 的"还有更多、请收窄"）。只留头会让模型把半截输出当成
// 全部结果。
func TestTruncateObservation_KeepsHeadAndTail(t *testing.T) {
	head := `{"proposal_id":"dbprop_head",`
	tail := `... (stopped at 100 matches — narrow with the path argument)`
	content := head + strings.Repeat("x", 64*1024) + tail

	got := truncateObservation(content, 8*1024)
	if len(got) > 8*1024 {
		t.Fatalf("result must fit the limit: %d > %d", len(got), 8*1024)
	}
	if !strings.Contains(got, "dbprop_head") {
		t.Error("head must survive")
	}
	if !strings.Contains(got, "narrow with the path argument") {
		t.Error("tail must survive")
	}
	if !strings.Contains(got, "原始") || !strings.Contains(got, "中间省略") {
		t.Errorf("truncation note must state the original size and the elision: %q", got[:200])
	}
	// 未超限的内容原样返回
	if short := "small"; truncateObservation(short, 8*1024) != short {
		t.Error("content within the limit must pass through untouched")
	}
}

// 切点必须落在 UTF-8 边界：语料与工具输出里大量中文，按字节硬切会切出乱码。
func TestTruncateObservation_RuneSafe(t *testing.T) {
	content := strings.Repeat("夜莺监控", 4096)
	for _, limit := range []int{512, 1024, 4096, 9000} {
		got := truncateObservation(content, limit)
		if len(got) > limit {
			t.Errorf("limit=%d: result %d bytes exceeds limit", limit, len(got))
		}
		if strings.ContainsRune(got, '�') {
			t.Errorf("limit=%d: truncation broke UTF-8", limit)
		}
	}
}

// limit 小到放不下提示文本时退化为安全前缀，绝不返回超过 limit 的内容；
// limit<=0 表示不限制。
func TestTruncateObservation_TinyLimit(t *testing.T) {
	content := strings.Repeat("夜莺", 1000)
	for _, limit := range []int{1, 3, 32, 200} {
		got := truncateObservation(content, limit)
		if len(got) > limit {
			t.Errorf("limit=%d: got %d bytes", limit, len(got))
		}
		if strings.ContainsRune(got, '�') {
			t.Errorf("limit=%d: broke UTF-8", limit)
		}
	}
	if got := truncateObservation(content, 0); got != content {
		t.Error("limit<=0 must mean unlimited")
	}
}

// capObservation 是 native 工具循环与 projectHistory 共用的那把尺子：豁免
// load_skill、不改入参、超限才动。
func TestCapObservation(t *testing.T) {
	big := strings.Repeat("y", LiveObservationCapBytes*2)

	obs := ChatMessage{Role: llm.RoleTool, ToolName: "search_code", Content: big}
	got := capObservation(obs, LiveObservationCapBytes)
	if len(got.Content) > LiveObservationCapBytes {
		t.Errorf("observation not capped: %d bytes", len(got.Content))
	}
	if len(obs.Content) != len(big) {
		t.Error("capObservation must not mutate its input")
	}

	skill := ChatMessage{Role: llm.RoleTool, ToolName: "load_skill", Content: big}
	if len(capObservation(skill, LiveObservationCapBytes).Content) != len(big) {
		t.Error("load_skill result must stay exempt")
	}

	user := ChatMessage{Role: "user", Content: big}
	if len(capObservation(user, LiveObservationCapBytes).Content) != len(big) {
		t.Error("non-observation messages must pass through")
	}
}

func TestProjectHistory_LoadSkillExempt(t *testing.T) {
	skill := "# Skill: x\n" + strings.Repeat("工作流", HistoryObservationCapBytes)
	h := []ChatMessage{
		{Role: "user", Content: "改大盘"},
		{Role: llm.RoleTool, ToolCallID: "c1", ToolName: "load_skill", Content: skill},
	}
	got := projectHistory(h, 10*1024*1024)
	if len(got[1].Content) != len(skill) {
		t.Fatal("load_skill result must be exempt from the observation cap")
	}
}

// TestProjectHistory_ClearsOldObservationsBeforeWindow：超预算时优先清旧观测正文
// （第 2 步），而不是直接掐头丢消息（第 3 步）——对话骨架（user/assistant 轮、工具
// 配对）完整保留，最近 HistoryKeepRecentObservations 条观测原文不动。
func TestProjectHistory_ClearsOldObservationsBeforeWindow(t *testing.T) {
	filler := strings.Repeat("x", 8*1024)
	var h []ChatMessage
	// 10 组完整轮，每组一条 8KB 观测；总量 ~80KB+
	for i := 0; i < 10; i++ {
		h = append(h,
			ChatMessage{Role: "user", Content: "问题"},
			ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c", Name: "get_x"}}},
			ChatMessage{Role: llm.RoleTool, ToolCallID: "c", ToolName: "get_x", Content: filler},
			ChatMessage{Role: "assistant", Content: "答"},
		)
	}
	// 预算 ~6 条观测原文的量：清掉最旧几条观测即可放下，不应触发窗口截断
	got := projectHistory(h, 52*1024)

	if len(got) != len(h) {
		t.Fatalf("clearing must preserve the conversation skeleton: %d != %d", len(got), len(h))
	}
	if strings.Contains(got[0].Content, "已省略") {
		t.Fatal("window elision must not trigger when clearing suffices")
	}

	var obsIdx []int
	for i, m := range got {
		if m.Role == llm.RoleTool {
			obsIdx = append(obsIdx, i)
		}
	}
	// 最旧的观测被清理为占位文本
	if !strings.Contains(got[obsIdx[0]].Content, "已因上下文长度限制清理") {
		t.Fatalf("oldest observation must be cleared, got %q", truncStr(got[obsIdx[0]].Content, 80))
	}
	// 最近 HistoryKeepRecentObservations 条观测原文保留
	for _, i := range obsIdx[len(obsIdx)-HistoryKeepRecentObservations:] {
		if got[i].Content != filler {
			t.Fatalf("recent observation [%d] must keep its original content", i)
		}
	}
	// canonical 历史不被回写
	for i := range h {
		if h[i].Role == llm.RoleTool && h[i].Content != filler {
			t.Fatal("projection must not mutate the canonical history")
		}
	}
}

// TestProjectHistory_ClearingExemptsLoadSkill：load_skill 结果是技能工作流本体，
// 清掉会直接破坏后续轮执行——豁免于旧观测清理。
func TestProjectHistory_ClearingExemptsLoadSkill(t *testing.T) {
	skill := "# Skill: x\n" + strings.Repeat("工作流", 4*1024)
	filler := strings.Repeat("y", 8*1024)
	h := []ChatMessage{
		{Role: "user", Content: "建仪表盘"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "s", Name: "load_skill"}}},
		{Role: llm.RoleTool, ToolCallID: "s", ToolName: "load_skill", Content: skill},
	}
	for i := 0; i < 8; i++ {
		h = append(h,
			ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c", Name: "get_x"}}},
			ChatMessage{Role: llm.RoleTool, ToolCallID: "c", ToolName: "get_x", Content: filler},
		)
	}
	// 总量 ≈ 37KB(skill) + 64KB(8×8KB 观测)；预算 80KB → 清最旧 3 条观测即可放下
	got := projectHistory(h, 80*1024)
	if got[2].ToolName != "load_skill" || got[2].Content != skill {
		t.Fatal("load_skill result must be exempt from old-observation clearing")
	}
	// 普通旧观测确实被清了（证明清理路径在工作，而不是预算根本没超）
	if !strings.Contains(got[4].Content, "已因上下文长度限制清理") {
		t.Fatalf("ordinary old observation must be cleared, got %q", truncStr(got[4].Content, 80))
	}
}

func TestProjectHistory_WindowStartsAtRealUserTurn(t *testing.T) {
	filler := strings.Repeat("a", 4*1024)
	var h []ChatMessage
	// 5 组完整轮：user → assistant(tool_calls) → tool → assistant(final)
	for i := 0; i < 5; i++ {
		h = append(h,
			ChatMessage{Role: "user", Content: "问题" + filler},
			ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c", Name: "get_x"}}},
			ChatMessage{Role: llm.RoleTool, ToolCallID: "c", ToolName: "get_x", Content: filler},
			ChatMessage{Role: "assistant", Content: "答" + filler},
		)
	}
	// 预算只够最后 ~1.5 组 → 窗口必须推进到一条真实 user 开头
	got := projectHistory(h, 24*1024)

	if len(got) >= len(h) {
		t.Fatalf("over budget must shrink: %d >= %d", len(got), len(h))
	}
	// 省略标记并入首条 user 轮，而非独立成轮——否则与窗口首条 user 形成
	// 连续两条 user 消息，严格校验交替的 provider 会 4xx
	if !strings.Contains(got[0].Content, "已省略") {
		t.Fatalf("first message must carry the elision marker, got %+v", got[0])
	}
	if !isRealUserTurn(got[0]) {
		t.Fatalf("window must start at a real user turn, got %+v", got[0])
	}
	for i := 1; i < len(got); i++ {
		if got[i].Role == "user" && got[i-1].Role == "user" {
			t.Fatalf("consecutive user turns at %d: %+v", i, got[i])
		}
	}
	// 工具结果配对完整：窗口里每条 tool 消息之前必须存在其 assistant 调用轮
	for i, m := range got {
		if m.Role == llm.RoleTool {
			paired := false
			for j := i - 1; j >= 1; j-- {
				if got[j].Role == "assistant" && len(got[j].ToolCalls) > 0 {
					paired = true
					break
				}
				if got[j].Role == "user" {
					break
				}
			}
			if !paired {
				t.Fatalf("orphan tool result at %d: %+v", i, m)
			}
		}
	}
}

// TestProjectHistory_MegaTurnSkipsOrphanObservations：最后一条真实 user 之后的
// 内容单独就超预算（一轮多工具调用的大观测）时，窗口不得以孤儿观测开窗——其
// assistant 调用轮已被丢弃，provider 会以配对不完整拒绝（4xx）。
func TestProjectHistory_MegaTurnSkipsOrphanObservations(t *testing.T) {
	filler := strings.Repeat("x", 15*1024)
	h := []ChatMessage{
		{Role: "user", Content: "改大盘"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "get_x"}, {ID: "c2", Name: "get_x"}, {ID: "c3", Name: "get_x"},
			{ID: "c4", Name: "get_x"}, {ID: "c5", Name: "get_x"}, {ID: "c6", Name: "get_x"},
		}},
		{Role: llm.RoleTool, ToolCallID: "c1", ToolName: "get_x", Content: filler},
		{Role: llm.RoleTool, ToolCallID: "c2", ToolName: "get_x", Content: filler},
		{Role: llm.RoleTool, ToolCallID: "c3", ToolName: "get_x", Content: filler},
		{Role: llm.RoleTool, ToolCallID: "c4", ToolName: "get_x", Content: filler},
		{Role: llm.RoleTool, ToolCallID: "c5", ToolName: "get_x", Content: filler},
		{Role: llm.RoleTool, ToolCallID: "c6", ToolName: "get_x", Content: filler},
		{Role: "assistant", Content: "done"},
	}
	got := projectHistory(h, 48*1024) // 预算放不下整轮观测
	if len(got) >= len(h) {
		t.Fatalf("over budget must shrink: %d >= %d", len(got), len(h))
	}
	if !strings.Contains(got[0].Content, "已省略") {
		t.Fatalf("first message must be the elision marker, got %+v", got[0])
	}
	if isObservationTurn(got[1]) {
		t.Fatalf("window must not start at an orphan observation: %+v", got[1])
	}
	// 兜底路径下标记独立成轮（落点是 assistant），仍须保持 user/assistant 交替
	for i := 1; i < len(got); i++ {
		if got[i].Role == "user" && got[i-1].Role == "user" {
			t.Fatalf("consecutive user turns at %d: %+v", i, got[i])
		}
	}
	// 窗口内任何 tool 结果都必须有在前的 assistant 调用轮
	for i, m := range got {
		if m.Role == llm.RoleTool {
			paired := false
			for j := i - 1; j >= 1; j-- {
				if got[j].Role == "assistant" && len(got[j].ToolCalls) > 0 {
					paired = true
					break
				}
			}
			if !paired {
				t.Fatalf("orphan tool result at %d: %+v", i, m)
			}
		}
	}
}

// TestProjectHistory_CountsToolCallArguments：assistant 工具调用轮的 Arguments
// 会原样回放（编辑/导入大盘场景单条几十 KB），必须计入预算——只数 Content 会让
// 投影实际体积数倍于预算。
func TestProjectHistory_CountsToolCallArguments(t *testing.T) {
	bigArgs := `{"payload":"` + strings.Repeat("y", 50*1024) + `"}`
	h := []ChatMessage{
		{Role: "user", Content: "导入大盘"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "import_dashboard", Arguments: bigArgs}}},
		{Role: llm.RoleTool, ToolCallID: "c1", ToolName: "import_dashboard", Content: `{"ok":true}`},
		{Role: "assistant", Content: "已导入"},
		{Role: "user", Content: "再看看"},
		{Role: "assistant", Content: "好的"},
	}
	got := projectHistory(h, 8*1024) // Content 总量远小于预算，Arguments 远超
	if len(got) >= len(h) {
		t.Fatal("tool-call arguments must count toward the budget (window must shrink)")
	}
	if !isRealUserTurn(got[0]) || !strings.Contains(got[0].Content, "再看看") {
		t.Fatalf("window must restart at the later real user turn, got %+v", got[0])
	}
}

func TestProjectHistory_NeverEmpty(t *testing.T) {
	h := []ChatMessage{
		{Role: "user", Content: strings.Repeat("超长", 64*1024)},
	}
	got := projectHistory(h, 1024)
	if len(got) == 0 {
		t.Fatal("projection must never return empty history")
	}
	last := got[len(got)-1]
	if !isRealUserTurn(last) {
		t.Fatalf("the last user turn must survive, got %+v", last)
	}
}
