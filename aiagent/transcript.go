package aiagent

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
