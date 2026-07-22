package aiagent

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
)

// 本文件实现上下文投影：canonical transcript（路由层持久化的完整历史）永不被
// 有损改写，喂给模型的只是它的一个"投影"。
//
// 观测截断（capObservation）是投影里唯一逐条生效的规则，两个调用点共用它：
// 本轮工具刚产出时按 LiveObservationCapBytes 截（native.go 的工具循环，只截
// 进 messages 的副本），跨轮回放时按更紧的 HistoryObservationCapBytes 再截一
// 次（下面第 1 步）。缺了前者，本轮里一次 search_code / 大查询就能把几百 KB
// 原文塞进 messages 并在剩余迭代中一路带着走——投影管不到还没进 History 的
// 消息。
//
// 其余两步只在跨轮投影时对 req.History 做：
//
//  1. 单条工具观测截断：超长观测（大查询结果等）截到 HistoryObservationCapBytes。
//     load_skill 的结果豁免——技能工作流被截断会直接破坏后续轮执行。
//  2. 旧观测清理：总量仍超预算时，从最旧的观测开始把正文替换为占位文本，保留
//     最近 HistoryKeepRecentObservations 条观测原文与 load_skill 结果。只清内容
//     不动结构——工具配对（assistant 调用轮 ↔ tool 结果轮）保持完整，模型仍能看
//     到"调过什么工具"，丢的只是陈旧观测正文（对齐 Anthropic context editing
//     clear_tool_uses 的零 LLM 确定性版：长会话的体积大头是工具结果，优先清
//     它们能保住对话骨架，比直接掐头丢整段消息损失小得多）。
//  3. 预算窗口：仍超预算时保留最新后缀，并保证窗口不以孤儿工具结果开窗——
//     工具结果必须跟在它的 assistant 调用之后，否则 provider 以配对不完整
//     拒绝（4xx）；优先从一条用户消息开始。被丢弃的部分以省略标记说明，
//     标记并入窗口首条 user 轮（独立成轮会破坏 user/assistant 严格交替）。
//
// 全部为纯确定性策略（零额外 LLM 成本）；LLM 摘要压缩可在其上替换第 3 步。

// projectHistory 返回 history 的投影。budget<=0 时用 DefaultHistoryBudgetBytes。
// 投影只用于本次 LLM 调用，绝不回写持久化历史。
func projectHistory(history []ChatMessage, budget int) []ChatMessage {
	if len(history) == 0 {
		return history
	}
	if budget <= 0 {
		budget = DefaultHistoryBudgetBytes
	}

	// 第 1 步：单条观测截断（拷贝后修改，不动调用方切片）。
	projected := make([]ChatMessage, len(history))
	copy(projected, history)
	total := 0
	for i := range projected {
		projected[i] = capObservation(projected[i], HistoryObservationCapBytes)
		total += msgSize(projected[i])
	}

	if total <= budget {
		return projected
	}

	// 第 2 步：旧观测清理——从最旧开始替换为占位文本，留够预算即停；最近
	// HistoryKeepRecentObservations 条观测始终保留原文（近期工具结果往往是
	// 本轮决策的直接依据）。
	var obs []int
	for i := range projected {
		if isCappableObservation(projected[i]) {
			obs = append(obs, i)
		}
	}
	if len(obs) > HistoryKeepRecentObservations {
		for _, i := range obs[:len(obs)-HistoryKeepRecentObservations] {
			if total <= budget {
				break
			}
			if len(clearedObservationNote) >= len(projected[i].Content) {
				continue // 原文已比占位文本短，清了不省反亏
			}
			total -= len(projected[i].Content) - len(clearedObservationNote)
			projected[i].Content = clearedObservationNote
		}
	}

	if total <= budget {
		return projected
	}

	// 第 3 步：预算窗口——从尾部向前累计，找到能放下的起点。
	start := len(projected)
	acc := 0
	for i := len(projected) - 1; i >= 0; i-- {
		if acc+msgSize(projected[i]) > budget && start < len(projected) {
			break
		}
		acc += msgSize(projected[i])
		start = i
	}

	// 边界修正：窗口必须从一条用户消息开始（跳过孤儿工具结果 / 无主 assistant 轮），
	// 最远推进到最后一条用户消息为止。
	lastUser := len(projected) - 1
	for lastUser > 0 && !isRealUserTurn(projected[lastUser]) {
		lastUser--
	}
	for start < lastUser && !isRealUserTurn(projected[start]) {
		start++
	}
	// 最后一条 user 之后的内容单独就超预算时（start 已越过 lastUser），上面
	// 的修正无从落点，窗口可能停在孤儿观测上——其 assistant 调用轮已被丢弃，
	// 以它开窗会被 provider 以配对不完整拒绝（4xx）。继续跳过观测轮直到合法落点。
	for start < len(projected)-1 && isObservationTurn(projected[start]) {
		start++
	}

	if start == 0 {
		return projected
	}

	marker := fmt.Sprintf("（系统提示：本会话较早的 %d 条消息因长度限制已省略，以下为最近的对话）", start)
	// 标记并入窗口首条 user 轮正文，而非独立成轮——独立插一条 user 消息会与
	// 它形成连续两条 user 轮，严格校验 user/assistant 交替的 provider/网关会
	// 4xx（converter 的连续 user 合并只覆盖 tool_result，不救纯文本）。
	// projected 是私有拷贝（第 1 步），原地改不影响调用方历史。
	if isRealUserTurn(projected[start]) {
		projected[start].Content = marker + "\n\n" + projected[start].Content
		return projected[start:]
	}
	// 窗口不以 user 开头（最后一条 user 之后单独超预算的兜底路径，落点是
	// assistant 轮）：前插独立 user 标记恰好补齐交替。
	out := make([]ChatMessage, 0, len(projected)-start+1)
	out = append(out, ChatMessage{Role: "user", Content: marker})
	out = append(out, projected[start:]...)
	return out
}

// msgSize 估算一条消息回放进请求的体积：正文 + 工具调用参数。assistant 工具
// 调用轮的 Arguments 会原样回放（编辑/导入大盘场景单条几十 KB），必须计入预算；
// 参数是 JSON、截不得，故只计量不截断。
func msgSize(m ChatMessage) int {
	n := len(m.Content)
	for _, tc := range m.ToolCalls {
		n += len(tc.Arguments)
	}
	return n
}

// clearedObservationNote 是观测轮被清理后的占位文本。模型据此知道"这里曾有一条
// 工具结果、需要时可重调工具"。
const clearedObservationNote = "(历史工具结果已因上下文长度限制清理；如需该数据请重新调用对应工具)"

// isObservationTurn 报告一条消息是否为工具观测轮（结构化 tool 结果轮）。
// 观测轮不能脱离其 assistant 调用轮独立开窗。
func isObservationTurn(m ChatMessage) bool {
	return m.Role == llm.RoleTool
}

// isCappableObservation 报告一条观测是否可截断。load_skill 结果豁免（截断会
// 破坏技能工作流）。
func isCappableObservation(m ChatMessage) bool {
	return m.Role == llm.RoleTool && m.ToolName != "load_skill"
}

// capObservation 返回 m 的投影副本：可截断的观测超过 limit 时截成首尾两段。
// 入参不被修改（ChatMessage 是值类型），canonical transcript 收原文。
func capObservation(m ChatMessage, limit int) ChatMessage {
	if !isCappableObservation(m) || len(m.Content) <= limit {
		return m
	}
	m.Content = truncateObservation(m.Content, limit)
	return m
}

// truncateObservation 把超长观测截成「提示头 + 正文头 + 中缝标记 + 正文尾」，
// 返回值总字节数不超过 limit（limit<=0 表示不限制，与 projectHistory 的 budget
// 语义一致）。
//
// 保留首尾而非只保留头部：头部承载关键产物（proposal_id 等 JSON 字段都在开
// 头），尾部承载收尾信息（search_code 的"已截断、请收窄 path 重试"提示、命令
// 退出码等）。只留头会把"被截掉的另一半"静默变成"结果就这些"，模型据此下结
// 论比看不到更危险。提示头写明原始字节数与行数，模型才知道该收窄条件重查而
// 不是接受这份残缺输出。切点对齐 UTF-8 边界，避免把中文切碎成乱码。
func truncateObservation(content string, limit int) string {
	if limit <= 0 || len(content) <= limit {
		return content
	}

	header := fmt.Sprintf("(观测过长已截断：原始 %d 字节 / %d 行，下面只保留首尾两段；需要完整内容请收窄查询条件后重新调用工具)\n",
		len(content), strings.Count(content, "\n")+1)
	// 省略量必然小于原文长度，用它算标记长度上界，保证最终不超 limit。
	const markerTpl = "\n...(中间省略 %d 字节)...\n"
	body := limit - len(header) - len(fmt.Sprintf(markerTpl, len(content)))
	if body <= 0 {
		// limit 小到放不下提示本身：退化为不带提示的安全前缀。
		return runePrefix(content, limit)
	}

	head := body / 2
	prefix := runePrefix(content, head)
	suffix := runeSuffix(content, body-head)
	return header + prefix + fmt.Sprintf(markerTpl, len(content)-len(prefix)-len(suffix)) + suffix
}

// runePrefix 返回 s 中不超过 n 字节、且不切碎 UTF-8 的最长前缀。
func runePrefix(s string, n int) string {
	if n >= len(s) {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// runeSuffix 返回 s 中不超过 n 字节、且不切碎 UTF-8 的最长后缀。
func runeSuffix(s string, n int) string {
	if n >= len(s) {
		return s
	}
	i := len(s) - n
	for i < len(s) && !utf8.RuneStart(s[i]) {
		i++
	}
	return s[i:]
}

// isRealUserTurn 报告一条消息是否为用户发言。
func isRealUserTurn(m ChatMessage) bool {
	return m.Role == "user"
}
