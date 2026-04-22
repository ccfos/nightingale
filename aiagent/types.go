package aiagent

import (
	"context"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/ccfos/nightingale/v6/aiagent/mcp"
	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/prom"
)

// ==================== 常量 ====================

const (
	// Agent 模式
	AgentModeReAct     = "react"      // ReAct 模式（默认）
	AgentModePlanReAct = "plan_react" // Plan + ReAct 混合模式

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
	StreamTypeText       = "text"
	StreamTypeDone       = "done"
	StreamTypeError      = "error"
	StreamTypePlan       = "plan"
	StreamTypeStep       = "step"
	StreamTypeSynthesis  = "synthesis"

	// 注：Agent 运行期默认值、HTTP 状态码上界等调优参数见 defaults.go。

	// ReAct 特殊标记
	ActionFinalAnswer = "Final Answer"
	ActionReplan      = "Replan"
	ActionStepComplete = "Step Complete"
)

// ==================== Agent 核心类型 ====================

// Agent 通用 AI Agent
type Agent struct {
	cfg *AgentConfig

	// LLM 客户端
	llmClient llm.LLM

	// Skills
	skillRegistry *SkillRegistry
	skillSelector *LLMSkillSelector

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
	AgentMode     string `json:"agent_mode,omitempty"`
	MaxIterations int    `json:"max_iterations"`
	Timeout       int    `json:"timeout"`
	OutputField   string `json:"output_field"`

	// Plan+ReAct 配置
	MaxPlanSteps      int `json:"max_plan_steps,omitempty"`
	MaxReplanCount    int `json:"max_replan_count,omitempty"`
	MaxStepIterations int `json:"max_step_iterations,omitempty"`

	// 可选能力
	Skills *SkillConfig `json:"skills,omitempty"`
	MCP    *mcp.Config  `json:"mcp,omitempty"`
	Stream bool         `json:"stream,omitempty"`

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
	Content    string         `json:"content"`          // 最终结果文本
	Steps      []ReActStep    `json:"steps"`            // 执行轨迹
	Plan       *ExecutionPlan `json:"plan,omitempty"`   // 执行计划（plan_react 模式）
	Iterations int            `json:"iterations"`       // 迭代次数
	Success    bool           `json:"success"`          // 是否成功
	Error      string         `json:"error,omitempty"`  // 错误信息
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
}

// BuiltinToolFunc 内置工具处理函数（不依赖 WorkflowContext）
type BuiltinToolFunc func(ctx context.Context, deps *ToolDeps, args map[string]interface{}, params map[string]string) (string, error)

// ExternalToolHandler 外部工具执行函数（用于 processor/skill 类型工具）
// 由适配层注入，核心 Agent 不关心具体实现
type ExternalToolHandler func(ctx context.Context, tool *AgentTool, args map[string]interface{}, req *AgentRequest) (string, error)

// ==================== ReAct 类型 ====================

// ReActStep ReAct 循环中的一步
type ReActStep struct {
	Thought     string `json:"thought"`
	Action      string `json:"action"`
	ActionInput string `json:"action_input"`
	Observation string `json:"observation"`
}

// ReActLoopConfig ReAct 循环配置
type ReActLoopConfig struct {
	MaxIterations  int
	TimeoutMessage string
	LogPrefix      string

	// 本次循环可见的工具集（runCtx.tools 快照）
	Tools []AgentTool

	// 流式支持（nil = 非流式）
	StreamChan chan *StreamChunk
	RequestID  string

	// 完成判断
	IsComplete           func(action string) bool
	ExtractPartialResult bool
}

// runCtx 单次 Run 的运行期状态（per-Run scope）
// 把 skills 和动态工具表从 AgentConfig 里剥离，让 Agent 可并发/重复 Run
type runCtx struct {
	skills []*SkillContent
	tools  []AgentTool // 静态（cfg.Tools）+ skill + MCP，按本次选中的 skills 装配
}

// ==================== Plan+ReAct 类型 ====================

// PlanStep 计划步骤
type PlanStep struct {
	StepNumber int    `json:"step_number"`
	Goal       string `json:"goal"`
	Approach   string `json:"approach"`
	Status     string `json:"status"`
	Summary    string `json:"summary"`
	Findings   string `json:"findings"`
	Error      string `json:"error"`

	ReActSteps []ReActStep `json:"react_steps,omitempty"`
	Iterations int         `json:"iterations"`
}

// ExecutionPlan 执行计划
type ExecutionPlan struct {
	TaskSummary string     `json:"task_summary"`
	Goal        string     `json:"goal"`
	FocusAreas  []string   `json:"focus_areas"`
	Steps       []PlanStep `json:"steps"`
	CurrentStep int        `json:"current_step"`
	ReplanCount int        `json:"replan_count"`
	Synthesis   string     `json:"synthesis"`
}

// ==================== LLM 消息类型 ====================

// ChatMessage OpenAI 格式的消息
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

