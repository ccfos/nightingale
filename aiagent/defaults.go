package aiagent

import "time"

// =============================================================================
// 跨域默认值 / 调优参数
//
// 把原先散落在 types.go / tool_executor.go / tools/file.go / stream_cache.go 等
// 位置的魔术常量集中到此。改调优参数只需要改这里。
// =============================================================================

// Agent 运行期默认值（AgentConfig 字段缺省时使用）
const (
	DefaultMaxIterations = 25
	DefaultTimeout       = 60000 // 60 秒（毫秒单位，和 llm.Config.Timeout 保持一致）
)

// 工具执行器相关
const (
	// HTTPStatusSuccessMax HTTP 工具视为成功的状态码上界（含），超出视为错误。
	HTTPStatusSuccessMax = 299

	// ToolOutputMaxBytes 工具（HTTP / 外部工具源）单次返回给 LLM 的最大字节数。
	// 超出会在尾部追加 "... (truncated)" 截断，防止长响应占爆上下文 token。
	ToolOutputMaxBytes = 4000

	// FileReadMaxBytes read_file 内置工具的单文件读取上限。
	FileReadMaxBytes = 64 * 1024
)

// 上下文投影相关（见 context_manager.go）
const (
	// DefaultHistoryBudgetBytes 喂给模型的历史投影默认预算（字节）。粗略对应
	// 2~4 万 token（中英混排），给 system prompt / skills / 本轮产出留足余量。
	// 可经 AgentConfig.HistoryBudgetBytes 按 LLM 配置覆盖。
	DefaultHistoryBudgetBytes = 96 * 1024

	// LiveObservationCapBytes 本轮工具循环中单条观测进 messages 的上限。工具
	// 结果是模型下一步决策的直接依据，比历史观测宽松：取 FileReadMaxBytes，
	// 即"设计上产出最大的内置工具"的额度，这样按额度产出的工具（read_file 等）
	// 一字不少，只有输出无界的工具（search_code 在语料上实测可达 110KB）会被
	// 收口。没有这道门时，本轮观测完全不受约束——projectHistory 只作用于
	// req.History，管不到还没进历史的当前轮。
	LiveObservationCapBytes = FileReadMaxBytes

	// HistoryObservationCapBytes 历史中单条工具观测的截断上限。大查询结果在
	// 后续轮次只剩参考价值，比 LiveObservationCapBytes 更紧。
	HistoryObservationCapBytes = 16 * 1024

	// HistoryKeepRecentObservations 旧观测清理（投影第 2 步）保留原文的最近观测
	// 条数；更早的观测在超预算时被替换为占位文本（对齐 Anthropic context editing
	// clear_tool_uses 的 keep 语义）。
	HistoryKeepRecentObservations = 5
)

// Stream cache 相关
const (
	// StreamTTL 未完成流的默认存活时间；完成流会缩短为 5 分钟。
	StreamTTL = time.Hour

	// StreamCleanupTick 后台清理过期流的轮询间隔。
	StreamCleanupTick = 5 * time.Minute
)
