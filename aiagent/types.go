package aiagent

import (
	"context"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/ccfos/nightingale/v6/aiagent/mcp"
	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/ccfos/nightingale/v6/pkg/sandbox"
	"github.com/ccfos/nightingale/v6/storage"
)

// ==================== 常量 ====================

const (
	// 工具类型
	ToolTypeHTTP      = "http"      // HTTP 请求工具
	ToolTypeBuiltin   = "builtin"   // 内置工具
	ToolTypeMCP       = "mcp"       // MCP 工具
	ToolTypeProcessor = "processor" // 夜莺处理器工具（需 ExternalToolHandler）
	ToolTypeSkill     = "skill"     // Skill 专用工具（需 ExternalToolHandler）

	// 流式数据类型
	StreamTypeThinking   = "thinking"
	StreamTypeToolCall   = "tool_call"
	StreamTypeToolResult = "tool_result"
	StreamTypeContent    = "content" // 正文 token，路由层归入 content 通道（非 reason）
	StreamTypeDone       = "done"
	StreamTypeError      = "error"
	// StreamTypeTranscript 携带本轮工具循环产生的规范消息（assistant 工具调用轮 + tool 结果轮），
	// 供路由层收集并持久化为下一轮可回放的结构化 transcript。仅在该 chunk 的 Transcript 字段非空。
	// 它不写入 stream bus，也不转发给 A2A，纯粹是 agent→router 的内部带外通道。
	StreamTypeTranscript = "transcript"
	// StreamTypeInterrupt：工具触发人在环中断（见 ToolInterrupt）。Content=确认
	// 文案，Metadata 携带 kind/tool/resume_args，路由层据此持久化 Pending 并结束
	// 本轮。同样是 agent→router 内部带外通道，不进 stream bus / A2A。
	StreamTypeInterrupt = "interrupt"

	// 注：Agent 运行期默认值、HTTP 状态码上界等调优参数见 defaults.go。
)

// ==================== Agent 核心类型 ====================

// Agent 通用 AI Agent
type Agent struct {
	cfg *AgentConfig

	// LLM 客户端
	llmClient llm.LLM

	// Skills
	skillRegistry *SkillRegistry

	// MCP
	mcpClientManager *mcp.ClientManager
	mcpServers       map[string]*mcp.ServerConfig

	// 外部工具处理器（用于 processor/skill 类型工具，由适配层注入）
	externalToolHandler ExternalToolHandler

	// 内置工具依赖（DBCtx、数据源获取器、过滤器等），由 WithToolDeps 注入
	toolDeps *ToolDeps
}

// AgentConfig Agent 配置（仅包含 Agent 行为相关字段，LLM 配置通过 WithLLMClient 注入）
type AgentConfig struct {
	// Agent 行为
	MaxIterations int    `json:"max_iterations"`
	Timeout       int    `json:"timeout"`
	OutputField   string `json:"output_field"`

	// 仅交互式 chat 在最终答案末尾追加"下一步建议"；workflow/事件路径开启会污染结构化输出，默认 false。
	GuidedFollowup bool `json:"guided_followup,omitempty"`

	// 可选能力
	Skills *SkillConfig `json:"skills,omitempty"`
	MCP    *mcp.Config  `json:"mcp,omitempty"`
	Stream bool         `json:"stream,omitempty"`

	// HistoryBudgetBytes 历史投影预算（字节），0 = DefaultHistoryBudgetBytes。
	// router 从 LLM 配置 ExtraConfig.context_length 粗略折算（token×3）。
	HistoryBudgetBytes int `json:"history_budget_bytes,omitempty"`

	// 用户提示词模板（支持 Go 模板语法，会被 text/template 解析）。
	// 适用于 adapter/processor 路径：用户在 JSON config 中显式写模板字符串，
	// 引用 {{.Params.X}} / {{.Event.Y}} 等变量。
	UserPromptTemplate string `json:"user_prompt_template"`

	// 已渲染好的用户提示词（按原样塞给 LLM，不经 text/template）。
	// 适用于 chat 路径：router 的 actionHandler.BuildPrompt 已经用 fmt.Sprintf
	// 把用户原文拼进去，此时再 parse 会把用户输入里的 {{ 当模板语法炸掉
	// （例如用户问 "告警模板怎么写 {{ .Alertname }}" 会让整轮对话 500）。
	//
	// 两者互斥：若 UserPromptRendered 非空优先用它。
	UserPromptRendered string `json:"-"`

	// 工具定义
	Tools []AgentTool `json:"tools"`
}

// AgentRequest Agent 执行请求
type AgentRequest struct {
	// 用户消息（核心输入）
	UserMessage string `json:"user_message"`

	// 通用参数（key-value，如 datasource_id, user_input 等）
	Params map[string]string `json:"params,omitempty"`

	// 运行时变量
	Vars map[string]interface{} `json:"vars,omitempty"`

	// 元数据（request_id 等）
	Metadata map[string]string `json:"metadata,omitempty"`

	// 多轮对话历史
	History []ChatMessage `json:"history,omitempty"`

	// 额外模板数据（用于 UserPromptTemplate 和 HTTP BodyTemplate 渲染兼容）
	// adapter 层填充 {"event": wfCtx.Event, "inputs": wfCtx.Inputs, "vars": wfCtx.Vars}，
	// 这些 key 会合并到模板渲染上下文中，保证旧模板（引用 .event/.inputs）继续工作
	TemplateExtra map[string]interface{} `json:"-"`

	// 流式输出通道（nil = 非流式）
	StreamChan chan *StreamChunk `json:"-"`

	// 父 context（用于调用方取消）
	ParentCtx context.Context `json:"-"`
}

// AgentResponse Agent 执行结果
type AgentResponse struct {
	Content    string     `json:"content"`         // 最终结果文本
	Steps      []ToolStep `json:"steps"`           // 执行轨迹
	Iterations int        `json:"iterations"`      // 迭代次数
	Success    bool       `json:"success"`         // 是否成功
	Error      string     `json:"error,omitempty"` // 错误信息

	// contentStreamed（仅包内）：流式模式下正文已逐 token 经 StreamTypeContent
	// 下发。executeNativeWithDone 据此给 Done chunk 打标，路由层只把 Done.Content
	// 当作解析/持久化用的权威正文，不再二次推流（防思考/回答整段重复）。
	contentStreamed bool
}

// StreamChunk Agent 自有的流式数据块
type StreamChunk struct {
	Type      string                 `json:"type"`
	Content   string                 `json:"content"`
	Delta     string                 `json:"delta,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	Timestamp int64                  `json:"timestamp"`
	Done      bool                   `json:"done,omitempty"`
	Error     string                 `json:"error,omitempty"`

	// Transcript 仅在 Type==StreamTypeTranscript 时设置：本轮新追加的规范消息
	// （按 wire 顺序，如 assistant 工具调用轮 + tool 结果轮）。
	Transcript []ChatMessage `json:"transcript,omitempty"`
}

// ==================== 工具类型 ====================

// AgentTool 工具定义
type AgentTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`

	// HTTP 工具配置
	URL           string            `json:"url,omitempty"`
	Method        string            `json:"method,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	BodyTemplate  string            `json:"body_template,omitempty"`
	Timeout       int               `json:"timeout,omitempty"`
	SkipSSLVerify bool              `json:"skip_ssl_verify,omitempty"`

	// 内部处理器工具配置（type=processor，由 ExternalToolHandler 处理）
	ProcessorType   string                 `json:"processor_type,omitempty"`
	ProcessorConfig map[string]interface{} `json:"processor_config,omitempty"`

	// Skill 工具配置（type=skill，由 ExternalToolHandler 处理）
	SkillName string `json:"skill_name,omitempty"`

	// MCP 工具配置
	MCPConfig *mcp.ToolConfig `json:"mcp_config,omitempty"`

	// 参数定义
	Parameters []ToolParameter `json:"parameters,omitempty"`
}

// ToolParameter 工具参数定义
type ToolParameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ToolDeps 内置工具的依赖集合
// 由宿主一次性构造并通过 WithToolDeps 注入 Agent，取代原先 builtin_tools.go 里一组包级可变变量。
// 所有 BuiltinToolFunc 都以第一等公民方式从形参拿依赖，不再读 package global。
type ToolDeps struct {
	DBCtx             *ctx.Context
	SkillsPath        string
	GetPromClient     func(dsId int64) prom.API
	GetSQLDatasource  func(dsType string, dsId int64) (datasource.Datasource, bool)
	FilterDatasources func([]*models.Datasource, *models.User) []*models.Datasource

	// 告警排障日志获取。由 center 路由注入：包装 alert-eval-detail / event-detail
	// 两个内部接口，用于排查"告警规则为什么没发告警"。两者都返回 (logs, instance, err)。
	GetAlertEvalLogs       func(ruleId string) ([]string, string, error)
	GetEventProcessingLogs func(eventHash string) ([]string, string, error)

	// Redis 用于读取主机心跳 (n9e_meta_update_time_*) 和 HostMeta (n9e_meta_*)。
	// host-health-diagnose skill 的实时态判断（BeatTime / Offset / CpuUtil / MemUtil）从这里来。
	Redis storage.Redis

	// Sandbox 是 Skill Python/Bash 脚本执行的隔离控制器（pkg/sandbox）。run_skill_script
	// 工具据此执行某 skill 的入口脚本。nil 或未启用时，工具回报「执行未开启」而非报错。
	Sandbox *sandbox.Sandbox

	// N9eAPIBaseURL 是 Skill Gateway 回环自调 n9e 自身 API 的基址（如
	// "http://127.0.0.1:17000"）。空则 Gateway 不启用。
	N9eAPIBaseURL string
	// CacheUserToken 把新建的 user token 即时注入 token 缓存（包装
	// memsto.UserTokenCache.Inject），让 Gateway 刚建的 token 当场可认证。
	CacheUserToken func(token string, user *models.User)
}

// BuiltinToolFunc 内置工具处理函数（不依赖 WorkflowContext）
type BuiltinToolFunc func(ctx context.Context, deps *ToolDeps, args map[string]interface{}, params map[string]string) (string, error)

// ExternalToolHandler 外部工具执行函数（用于 processor/skill 类型工具）
// 由适配层注入，核心 Agent 不关心具体实现
type ExternalToolHandler func(ctx context.Context, tool *AgentTool, args map[string]interface{}, req *AgentRequest) (string, error)

// ==================== 工具循环类型 ====================

// ToolStep 工具循环执行轨迹中的一步（思考 → 调工具 → 观测）。
// JSON 字段名是对外 API 的 wire 格式（沿用早期 ReAct 协议词汇），不可随类型改名。
type ToolStep struct {
	Thought     string `json:"thought"`
	Action      string `json:"action"`
	ActionInput string `json:"action_input"`
	Observation string `json:"observation"`
}

// ToolLoopConfig 工具循环配置
type ToolLoopConfig struct {
	MaxIterations  int
	TimeoutMessage string
	LogPrefix      string

	// 本次循环可见的工具集（runCtx.tools 快照）
	Tools []AgentTool

	// 流式支持（nil = 非流式）
	StreamChan chan *StreamChunk
	RequestID  string

	ExtractPartialResult bool

	// EmitTranscript 为真时，循环在每次向上下文追加工具调用轮/结果轮后，经
	// StreamChan 发一个 StreamTypeTranscript chunk，让路由层持久化本轮完整
	// transcript。仅顶层 chat（executeNative）置真。
	EmitTranscript bool
}

// runCtx 单次 Run 的运行期状态（per-Run scope）
// 把 skills 和动态工具表从 AgentConfig 里剥离，让 Agent 可并发/重复 Run
type runCtx struct {
	skills []*SkillContent
	tools  []AgentTool // 静态（cfg.Tools）+ skill + MCP，按本次选中的 skills 装配
}

// ==================== LLM 消息类型 ====================

// ChatMessage OpenAI 格式的消息
//
// 工具字段是"结构化工具轮"的统一表示（原生 function calling），与
// llm.Message 一一对应并随 transcript 持久化。
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`

	// ToolCalls 仅 assistant 轮：本轮发起的工具调用。
	ToolCalls []llm.ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID/ToolName 仅 role="tool" 结果轮（语义见 llm.Message）。
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`

	// ThinkingBlocks 仅 assistant 轮：Anthropic 系扩展思考块（带签名）。随
	// transcript 持久化，回放时由 provider 适配层回填——开启 thinking 后工具
	// 续轮的协议硬性要求（语义见 llm.ThinkingBlock）。
	ThinkingBlocks []llm.ThinkingBlock `json:"thinking_blocks,omitempty"`
}
