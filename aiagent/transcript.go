package aiagent

import "strings"

// 本文件把对话状态从"只存最终答复文本"升级为"持久化/回放完整结构化 transcript"，
// 使工具产物（如 dashboard proposal_id）跨轮对模型可见。

// TranscriptSchemaVersion 是持久化 transcript 的 schema 版本。
// 历史对话不做向后兼容（旧会话可弃用），此字段仅用于向前演进。
const TranscriptSchemaVersion = 1

// TranscriptEnvelope 是落库的会话历史外壳：一个版本号 + 规范消息序列。
// 存进 models.AssistantMessageExtra.HistoryMessages（[]byte，json 编码）。
// 放在 aiagent 包而非 models 包，因为 aiagent 已 import models，反向会造成循环。
type TranscriptEnvelope struct {
	SchemaVersion int           `json:"schema_version"`
	Messages      []ChatMessage `json:"messages"`
}

// VisibleConversation 把完整 transcript 投影为"人类对话视图"：只保留对路由/摘要等
// 上层判断有意义的轮次——真实的用户消息、以及 assistant 的最终答复，过滤掉 ReAct
// 脚手架（"Observation:" 用户轮、含 "Action: <非 Final Answer>" 的中间工具调用轮）。
//
// 动机：history 包含 ReAct 中间轮，直接喂给上层消费者会让 "Observation: ..."
// 顶替真实的用户消息。注意：这是"投影"，canonical transcript 本身不被改写——
// 只有需要"人类视图"的消费者才用它。
func VisibleConversation(h []ChatMessage) []ChatMessage {
	out := make([]ChatMessage, 0, len(h))
	for _, m := range h {
		switch m.Role {
		case "user":
			// "Observation:" 前缀同时覆盖真实工具观测与格式纠正回灌
			// （reactFormatCorrection 也以 "Observation:" 开头）。
			if strings.HasPrefix(strings.TrimSpace(m.Content), "Observation:") {
				continue
			}
			out = append(out, m)
		case "assistant":
			// 跳过中间工具调用轮；保留最终答复（router 持久化的终态 assistant
			// 轮是 fullContent，不含行首 "Action:"）。
			if isReActToolTurn(m.Content) {
				continue
			}
			out = append(out, m)
		default:
			// system 等其它角色：历史里通常不出现（system 每轮重建），原样保留。
			out = append(out, m)
		}
	}
	return out
}

// isReActToolTurn 判断一个 assistant 轮是否为 ReAct 中间工具调用轮：含一行以
// "Action:" 开头且其值非 "Final Answer"。终态答复（fullContent）不含此行，故为 false。
//
// 故意只看 "Action:" 行而不调用 looksLikeToolCall：后者匹配 JSON/XML 工具形状，
// 用在 assistant 终答上可能误伤含示例 JSON 的正常答复。格式漂移产生的畸形工具轮
// （无 "Action:" 行）较罕见，且与被过滤的 "Observation:" 纠正轮成对出现，可接受。
func isReActToolTurn(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Action:") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(line, "Action:"))
		if v != "" && !strings.HasPrefix(v, ActionFinalAnswer) {
			return true
		}
	}
	return false
}
