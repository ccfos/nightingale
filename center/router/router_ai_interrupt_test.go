package router

import (
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
)

func TestClassifyApproval(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// 明确确认
		{"确认", approvalYes},
		{"确定", approvalYes},
		{"好的", approvalYes},
		{"可以", approvalYes},
		{"ok", approvalYes},
		{"OK", approvalYes},
		{"yes", approvalYes},
		{"确认。", approvalYes},
		{"确认无误", approvalYes},
		{"就这么改", approvalYes},
		{"确认提交", approvalYes},
		{"没问题", approvalYes},
		// 明确拒绝（注意「不要确认」必须judge为拒绝——reject 优先）
		{"取消", approvalNo},
		{"不对", approvalNo},
		{"先别改", approvalNo},
		{"算了", approvalNo},
		{"不要确认", approvalNo},
		{"cancel", approvalNo},
		// 语义不明 / 带新要求 → 回归 agent 流程
		{"", approvalUnclear},
		{"把阈值再改成30", approvalUnclear},
		{"确认吗？", approvalUnclear},
		{"等等，第一行改成平均值", approvalUnclear},
		{"顺便把内存图也加一条曲线", approvalUnclear},
		{"确认一下这个改动对生产环境有没有影响，评估完再说", approvalUnclear}, // 长句含「确认」但带新要求
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := classifyApproval(c.in); got != c.want {
				t.Fatalf("classifyApproval(%q) = %s, want %s", c.in, got, c.want)
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
	out := formatResumeResult(`{"applied":true,"name":"主机监控","changes":["panel-0 PromQL 包裹 max()","panel-1 PromQL 包裹 max()"]}`)
	if !strings.Contains(out, "已确认并写入") || !strings.Contains(out, "主机监控") || !strings.Contains(out, "panel-0") {
		t.Fatalf("formatted = %q", out)
	}
	// 不认识的形态原样透传
	raw := `{"something":"else"}`
	if got := formatResumeResult(raw); got != raw {
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
		if rt.tryResumePending(state, "s1", pending, nil, nil) {
			t.Fatalf("input pending must fall through to agent flow (content=%q)", content)
		}
	}
}
