package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/prom"

	"github.com/toolkits/pkg/logger"

	"github.com/ccfos/nightingale/v6/datasource"
)

// 人在环 resume。上一轮以 PendingInterrupt 收尾时，本轮用户回复在进入
// agent 流程之前先走确定性裁决：
//   - 明确确认 → 直接重放工具的 apply 腿（pending.ResumeArgs），零 LLM 参与——
//     确认环节不再依赖模型记忆/复述任何 id；
//   - 明确拒绝 → 作废，告知已取消；
//   - 语义不明（比如继续改需求）→ 回归正常 agent 流程，pending 不再携带，
//     旧提案由工具自身的 TTL / 单次消费 / 基线哈希门兜底作废。

const (
	approvalYes     = "approve"
	approvalNo      = "reject"
	approvalUnclear = "unclear"
)

// approveExact / rejectMarkers / hesitationMarkers 故意保守：只把"明确无歧义"的
// 回复交给确定性路径，边界情况宁可多走一轮 agent（重新提案），不可误写库。
var (
	approveExact = map[string]bool{
		"确认": true, "确定": true, "对": true, "好": true, "好的": true,
		"可以": true, "可以的": true, "行": true, "嗯": true, "是": true, "是的": true,
		"同意": true, "提交": true, "执行": true, "执行吧": true, "改吧": true,
		"提交吧": true, "没问题": true, "就这么改": true, "确认无误": true,
		"确认修改": true, "确认提交": true, "ok": true, "okay": true, "yes": true,
		"y": true, "go": true, "lgtm": true,
	}
	approveMarkers   = []string{"确认", "同意", "提交", "没问题"}
	rejectMarkers    = []string{"取消", "不对", "不要", "别改", "先别", "算了", "不用", "不行", "cancel"}
	hesitateMarkers  = []string{"不", "别", "先", "吗", "?", "？", "再", "改成", "等", "换", "但", "一下"}
	maxApproveLength = 12 // 超过此长度的回复大概率带着新要求，交给 agent 处理
)

// antiAskIdioms 是"反追问"惯用语：字面带否定词（不要/不用/无需）但语义是强确认
// （"别问了，直接干"）。必须先于 rejectMarkers 剥离，否则"不要"子串把强确认误判
// 成拒绝——线上事故：A2A 上游 agent 发"直接执行写入，不要再次询问确认"被判
// reject，改动被取消。注意"不要确认"（真拒绝）不在此列：区分点是"再/再次/询问/
// 问"的追问语义。同族内长串在前，保证整体剥除。
var antiAskIdioms = []string{
	"不要再次询问确认", "不要再次询问", "不要再询问", "不要询问", "不要再问",
	"不要再次确认", "不要再确认",
	"无需再次确认", "无需再确认", "无需二次确认", "无需确认",
	"不用再次确认", "不用再确认", "不用确认", "不用再询问", "不用再问",
	"不需要再次确认", "不需要再确认", "不需要确认",
	"别再询问", "别再问", "别问了",
}

// strongApprovePrefixes 是"句首即表态"的确认形态。A2A 上游 agent 代用户确认时
// 常复述改动内容（"已确认，将面板从 hexbin 改为 timeseries"），长度远超
// maxApproveLength，不能因长度一刀切 unclear。
var strongApprovePrefixes = []string{
	"用户已确认", "已确认", "确认", "同意",
	"请立即执行", "请直接执行", "请执行", "立即执行", "直接执行",
}

// stripAntiAskIdioms 把反追问惯用语从回复中剥除，返回剥除后的文本和是否命中。
func stripAntiAskIdioms(t string) (string, bool) {
	found := false
	for _, idiom := range antiAskIdioms {
		if strings.Contains(t, idiom) {
			t = strings.ReplaceAll(t, idiom, "")
			found = true
		}
	}
	return strings.Trim(t, "，,。.、；; "), found
}

func hasStrongApprovePrefix(t string) bool {
	for _, p := range strongApprovePrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// classifyApproval 对"待确认中断"的用户回复做确定性三分类。纯函数，可表测。
func classifyApproval(text string) string {
	t := strings.ToLower(strings.TrimSpace(text))
	t = strings.Trim(t, "。.!！~ ")
	if t == "" {
		return approvalUnclear
	}

	// 反追问惯用语先剥离：它们字面含"不要/不用"，留着会被 rejectMarkers 误伤。
	stripped, hasAntiAsk := stripAntiAskIdioms(t)

	for _, m := range rejectMarkers {
		if strings.Contains(stripped, m) {
			return approvalNo
		}
	}
	if approveExact[stripped] {
		return approvalYes
	}

	hesitates := func() bool {
		for _, m := range hesitateMarkers {
			if strings.Contains(stripped, m) {
				return true
			}
		}
		return false
	}

	// 强确认句式：反追问惯用语在场，或句首即明确表态。此类回复常复述改动内容
	// （A2A 上游 agent 代确认），不设长度上限；但任何犹豫/新要求信号（hesitate）
	// 都让它回归 agent 流程——宁可多走一轮提案，不可误写库。
	if hasAntiAsk || hasStrongApprovePrefix(stripped) {
		if !hesitates() {
			return approvalYes
		}
		return approvalUnclear
	}

	if utf8.RuneCountInString(stripped) <= maxApproveLength {
		hasApprove := false
		for _, m := range approveMarkers {
			if strings.Contains(stripped, m) {
				hasApprove = true
				break
			}
		}
		if hasApprove && !hesitates() {
			return approvalYes
		}
	}
	return approvalUnclear
}

// tryResumePending 处理待确认中断。返回 true 表示本轮已被确定性处理完毕
// （已写终态、调用方直接 return）；false 表示回复语义不明，回归正常 agent 流程。
func (rt *Router) tryResumePending(state *MessageState, streamID string, pending *models.PendingInterrupt, history []aiagent.ChatMessage, prevRoute *models.ConversationRoute) bool {
	msg := state.Msg()

	// input 类中断（缺参表单）不做确定性重放：
	// 表单选择可能改变后续生成（如换了数据源，PromQL 必须重写），重放陈旧参数会
	// 写错资源。resume = 带着补全的 Context（action.param 已合并）重跑 agent，
	// 路由由 AwaitingForm 延续信号接住。
	if pending.Kind == aiagent.InterruptKindInput {
		logger.Infof("[Assistant] pending input tool=%s: re-entering agent flow with enriched context (chat=%s seq=%d)",
			pending.Tool, msg.ChatID, msg.SeqID)
		return false
	}

	verdict := classifyApproval(msg.Query.Content)
	if verdict == approvalUnclear {
		logger.Infof("[Assistant] pending %s tool=%s: reply unclear, falling back to agent flow (chat=%s seq=%d)",
			pending.Kind, pending.Tool, msg.ChatID, msg.SeqID)
		return false
	}

	var text string
	if verdict == approvalNo {
		text = "好的，已取消本次改动，原配置保持不变。如需继续调整，直接说明新的要求即可。"
		logger.Infof("[Assistant] pending tool=%s rejected by user (chat=%s seq=%d)", pending.Tool, msg.ChatID, msg.SeqID)
	} else if cached, ok := rt.getResumeEffect(resumeEffectKey(msg.ChatID, pending.SeqID, pending.Tool, pending.ResumeArgs)); ok {
		// 效果台账命中：同一个 pending 已成功重放过
		// （比如落库后终态持久化失败、客户端重试确认）。幂等返回首次效果，
		// 绝不二次执行写操作。
		text = formatResumeResult(cached)
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

		out, handled, err := aiagent.ExecuteBuiltinTool(context.Background(), rt.buildToolDeps(), pending.Tool, params, pending.ResumeArgs)
		switch {
		case !handled:
			text = fmt.Sprintf("确认失败：工具 %s 不可用。请重新发起修改。", pending.Tool)
		case err != nil:
			text = fmt.Sprintf("确认失败：%v\n\n请重新发起修改。", err)
		default:
			text = formatResumeResult(out)
			// 仅成功记账：瞬时失败保持可重试。
			rt.putResumeEffect(resumeEffectKey(msg.ChatID, pending.SeqID, pending.Tool, pending.ResumeArgs), out)
		}
		logger.Infof("[Assistant] pending tool=%s approved, replayed deterministically (chat=%s seq=%d handled=%v err=%v)",
			pending.Tool, msg.ChatID, msg.SeqID, handled, err)
	}

	responses := []models.AssistantMessageResponse{{
		ContentType: models.ContentTypeMarkdown,
		Content:     text,
	}}
	rt.finishHaltedMessage(state, streamID, history, responses, prevRoute, text)
	return true
}

// formatResumeResult 把工具 apply 腿的 JSON 结果渲染成给用户看的 markdown；
// 不认识的形态原样返回。
func formatResumeResult(out string) string {
	var r struct {
		Applied bool     `json:"applied"`
		Name    string   `json:"name"`
		Changes []string `json:"changes"`
	}
	if err := json.Unmarshal([]byte(out), &r); err == nil && r.Applied {
		var sb strings.Builder
		sb.WriteString("✅ 已确认并写入")
		if r.Name != "" {
			sb.WriteString("：")
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
		DBCtx:         rt.Ctx,
		GetPromClient: func(dsId int64) prom.API { return rt.PromClients.GetCli(dsId) },
		GetSQLDatasource: func(dsType string, dsId int64) (datasource.Datasource, bool) {
			return dscache.DsCache.Get(dsType, dsId)
		},
		FilterDatasources:      rt.DatasourceCache.DatasourceFilter,
		GetAlertEvalLogs:       rt.getAlertEvalLogs,
		GetEventProcessingLogs: rt.getEventLogs,
		Redis:                  rt.Redis,
	}
}
