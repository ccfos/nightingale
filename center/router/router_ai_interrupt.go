package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/prom"

	"github.com/toolkits/pkg/logger"

	"github.com/ccfos/nightingale/v6/datasource"
)

// 人在环 resume。上一轮以 PendingInterrupt 收尾时，本轮用户回复在进入
// agent 流程之前先裁决 approve/reject/unclear，按通道分三层：
//   1. 结构化通道 action.param["approval"]（FE 按钮 / A2A metadata）——零 NLP；
//   2. 整串精确匹配（approve/reject 协议词 + 各语言裸词）；
//   3. LLM 意图三分类（自由文本兜底，多语言；任何失败降级 unclear）。
// 裁决后的处理：
//   - approve → 直接重放工具的 apply 腿（pending.ResumeArgs）——确认环节不
//     依赖模型记忆/复述任何 id，LLM（若参与）只裁决意图、不经手参数；
//   - reject → 作废，告知已取消；
//   - unclear（比如继续改需求）→ 回归正常 agent 流程，pending 不再携带，
//     旧提案由工具自身的 TTL / 单次消费 / 基线哈希门兜底作废。

const (
	approvalYes     = "approve"
	approvalNo      = "reject"
	approvalUnclear = "unclear"
)

// approvalFromParam 读结构化确认通道（Layer 1），返回 ""（未携带/值不认识）时
// 降级文本分类。值契约：字符串 "approve"/"reject"（A2A 上游），或表单候选 ID
// （FE 按钮，JSON round-trip 后是 float64）。不认识的值不猜。
func approvalFromParam(param map[string]interface{}) string {
	v, ok := param[aiagent.ApprovalParamKey]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "approve":
			return approvalYes
		case "reject":
			return approvalNo
		}
	case float64:
		switch int64(x) {
		case aiagent.ApprovalCandidateApprove:
			return approvalYes
		case aiagent.ApprovalCandidateReject:
			return approvalNo
		}
	}
	return ""
}

// approveExact / rejectExact 整串精确匹配（Layer 2，lower+trim 后整句比对）。
// 只收录"整句即表态"的裸词：整串匹配在任何语言里都没有"否定词嵌套"歧义——
// 子串匹配做不到（"不要再问"含"不要"、"don't ask again"含"don't"，正是已删除
// 的旧关键词启发式的线上事故根源）。本表不追求覆盖率，只为高频裸词省一次 LLM
// 调用；接不住的自由文本一律升级 LLM 意图分类。
var (
	approveExact = map[string]bool{
		"approve": true, "approved": true, // 协议词（A2A 上游按 input-required 提示原样回复）
		"确认": true, "确定": true, "对": true, "好": true, "好的": true,
		"可以": true, "可以的": true, "行": true, "嗯": true, "是": true, "是的": true,
		"同意": true, "提交": true, "执行": true, "执行吧": true, "改吧": true,
		"提交吧": true, "没问题": true, "就这么改": true, "确认无误": true,
		"确认修改": true, "确认提交": true,
		"ok": true, "okay": true, "yes": true, "yep": true, "y": true,
		"sure": true, "go": true, "go ahead": true, "do it": true,
		"confirm": true, "confirmed": true, "lgtm": true,
		"はい": true, "да": true,
	}
	rejectExact = map[string]bool{
		"reject": true, "rejected": true, "cancel": true, "canceled": true, "cancelled": true,
		"取消": true, "不": true, "不要": true, "不用": true, "不行": true,
		"不对": true, "别改": true, "先别": true, "先别改": true, "算了": true, "不要确认": true,
		"no": true, "n": true, "nope": true, "stop": true,
		"いいえ": true, "нет": true,
	}
)

// normalizeApprovalText 把用户回复归一成整串比对用的形态。两张确认词表
// （approveExact / orphanApproveExact）共用它，避免各写一份 trim 集后漂移。
// trim 集含反引号/引号：A2A 协议提示写的是 Reply exactly `approve`，上游
// LLM 把 exactly 理解为连反引号照抄时回复是 "`approve`"——自己的协议提示
// 必须自己接得住，否则白白多烧一次 LLM 分类调用。
func normalizeApprovalText(text string) string {
	t := strings.ToLower(strings.TrimSpace(text))
	return strings.Trim(t, "。.!！~ `\"'“”‘’「」")
}

// classifyApprovalExact 整串精确匹配层。命不中返回 unclear，由调用方决定是否
// 升级 LLM 分类。纯函数，可表测。reject 先查：两表撞词时拒绝优先。
func classifyApprovalExact(text string) string {
	t := normalizeApprovalText(text)
	if t == "" {
		return approvalUnclear
	}
	if rejectExact[t] {
		return approvalNo
	}
	if approveExact[t] {
		return approvalYes
	}
	return approvalUnclear
}

// approvalClassifyTimeout 限定单次意图分类调用：分类是确认轮的串行前置，不能
// 吃满普通 LLM 调用的超时预算——超时即降级 unclear 回 agent 流程。
const approvalClassifyTimeout = 15 * time.Second

// approvalClassifySystem 是意图分类的 system prompt。要点：任何语言；approve
// 必须是"无修改地执行原提案"；掺新要求/条件/提问 → unclear；同意混拒绝 →
// reject；拿不准永远 unclear——宁可多走一轮提案，不可误写库。
const approvalClassifySystem = `You are a strict consent classifier guarding a pending write operation in a monitoring system. The user was shown a proposed change and asked to confirm it. Classify the user's reply into exactly one verdict:

- "approve": the reply unambiguously consents to applying the proposed change AS-IS. Imperatives like "stop asking, just do it" / "不要再次询问，直接执行" are approve.
- "reject": the reply declines or cancels the change.
- "unclear": anything else — new requirements, modifications, conditions, questions, or uncertain intent.

Rules:
- The reply may be in any language.
- Consent mixed with ANY new requirement or modification -> "unclear" (the change must be re-proposed).
- Consent mixed with cancellation -> "reject".
- When in doubt -> "unclear". Never guess "approve".

Output ONLY the JSON {"verdict":"approve|reject|unclear"} with no other text.`

// classifyApprovalLLM 对精确匹配接不住的自由文本做 LLM 意图三分类（Layer 3，
// 多语言）。配置缺失、调用失败、输出不可解析一律降级 unclear——回归 agent
// 流程重提案，结构化/精确两层保持零 LLM 依赖。
func (rt *Router) classifyApprovalLLM(confirmPrompt, reply string) string {
	agent, err := models.AIAgentGetByUseCase(rt.Ctx, "chat")
	if err != nil {
		logger.Warningf("[Assistant] approval classify: load chat agent: %v", err)
	}
	llmCfg := rt.resolveChatLLMConfig(agent)
	if llmCfg == nil {
		return approvalUnclear
	}
	client, _, err := rt.chatLLMClient(llmCfg)
	if err != nil {
		logger.Warningf("[Assistant] approval classify: create LLM client: %v", err)
		return approvalUnclear
	}
	ctx, cancel := context.WithTimeout(context.Background(), approvalClassifyTimeout)
	defer cancel()
	out, err := llm.ChatWithSystem(ctx, client, approvalClassifySystem,
		fmt.Sprintf("Confirmation question shown to the user:\n%s\n\nUser reply:\n%s", confirmPrompt, reply))
	if err != nil {
		logger.Warningf("[Assistant] approval classify LLM call failed: %v", err)
		return approvalUnclear
	}
	return parseApprovalVerdict(out)
}

// parseApprovalVerdict 从分类输出提取 verdict。按首/末花括号截取以容忍 code
// fence / 前后缀废话；其余任何解析失败 → unclear。
func parseApprovalVerdict(out string) string {
	start := strings.Index(out, "{")
	end := strings.LastIndex(out, "}")
	if start < 0 || end <= start {
		return approvalUnclear
	}
	var v struct {
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal([]byte(out[start:end+1]), &v); err != nil {
		return approvalUnclear
	}
	switch strings.ToLower(strings.TrimSpace(v.Verdict)) {
	case "approve":
		return approvalYes
	case "reject":
		return approvalNo
	}
	return approvalUnclear
}

// tryResumePending 处理待确认中断。ctx 是本轮消息的 parentCtx——确认腿的工具
// 重放挂在它上面，用户点取消（/assistant/message/cancel）时正在等待的工具能
// 立即收手（如 dispatch_task_stateless 的执行结果轮询最长会等 300s）。
// 返回 (handled, continuation)：
//   - handled=true：本轮已被确定性处理完毕（含取消/终态回执），调用方直接 return；
//   - handled=false 且 continuation==""：回复语义不明（或 input 类），回归正常 agent 流程；
//   - handled=false 且 continuation!=""：确认成功且工具声明 ResumeAfterConfirm，
//     由调用方把 continuation 作为正式历史上下文注入 agent，让模型基于结果继续分析。
func (rt *Router) tryResumePending(ctx context.Context, state *MessageState, streamID string, pending *models.PendingInterrupt, history []aiagent.ChatMessage, prevRoute *models.ConversationRoute, lang string) (bool, string) {
	msg := state.Msg()
	if pending == nil {
		return false, ""
	}

	// input 类中断（缺参表单）不做确定性重放：
	// 表单选择可能改变后续生成（如换了数据源，PromQL 必须重写），重放陈旧参数会
	// 写错资源。resume = 带着补全的 Context（action.param 已合并）重跑 agent，
	// 路由由 AwaitingForm 延续信号接住。
	if pending.Kind == aiagent.InterruptKindInput {
		logger.Infof("[Assistant] pending input tool=%s: re-entering agent flow with enriched context (chat=%s seq=%d)",
			pending.Tool, msg.ChatID, msg.SeqID)
		return false, ""
	}

	// 三层裁决：结构化 param → 整串精确匹配 → LLM 意图分类（仅自由文本）。
	verdict := approvalFromParam(msg.Query.Action.Param)
	if verdict == "" {
		verdict = classifyApprovalExact(msg.Query.Content)
		if verdict == approvalUnclear && strings.TrimSpace(msg.Query.Content) != "" {
			// LLM 分类是串行前置（典型 1-3s，超时上限 15s）。写一条 CurStep 让
			// 轮询 /detail 的前端在慢路径上有具体状态可显示——CurStep 为空时
			// 前端兜底显示通用"生成中"，所以这只影响慢尾的文案具体度。
			state.Update(context.Background(), func(m *models.AssistantMessage) {
				m.CurStep = "Understanding your reply..."
			})
			verdict = rt.classifyApprovalLLM(pending.Prompt, msg.Query.Content)
		}
	}
	if verdict == approvalUnclear {
		logger.Infof("[Assistant] pending %s tool=%s: reply unclear, falling back to agent flow (chat=%s seq=%d)",
			pending.Kind, pending.Tool, msg.ChatID, msg.SeqID)
		return false, ""
	}

	var text string

	// 是否"确认后回接 agent 续跑"由提出确认的工具在 propose 腿声明的 flag 决定
	// （随 PendingInterrupt 持久化，见 ToolInterrupt.ResumeAfterConfirm），而非按
	// 工具名或静态工具定义判断——与"是否需要确认"同源同址，新增此类工具无需改路由。
	cont := pending.ResumeAfterConfirm

	if verdict == approvalNo {
		text = resumeText(lang,
			"好的，已取消本次改动，原配置保持不变。如需继续调整，直接说明新的要求即可。",
			"OK — the change has been cancelled and the original configuration is unchanged. To continue, just describe the new requirements.")
		logger.Infof("[Assistant] pending tool=%s rejected by user (chat=%s seq=%d)", pending.Tool, msg.ChatID, msg.SeqID)
	} else if cached, ok := rt.getResumeEffect(resumeEffectKey(msg.ChatID, pending.SeqID, pending.Tool, pending.ResumeArgs)); ok {
		// 效果台账命中：同一个 pending 已成功重放过
		// （比如落库后终态持久化失败、客户端重试确认）。幂等返回首次效果，
		// 绝不二次执行写操作。cont 工具则直接把首次效果喂回 agent 续跑分析，
		// 同样避免重复执行。
		if cont {
			logger.Infof("[Assistant] pending tool=%s already applied, feeding result back to agent (chat=%s seq=%d)",
				pending.Tool, msg.ChatID, msg.SeqID)
			return false, toolContinuationText(lang, cached)
		}
		text = formatResumeResult(cached, lang)
		logger.Infof("[Assistant] pending tool=%s already applied, served from effect ledger (chat=%s seq=%d)",
			pending.Tool, msg.ChatID, msg.SeqID)
	} else {
		// 重放参数由工具在 propose 腿备好；身份键覆盖为本轮（晚于提案轮的
		// 服务端门由此通过——确认确实发生在更晚一轮）。
		params := make(map[string]string, len(pending.Params)+3)
		for k, v := range pending.Params {
			params[k] = v
		}
		params["chat_id"] = msg.ChatID
		params["seq_id"] = fmt.Sprintf("%d", msg.SeqID)
		params["user_input"] = msg.Query.Content
		// 语言用本轮的（而非提案轮快照），confirm 腿的回执文案跟随当前 UI 语言。
		params["lang"] = lang

		out, handled, err := aiagent.ExecuteBuiltinTool(ctx, rt.buildToolDeps(), pending.Tool, params, pending.ResumeArgs)
		switch {
		case !handled:
			text = fmt.Sprintf(resumeText(lang,
				"确认失败：工具 %s 不可用。请重新发起修改。",
				"Confirmation failed: tool %s is unavailable. Please restart the modification."), pending.Tool)
		case err != nil:
			text = fmt.Sprintf(resumeText(lang,
				"确认失败：%v\n\n请重新发起修改。",
				"Confirmation failed: %v\n\nPlease restart the modification."), err)
		default:
			// 仅成功记账：瞬时失败保持可重试。
			rt.putResumeEffect(resumeEffectKey(msg.ChatID, pending.SeqID, pending.Tool, pending.ResumeArgs), out)
			if cont {
				// cont 工具确认腿已真正执行并拿到结果：不终结本轮，
				// 把结果作为正式上下文交回 agent 继续分析（模型基于结果产出最终结论）。
				logger.Infof("[Assistant] pending tool=%s approved, feeding result back to agent (chat=%s seq=%d)",
					pending.Tool, msg.ChatID, msg.SeqID)
				return false, toolContinuationText(lang, out)
			}
			text = formatResumeResult(out, lang)
		}
		logger.Infof("[Assistant] pending tool=%s approved, replayed deterministically (chat=%s seq=%d handled=%v err=%v)",
			pending.Tool, msg.ChatID, msg.SeqID, handled, err)
	}

	responses := []models.AssistantMessageResponse{{
		ContentType: models.ContentTypeMarkdown,
		Content:     text,
	}}
	rt.finishHaltedMessage(state, streamID, history, responses, prevRoute, text)
	return true, ""
}

// toolContinuationText 构造把工具执行结果注入 agent 续跑的 user input 载荷。
// 通过 aiagent.LangText 做中英文案选取，符合 router 其余回执的一致性约定。
func toolContinuationText(lang, result string) string {
	return fmt.Sprintf(aiagent.LangText(lang,
		"\n\n用户已确认，操作已执行完毕。请基于以下执行结果，结合原始请求给出结论（例如：需要处理的对象及理由、下一步建议）。\n\n执行结果：\n%s",
		"\n\nThe user has confirmed and the operation has completed. Based on the execution result below and the original request, give a conclusion (for example: items to address with reasons, next-step suggestions).\n\nResult:\n%s"), result)
}

// resumeText 选取确定性 resume 路径的预制文案：这些文案不经 LLM、无法逐语言
// 生成。语言选取规则（zh 默认 / en 兜底）统一在 aiagent.LangText。
func resumeText(lang, zh, en string) string {
	return aiagent.LangText(lang, zh, en)
}

// orphanApproveExact 是"孤儿确认"判定专用的确认词表，只收无歧义的显式确认词。
// 不复用 approveExact：那张表里的裸词（好/嗯/对/行/ok/yes/go…）只有在
// prevPending != nil——上一轮刚问过"确不确认"——的强上下文里才等价于同意。孤儿
// 判定发生在没有待确认提案的普通对话轮上，而 GuidedFollowup 要求每轮答案末尾
// 都追问一句"下一步"（见 aiagent/prompts/guided_followup.md），用户回"好"是接受
// 建议，不是确认写操作；确认腿成功后的回执轮同样不带 pending，用户回"好的"也会
// 落到这里。按裸词注入会让这两条主路径都被"当前没有待确认的修改"打断。
//
// 收窄不损失真阳性：要拦的那条伪造路径本身就在教用户回"确认"——伪造文案抄的是
// 工具的确认文案，A2A 提示是 Reply exactly `approve`，FE 按钮是 BuildApprovalForm
// 的"确认执行"/"Apply"。真确认只会落在显式词上。
var orphanApproveExact = map[string]bool{
	"确认": true, "确认修改": true, "确认提交": true, "确认无误": true,
	"同意": true, "就这么改": true,
	"approve": true, "approved": true, "confirm": true, "confirmed": true,
}

// shouldInjectOrphanResume 判定本轮是否为"孤儿确认"：用户明确表达了确认，但上
// 一条消息没有待确认的提案。纯函数，可表测。
// seqID <= 1 直接否：首轮没有"上一轮"，此时的 confirm 不可能是在确认什么。
// 结构化通道优先于文本，与 tryResumePending 的分层裁决保持同一口径。
func shouldInjectOrphanResume(seqID int64, prevPending *models.PendingInterrupt, content string, param map[string]interface{}) bool {
	if seqID <= 1 || prevPending != nil {
		return false
	}
	switch approvalFromParam(param) {
	case approvalYes:
		return true
	case approvalNo:
		return false
	}
	return orphanApproveExact[normalizeApprovalText(content)]
}

// orphanResumeDirective 生成"孤儿确认"注入文案：用户明确回复确认意图，但上一条
// 消息没有待确认的提案（模型伪造提案文案而未调用工具、或提案已过期/被消费）。
//
// 这是**提示词层的 best-effort 约束**，不是确定性拦截：模型可以不遵守。真正不
// 依赖模型自觉的门在工具侧——提案的 TTL / 单次消费 / 基线哈希（见
// aiagent/tools/update_proposal.go 的 confirmUpdateGate），重放不会双写。这里只
// 是把"无提案"这个事实喂给模型，压低它编造成功回执的概率。
//
// 文案分两支而不是一口咬定"没有待确认的修改"：命中本注入的词
// （确认/同意/confirm…）同时也是正常对话里请用户拍板时的应答词——内置技能明确
// 要求模型在候选不唯一时"list them in your reply and ask the user to confirm"
// （create-alert-rule/SKILL.md），GuidedFollowup 又要求每轮结尾追问下一步，这些
// 轮次都不产生 PendingInterrupt。词表无法区分"伪造的暂存"和"正常的选项确认"，
// 所以把判别权交给看得见上文的模型，只把"不得声称已生效"设成两支共同的硬底线。
// 同理不写死 update_*：误判时用户可能正在确认一次 create_*，指名工具族会误导。
func orphanResumeDirective(lang string) string {
	return resumeText(lang,
		"\n\n【系统提示】用户回复了确认，但当前没有待确认的修改提案。请按以下口径处理：\n"+
			"- 如果用户确认的是你上一轮请他选择/拍板的内容（业务组、数据源、库表、方案等），据此继续，直接调用对应的工具完成操作；\n"+
			"- 如果用户确认的是你上一轮声称\"已暂存、待确认\"的改动，请如实告知用户当前没有待确认的修改（上一轮可能只输出了确认文案而实际未调用工具，或提案已失效/已被处理），并重新调用对应的工具提交新提案。\n"+
			"无论哪种情况，都不要声称任何改动已生效——除非本轮的工具调用确实返回了成功结果。",
		"\n\n[SYSTEM] The user replied with a confirmation, but there is no pending change proposal right now. Handle it as follows:\n"+
			"- If the confirmation refers to a choice you asked the user to make in the previous turn (business group, datasource, database/table, a proposed plan), go ahead and call the appropriate tool to carry it out.\n"+
			"- If the confirmation refers to a change you claimed was staged and awaiting approval, tell the user honestly that there is no pending change (the previous turn may only have printed the confirmation copy without actually calling the tool, or the proposal expired / was already consumed), then call the appropriate tool again to submit a fresh proposal.\n"+
			"Either way, do NOT claim anything was applied unless a tool call in THIS turn actually returned success.")
}

// formatResumeResult 把工具 apply 腿的 JSON 结果渲染成给用户看的 markdown；
// 不认识的形态原样返回。
func formatResumeResult(out, lang string) string {
	var r struct {
		Applied bool     `json:"applied"`
		Name    string   `json:"name"`
		Changes []string `json:"changes"`
	}
	if err := json.Unmarshal([]byte(out), &r); err == nil && r.Applied {
		var sb strings.Builder
		sb.WriteString(resumeText(lang, "✅ 已确认并写入", "✅ Confirmed and applied"))
		if r.Name != "" {
			sb.WriteString(resumeText(lang, "：", ": "))
			sb.WriteString(r.Name)
		}
		if len(r.Changes) > 0 {
			sb.WriteString("\n")
			for _, c := range r.Changes {
				sb.WriteString("\n- ")
				sb.WriteString(c)
			}
		}
		return sb.String()
	}
	return out
}

// ==================== Resume 效果台账（Step 5，I6 幂等） ====================

// resumeEffectTTL：一次确认效果的可幂等窗口。重试确认通常发生在秒~分钟级，
// 1 小时绰绰有余；过期后兜底是工具自身的单次消费门（重放会明确报错而非双写）。
const resumeEffectTTL = time.Hour

// resumeEffectKey 标识"同一个 pending 的同一次重放"：提案轮 + 工具 + 重放参数哈希。
func resumeEffectKey(chatID string, proposalSeq int64, tool, resumeArgs string) string {
	h := sha256.Sum256([]byte(tool + "\x00" + resumeArgs))
	return fmt.Sprintf("n9e_ai_resume_effect:%s:%d:%s", chatID, proposalSeq, hex.EncodeToString(h[:8]))
}

func (rt *Router) getResumeEffect(key string) (string, bool) {
	if rt.Redis == nil {
		return "", false
	}
	v, err := rt.Redis.Get(context.Background(), key).Result()
	if err != nil || v == "" {
		return "", false
	}
	return v, true
}

func (rt *Router) putResumeEffect(key, out string) {
	if rt.Redis == nil {
		return
	}
	if err := rt.Redis.Set(context.Background(), key, out, resumeEffectTTL).Err(); err != nil {
		logger.Warningf("[Assistant] persist resume effect %s: %v", key, err)
	}
}

// historyBudgetFromContextLength 把 LLM 配置的上下文长度（token）粗略折算成
// 历史投影预算（字节，中英混排按 ~3 字节/token 估，再留一半给 system/本轮）。
// 未配置时返回 0（用 aiagent.DefaultHistoryBudgetBytes）。
func historyBudgetFromContextLength(contextLength *int) int {
	if contextLength == nil || *contextLength <= 0 {
		return 0
	}
	return *contextLength * 3 / 2
}

// buildToolDeps 组装内置工具依赖（chat 主流程与 resume 重放共用同一构造）。
func (rt *Router) buildToolDeps() *aiagent.ToolDeps {
	return &aiagent.ToolDeps{
		DBCtx: rt.Ctx,
		// SkillsPath lets skill-authoring tools (create_skill / update_skill)
		// materialize a just-saved skill to disk immediately. In the main chat
		// flow InitSkills overwrites this with the absolute path; setting it here
		// also covers the resume/replay path (confirm leg of a write proposal),
		// which builds ToolDeps without going through InitSkills.
		SkillsPath:    rt.Center.AIAgent.SkillsPath,
		GetPromClient: func(dsId int64) prom.API { return rt.PromClients.GetCli(dsId) },
		GetSQLDatasource: func(dsType string, dsId int64) (datasource.Datasource, bool) {
			return dscache.DsCache.Get(dsType, dsId)
		},
		FilterDatasources:      rt.DatasourceCache.DatasourceFilter,
		GetAlertEvalLogs:       rt.getAlertEvalLogs,
		GetEventProcessingLogs: rt.getEventLogs,
		Redis:                  rt.Redis,
		Sandbox:                rt.Sandbox,
		IbexEnabled:            rt.Ibex.Enable,
		// Skill Gateway HTTP passthrough: loopback base for n9e's own API + a hook
		// to inject a freshly-created user token into the live auth cache so it
		// works immediately (skips the ~9s sync). 127.0.0.1 reaches our own
		// listener (bound 0.0.0.0); the sandbox can't reach it directly (SSRF floor).
		N9eAPIBaseURL: fmt.Sprintf("http://127.0.0.1:%d", rt.HTTP.Port),
		CacheUserToken: func(token string, user *models.User) {
			if rt.UserTokenCache != nil {
				rt.UserTokenCache.Inject(token, user)
			}
		},
	}
}
