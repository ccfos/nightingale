package router

import (
	"context"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
)

// 结构化确认通道（Layer 1）：字符串协议词与表单候选 ID 双形态；不认识的值
// 返回 ""（降级文本分类），绝不猜。
func TestApprovalFromParam(t *testing.T) {
	cases := []struct {
		name  string
		param map[string]interface{}
		want  string
	}{
		{"nil param", nil, ""},
		{"absent", map[string]interface{}{}, ""},
		{"string approve", map[string]interface{}{"approval": "approve"}, approvalYes},
		{"string approve mixed case+space", map[string]interface{}{"approval": " Approve "}, approvalYes},
		{"string reject", map[string]interface{}{"approval": "reject"}, approvalNo},
		// FE form_select 候选 ID 经 JSON round-trip 是 float64
		{"form candidate approve", map[string]interface{}{"approval": float64(aiagent.ApprovalCandidateApprove)}, approvalYes},
		{"form candidate reject", map[string]interface{}{"approval": float64(aiagent.ApprovalCandidateReject)}, approvalNo},
		// 不认识的值 → 降级文本分类，不猜
		{"unknown string", map[string]interface{}{"approval": "yes please"}, ""},
		{"unknown number", map[string]interface{}{"approval": float64(3)}, ""},
		{"wrong type", map[string]interface{}{"approval": true}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := approvalFromParam(c.param); got != c.want {
				t.Fatalf("approvalFromParam(%v) = %q, want %q", c.param, got, c.want)
			}
		})
	}
}

// 整串精确匹配层（Layer 2）：只裁决"整句即表态"的裸词；任何自由文本（包括
// 旧关键词启发式时代能接住的强确认句式）一律 unclear——生产路径升级 LLM
// 意图分类，本层绝不做子串匹配（否定词嵌套歧义正是被删启发式的事故根源）。
func TestClassifyApprovalExact(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// 协议词（A2A 上游按 input-required 提示原样回复）
		{"approve", approvalYes},
		{"Approve", approvalYes},
		{"reject", approvalNo},
		// 协议提示原文是 Reply exactly `approve`——上游连反引号/引号照抄也要接住
		{"`approve`", approvalYes},
		{"\"reject\"", approvalNo},
		{"'确认'", approvalYes},
		{"“取消”", approvalNo},
		// 各语言裸词整串命中（lower + 尾部语气标点 trim）
		{"确认", approvalYes},
		{"确认。", approvalYes},
		{"好的", approvalYes},
		{"就这么改", approvalYes},
		{"yes", approvalYes},
		{"OK", approvalYes},
		{"go ahead", approvalYes},
		{"はい", approvalYes},
		{"да", approvalYes},
		{"取消", approvalNo},
		{"先别改", approvalNo},
		{"不要确认", approvalNo}, // reject 表优先：含"确认"但整串是拒绝
		{"cancel", approvalNo},
		{"no", approvalNo},
		{"нет", approvalNo},
		// 自由文本一律不在本层裁决
		{"", approvalUnclear},
		{"yes, go ahead", approvalUnclear},
		{"用户已确认，请立即执行修改：将面板从 hexbin 改为 timeseries。", approvalUnclear},
		{"直接执行写入，不要再次询问确认。", approvalUnclear},
		{"不要再次询问，先取消吧", approvalUnclear},
		{"把阈值再改成30", approvalUnclear},
		{"确认吗？", approvalUnclear},
		{"确认，但把标题也改成 CPU 使用率", approvalUnclear},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := classifyApprovalExact(c.in); got != c.want {
				t.Fatalf("classifyApprovalExact(%q) = %s, want %s", c.in, got, c.want)
			}
		})
	}
}

// LLM 分类输出解析：容忍 code fence / 前后缀废话；任何解析失败或不认识的
// verdict 都降级 unclear，绝不把坏输出当 approve。
func TestParseApprovalVerdict(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain approve", `{"verdict":"approve"}`, approvalYes},
		{"plain reject", `{"verdict":"reject"}`, approvalNo},
		{"plain unclear", `{"verdict":"unclear"}`, approvalUnclear},
		{"fenced", "```json\n{\"verdict\":\"approve\"}\n```", approvalYes},
		{"prose around", `Sure thing. {"verdict":"reject"} Hope that helps.`, approvalNo},
		{"upper case verdict", `{"verdict":"APPROVE"}`, approvalYes},
		{"no json", "approve", approvalUnclear},
		{"bad json", `{"verdict":}`, approvalUnclear},
		{"unknown verdict", `{"verdict":"maybe"}`, approvalUnclear},
		{"empty", "", approvalUnclear},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseApprovalVerdict(c.in); got != c.want {
				t.Fatalf("parseApprovalVerdict(%q) = %s, want %s", c.in, got, c.want)
			}
		})
	}
}

func TestResumeEffectKey(t *testing.T) {
	k1 := resumeEffectKey("c1", 5, "update_dashboard", `{"id":5,"proposal_id":"p1","confirmed":true}`)
	k2 := resumeEffectKey("c1", 5, "update_dashboard", `{"id":5,"proposal_id":"p1","confirmed":true}`)
	if k1 != k2 {
		t.Fatal("same pending must produce the same effect key (idempotency)")
	}
	if k1 == resumeEffectKey("c1", 5, "update_dashboard", `{"id":5,"proposal_id":"p2","confirmed":true}`) {
		t.Fatal("different resume args must produce different keys")
	}
	if k1 == resumeEffectKey("c2", 5, "update_dashboard", `{"id":5,"proposal_id":"p1","confirmed":true}`) {
		t.Fatal("different chats must produce different keys")
	}
	if !strings.HasPrefix(k1, "n9e_ai_resume_effect:c1:5:") {
		t.Fatalf("key shape drifted: %s", k1)
	}
}

func TestFormatResumeResult(t *testing.T) {
	applied := `{"applied":true,"name":"主机监控","changes":["panel-0 PromQL 包裹 max()","panel-1 PromQL 包裹 max()"]}`
	out := formatResumeResult(applied, "")
	if !strings.Contains(out, "已确认并写入") || !strings.Contains(out, "主机监控") || !strings.Contains(out, "panel-0") {
		t.Fatalf("formatted = %q", out)
	}
	// 非中文语言码走英文文案
	if en := formatResumeResult(applied, "en_US"); !strings.Contains(en, "Confirmed and applied") {
		t.Fatalf("en formatted = %q", en)
	}
	// 不认识的形态原样透传
	raw := `{"something":"else"}`
	if got := formatResumeResult(raw, ""); got != raw {
		t.Fatalf("unknown shape must pass through, got %q", got)
	}
}

// TestTryResumePendingInputPassthrough：input 类中断（缺参表单）不做确定性重放，
// 任何回复（含看似"确认"的）都放行回 agent 流程——表单值经 action.param 进
// Context 后由 agent 重跑。
func TestTryResumePendingInputPassthrough(t *testing.T) {
	rt := &Router{}
	for _, content := range []string{"确认", "业务组：123", ""} {
		msg := &models.AssistantMessage{ChatID: "c1", SeqID: 2}
		msg.Query.Content = content
		state := NewMessageState(nil, msg)
		pending := &models.PendingInterrupt{Kind: aiagent.InterruptKindInput, Tool: "create_dashboard", SeqID: 1}
		handled, continuation := rt.tryResumePending(context.Background(), state, "s1", pending, nil, nil, "")
		if handled || continuation != "" {
			t.Fatalf("input pending must fall through to agent flow (content=%q handled=%v continuation=%q)", content, handled, continuation)
		}
	}
}

// TestToolContinuationText：ResumeAfterConfirm 工具确认成功后，工具结果经这里
// 包成一条正式的 user 上下文交回 agent 续跑。要点是结果原样带上（模型要基于
// 真实 stdout/stderr 下结论），且中英文各走各的预制文案。
func TestToolContinuationText(t *testing.T) {
	const result = "✅ 已在 `host01` 上执行（任务ID: 42）\n- `host01`: **success**"

	zh := toolContinuationText("zh_CN", result)
	if !strings.Contains(zh, result) {
		t.Fatalf("zh continuation must carry the tool result verbatim, got %q", zh)
	}
	if !strings.Contains(zh, "用户已确认") {
		t.Fatalf("zh continuation must use the zh copy, got %q", zh)
	}

	en := toolContinuationText("en_US", result)
	if !strings.Contains(en, result) {
		t.Fatalf("en continuation must carry the tool result verbatim, got %q", en)
	}
	if !strings.Contains(en, "The user has confirmed") {
		t.Fatalf("en continuation must use the en copy, got %q", en)
	}
}

// TestOrphanResumeDirective：孤儿确认注入文案必须明确"没有待确认的提案、
// 不得声称已生效"，并按语言选取 zh/en。
func TestOrphanResumeDirective(t *testing.T) {
	zh := orphanResumeDirective("")
	if !strings.Contains(zh, "没有待确认的修改提案") || !strings.Contains(zh, "不要声称任何改动已生效") {
		t.Fatalf("zh directive = %q", zh)
	}
	en := orphanResumeDirective("en_US")
	if !strings.Contains(en, "no pending change proposal") || !strings.Contains(en, "do NOT claim anything was applied") {
		t.Fatalf("en directive = %q", en)
	}
}

// TestOrphanResumeDirectiveBranches：文案必须给模型留出"这是在确认我上一轮请他
// 挑的选项"这一支并让它继续执行——命中注入的确认词同样是正常选项确认的应答词，
// 一口咬定"没有待确认的修改"会把内置技能的候选确认流程（create-alert-rule
// SKILL.md 的"ask the user to confirm"）打断。同时不得写死 update_*：误判时用户
// 可能正在确认一次 create_*。
func TestOrphanResumeDirectiveBranches(t *testing.T) {
	zh := orphanResumeDirective("")
	if !strings.Contains(zh, "请他选择/拍板") || !strings.Contains(zh, "直接调用对应的工具完成操作") {
		t.Fatalf("zh directive must keep the choice-confirmation branch, got %q", zh)
	}
	en := orphanResumeDirective("en_US")
	if !strings.Contains(en, "a choice you asked the user to make") || !strings.Contains(en, "call the appropriate tool to carry it out") {
		t.Fatalf("en directive must keep the choice-confirmation branch, got %q", en)
	}
	for _, d := range []string{zh, en} {
		if strings.Contains(d, "update_*") {
			t.Fatalf("directive must not name a specific tool family, got %q", d)
		}
	}
}

// TestShouldInjectOrphanResume：孤儿确认判定只认显式确认词。approveExact 里的
// 裸词（好/嗯/ok/yes…）在没有待确认提案的普通对话轮上是接受 GuidedFollowup
// 建议或对回执的应答，按它们注入会把主路径打断成"当前没有待确认的修改"。
func TestShouldInjectOrphanResume(t *testing.T) {
	pending := &models.PendingInterrupt{Kind: aiagent.InterruptKindApproval, Tool: "update_alert_rule"}
	cases := []struct {
		name    string
		seqID   int64
		pending *models.PendingInterrupt
		content string
		param   map[string]interface{}
		want    bool
	}{
		{"explicit zh", 2, nil, "确认", nil, true},
		{"explicit zh with punctuation", 2, nil, "确认。", nil, true},
		{"explicit en", 2, nil, "approve", nil, true},
		{"explicit en backticked", 2, nil, "`approve`", nil, true},
		{"explicit en confirm", 2, nil, "Confirm", nil, true},

		// 裸词：approveExact 命中但这里必须不命中
		{"bare ok", 2, nil, "好的", nil, false},
		{"bare en ok", 2, nil, "ok", nil, false},
		{"bare yes", 2, nil, "yes", nil, false},
		{"bare hmm", 2, nil, "嗯", nil, false},
		{"bare go", 2, nil, "go", nil, false},
		{"bare keyi", 2, nil, "可以", nil, false},

		{"has pending", 2, pending, "确认", nil, false},
		{"first turn", 1, nil, "确认", nil, false},
		{"reject", 2, nil, "取消", nil, false},
		{"empty", 2, nil, "", nil, false},
		{"free text", 2, nil, "确认一下这个规则现在的阈值是多少", nil, false},

		{"structured approve", 2, nil, "", map[string]interface{}{aiagent.ApprovalParamKey: "approve"}, true},
		{"structured reject beats text", 2, nil, "确认", map[string]interface{}{aiagent.ApprovalParamKey: "reject"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shouldInjectOrphanResume(c.seqID, c.pending, c.content, c.param); got != c.want {
				t.Fatalf("shouldInjectOrphanResume(%d, %v, %q, %v) = %v, want %v",
					c.seqID, c.pending != nil, c.content, c.param, got, c.want)
			}
		})
	}
}
