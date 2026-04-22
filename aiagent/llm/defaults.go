package llm

import "time"

// =============================================================================
// LLM 调优参数
//
// provider 独有的默认端点留在各自文件（DefaultOpenAIURL / DefaultClaudeURL /
// DefaultClaudeMaxTokens 等），这里只放跨 provider 共享的重试 & 超时参数。
// =============================================================================

const (
	// maxRetries 每次 LLM 调用在 rate limit / 5xx 上的最多重试次数（不含首次）
	maxRetries = 3

	// initialRetryWait 首次重试等待时间，遇到 429 时从这里起步（之后指数退避）
	initialRetryWait = 5 * time.Second

	// maxRetryWait 指数退避的上界
	maxRetryWait = 60 * time.Second

	// DefaultHTTPTimeout Config.Timeout 缺省值（毫秒）
	DefaultHTTPTimeout = 60000
)
