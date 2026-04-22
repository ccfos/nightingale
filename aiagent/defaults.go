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
	DefaultMaxIterations     = 10
	DefaultTimeout           = 60000 // 60 秒（毫秒单位，和 llm.Config.Timeout 保持一致）
	DefaultMaxPlanSteps      = 10
	DefaultMaxReplanCount    = 2
	DefaultMaxStepIterations = 5
)

// 工具执行器相关
const (
	// HTTPStatusSuccessMax HTTP 工具视为成功的状态码上界（含），超出视为错误。
	HTTPStatusSuccessMax = 299

	// ToolOutputMaxBytes 工具（HTTP / MCP）单次返回给 LLM 的最大字节数。
	// 超出会在尾部追加 "... (truncated)" 截断，防止长响应占爆上下文 token。
	ToolOutputMaxBytes = 4000

	// FileReadMaxBytes read_file 内置工具的单文件读取上限。
	FileReadMaxBytes = 64 * 1024
)

// Stream cache 相关
const (
	// StreamTTL 未完成流的默认存活时间；完成流会缩短为 5 分钟。
	StreamTTL = time.Hour

	// StreamCleanupTick 后台清理过期流的轮询间隔。
	StreamCleanupTick = 5 * time.Minute
)
