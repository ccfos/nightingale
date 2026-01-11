package models

// WorkflowNode 工作流节点
type WorkflowNode struct {
	ID       string      `json:"id"`                 // 节点唯一ID
	Name     string      `json:"name"`               // 显示名称
	Type     string      `json:"type"`               // 节点类型（对应 Processor typ）
	Position []float64   `json:"position,omitempty"` // [x, y] UI位置
	Config   interface{} `json:"config"`             // 节点配置

	// 执行控制
	Disabled       bool `json:"disabled,omitempty"`
	ContinueOnFail bool `json:"continue_on_fail,omitempty"`
	RetryOnFail    bool `json:"retry_on_fail,omitempty"`
	MaxRetries     int  `json:"max_retries,omitempty"`
	RetryInterval  int  `json:"retry_interval,omitempty"` // 秒
}

// Connections 节点连接关系 map[源节点ID]NodeConnections
type Connections map[string]NodeConnections

// NodeConnections 单个节点的输出连接
type NodeConnections struct {
	// Main 输出端口的连接
	// Main[outputIndex] = []ConnectionTarget
	Main [][]ConnectionTarget `json:"main"`
}

// ConnectionTarget 连接目标
type ConnectionTarget struct {
	Node  string `json:"node"`  // 目标节点ID
	Type  string `json:"type"`  // 输入类型，通常是 "main"
	Index int    `json:"index"` // 目标节点的输入端口索引
}

// EnvVariable 环境变量
type EnvVariable struct {
	Key         string `json:"key"`                   // 变量名
	Value       string `json:"value"`                 // 默认值
	Description string `json:"description,omitempty"` // 描述
	Secret      bool   `json:"secret,omitempty"`      // 是否敏感（日志脱敏）
	Required    bool   `json:"required,omitempty"`    // 是否必填
}

// NodeOutput 节点执行输出
type NodeOutput struct {
	WfCtx       *WorkflowContext `json:"wf_ctx"`                 // 处理后的工作流上下文
	Message     string           `json:"message"`                // 处理消息
	Terminate   bool             `json:"terminate"`              // 是否终止流程
	BranchIndex *int             `json:"branch_index,omitempty"` // 分支索引（条件节点使用）

	// 流式输出支持
	Stream     bool              `json:"stream,omitempty"` // 是否流式输出
	StreamChan chan *StreamChunk `json:"-"`                // 流式数据通道（不序列化）
}

// WorkflowResult 工作流执行结果
type WorkflowResult struct {
	Event       *AlertCurEvent         `json:"event"`        // 最终事件
	Status      string                 `json:"status"`       // success, failed, streaming
	Message     string                 `json:"message"`      // 汇总消息
	NodeResults []*NodeExecutionResult `json:"node_results"` // 各节点执行结果
	ErrorNode   string                 `json:"error_node,omitempty"`

	// 流式输出支持
	Stream     bool              `json:"stream,omitempty"` // 是否流式输出
	StreamChan chan *StreamChunk `json:"-"`                // 流式数据通道（不序列化）
}

// NodeExecutionResult 节点执行结果
type NodeExecutionResult struct {
	NodeID      string `json:"node_id"`
	NodeName    string `json:"node_name"`
	NodeType    string `json:"node_type"`
	Status      string `json:"status"` // success, failed, skipped
	Message     string `json:"message"`
	StartedAt   int64  `json:"started_at"`
	FinishedAt  int64  `json:"finished_at"`
	DurationMs  int64  `json:"duration_ms"`
	Error       string `json:"error,omitempty"`
	BranchIndex *int   `json:"branch_index,omitempty"` // 条件节点的分支选择
}

// 触发模式常量
const (
	TriggerModeEvent = "event" // 告警事件触发
	TriggerModeAPI   = "api"   // API 触发
	TriggerModeCron  = "cron"  // 定时触发（后续支持）
)

// WorkflowTriggerContext 工作流触发上下文
type WorkflowTriggerContext struct {
	// 触发模式
	Mode string `json:"mode"`

	// 触发者
	TriggerBy string `json:"trigger_by"`

	// 请求ID（API/Cron 触发使用）
	RequestID string `json:"request_id"`

	// 环境变量覆盖
	EnvOverrides map[string]string `json:"env_overrides"`

	// 流式输出（API 调用时动态指定）
	Stream bool `json:"stream"`

	// Cron 相关（后续使用）
	CronJobID   string `json:"cron_job_id,omitempty"`
	CronExpr    string `json:"cron_expr,omitempty"`
	ScheduledAt int64  `json:"scheduled_at,omitempty"`
}

type WorkflowContext struct {
	Event    *AlertCurEvent         `json:"event"`            // 当前事件
	Env      map[string]string      `json:"env"`              // 环境变量/配置（静态，来自 Pipeline 配置）
	Vars     map[string]interface{} `json:"vars"`             // 节点间传递的数据（动态，运行时产生）
	Metadata map[string]string      `json:"metadata"`         // 执行元数据（request_id、start_time 等）
	Output   map[string]interface{} `json:"output,omitempty"` // 输出结果（非告警场景使用）

	// 流式输出支持
	Stream     bool              `json:"-"` // 是否启用流式输出（不序列化）
	StreamChan chan *StreamChunk `json:"-"` // 流式数据通道（不序列化）
}

// SanitizedEnv 返回脱敏后的环境变量（用于日志和存储）
func (ctx *WorkflowContext) SanitizedEnv(secretKeys map[string]bool) map[string]string {
	sanitized := make(map[string]string)
	for k, v := range ctx.Env {
		if secretKeys[k] {
			sanitized[k] = "******"
		} else {
			sanitized[k] = v
		}
	}
	return sanitized
}

// StreamChunk 类型常量
const (
	StreamTypeThinking   = "thinking"    // AI 思考过程（ReAct Thought）
	StreamTypeToolCall   = "tool_call"   // 工具调用
	StreamTypeToolResult = "tool_result" // 工具执行结果
	StreamTypeText       = "text"        // LLM 文本输出
	StreamTypeDone       = "done"        // 完成
	StreamTypeError      = "error"       // 错误
)

// StreamChunk 流式数据块
type StreamChunk struct {
	Type      string      `json:"type"`                 // thinking / tool_call / tool_result / text / done / error
	Content   string      `json:"content"`              // 完整内容（累积）
	Delta     string      `json:"delta,omitempty"`      // 增量内容
	NodeID    string      `json:"node_id,omitempty"`    // 当前节点 ID
	RequestID string      `json:"request_id,omitempty"` // 请求追踪 ID
	Metadata  interface{} `json:"metadata,omitempty"`   // 额外元数据（如工具调用参数）
	Done      bool        `json:"done"`                 // 是否结束
	Error     string      `json:"error,omitempty"`      // 错误信息
	Timestamp int64       `json:"timestamp"`            // 时间戳(毫秒)
}
