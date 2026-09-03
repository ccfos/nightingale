package aiagent

import "fmt"

// 本文件实现通用人在环原语：任意内置工具可在执行中返回 *ToolInterrupt 表示
// "需要用户确认/输入才能继续"。运行时（loop + router）接力完成：
//
//	工具 propose 腿返回 ToolInterrupt
//	  → loop 立即停轮，把 Prompt 作为本轮答复（经 Done chunk 走既有 content 通道）
//	  → router 把 Pending（工具名 + 重放参数）持久化进 MessageExtra
//	  → 用户下一轮明确确认时，router 直接确定性重放工具 apply 腿——零 LLM 参与，
//	    确认环节不再依赖模型记忆/复述任何 id；明确拒绝则作废；其余回复回到正常
//	    agent 流程（旧提案由工具自身的 TTL/单次消费门兜底作废）。
//
// 对比旧约定（模型自己渲染 diff、自己在下一轮带 proposal_id+confirmed 重调工具）：
// 确认文案由工具确定性生成，确认动作由运行时确定性执行，模型只负责发起 propose。
type ToolInterrupt struct {
	// Kind: "approval"（确认后重放 ResumeArgs）/ "input"（需用户补充信息，带
	// Form 结构化表单；resume = 带补全的上下文重跑 agent，不重放——表单选择
	// 可能改变后续生成，重放陈旧参数会写错资源）。
	Kind string `json:"kind"`

	// Prompt 给用户看的确认文案（markdown，含改动 diff 与确认问句）。
	// 它会原样成为本轮的答复正文——前端格式不变，就是一条普通 markdown 消息。
	// input 类中断同时作为纯文本客户端（A2A）的表单兜底文案。
	Prompt string `json:"prompt"`

	// ResumeArgs 用户确认后重放本工具所需的完整参数 JSON（由工具在 propose 腿
	// 一次性备好，如 {"id":5,"proposal_id":"dbprop_x","confirmed":true}）。
	// 运行时不理解其内容，原样回传——通用契约，无需任何 per-tool 知识。
	// 仅 approval 类使用；input 类恒为空。
	ResumeArgs string `json:"resume_args"`

	// Form 仅 input 类：form_select 载荷 JSON（BuildCreationForm 产出，与
	// preflight 表单字节级同构）。router 把它渲染成 ContentTypeFormSelect
	// response——前端契约与 preflight 路径完全一致。
	Form string `json:"form,omitempty"`
}

const (
	InterruptKindApproval = "approval"
	InterruptKindInput    = "input"
)

func (t *ToolInterrupt) Error() string {
	return fmt.Sprintf("tool interrupt (%s): awaiting user response", t.Kind)
}
