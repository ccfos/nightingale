package router

import (
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
		if rt.tryResumePending(state, "s1", pending, nil, nil, "") {
			t.Fatalf("input pending must fall through to agent flow (content=%q)", content)
		}
	}
}
