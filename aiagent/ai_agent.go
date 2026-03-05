package aiagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/ccfos/nightingale/v6/aiagent/prompts"

	"github.com/toolkits/pkg/logger"
)

const (
	// 工具类型常量
	ToolTypeHTTP      = "http"      // HTTP 请求工具
	ToolTypeProcessor = "processor" // 夜莺内部处理器工具
	ToolTypeSkill     = "skill"     // Skill 专用工具（延迟加载）
	ToolTypeMCP       = "mcp"       // MCP (Model Context Protocol) 工具

	// Agent 模式常量
	AgentModeReAct     = "react"      // ReAct 模式（默认）- 适合简单/中等任务
	AgentModePlanReAct = "plan_react" // Plan + ReAct 混合模式 - 适合复杂任务

	// 默认值常量
	DefaultMaxIterations     = 10
	DefaultTimeout           = 60000 // 60秒
	DefaultMaxPlanSteps      = 10    // Plan 模式最大步骤数
	DefaultMaxReplanCount    = 2     // 最大重新规划次数
	DefaultMaxStepIterations = 5     // 每个步骤最大 ReAct 迭代次数
	DefaultMaxFindings       = 10    // 默认最大关键发现数量
	DefaultMaxHypotheses     = 5     // 默认最大假设数量
	DefaultMaxEvidence       = 20    // 默认最大证据数量

	// HTTP 状态码
	HTTPStatusSuccessMax = 299

	// ReAct 特殊标记
	ActionFinalAnswer = "Final Answer"

	// Plan and Execute 特殊标记
	ActionReplan = "Replan" // 重新规划

	// Plan+ReAct 流式模式专用 StreamType 常量（扩展 models.StreamType*）
	StreamTypePlan      = "plan"      // 规划阶段（计划生成）
	StreamTypeStep      = "step"      // 步骤执行开始/进度
	StreamTypeSynthesis = "synthesis" // 综合分析阶段
)

// AIAgentConfig AI Agent 处理器配置（根因分析 Agent）
// 支持 ReAct 和 Plan-Execute 两种架构模式
type AIAgentConfig struct {
	// LLM 配置
	Provider string            `json:"provider"` // LLM Provider: openai(默认), claude, gemini, ollama
	LLMURL   string            `json:"llm_url"`  // LLM API 地址（可选，有默认值）
	Model    string            `json:"model"`    // 模型名称
	APIKey   string            `json:"api_key"`  // API Key
	Headers  map[string]string `json:"headers"`  // 额外的请求头

	// Agent 模式：react（默认）或 plan_execute
	AgentMode string `json:"agent_mode,omitempty"`

	// 用户提示词模板（支持 Go 模板语法，用于构建分析请求）
	// 可用变量：{{.AlertContent}} - 告警内容
	UserPromptTemplate string `json:"user_prompt_template"`

	// 可用工具列表
	Tools []AgentTool `json:"tools"`

	// 执行控制
	MaxIterations int `json:"max_iterations"` // ReAct 最大迭代次数，默认 10
	Timeout       int `json:"timeout"`        // 超时时间(ms)，默认 60000

	// Plan+ReAct 模式配置
	MaxPlanSteps      int `json:"max_plan_steps,omitempty"`      // 最大计划步骤数，默认 10
	MaxReplanCount    int `json:"max_replan_count,omitempty"`    // 最大重新规划次数，默认 2
	MaxStepIterations int `json:"max_step_iterations,omitempty"` // 每个步骤最大 ReAct 迭代次数，默认 5

	// Memory 配置
	Memory *MemoryConfig `json:"memory,omitempty"` // 记忆配置

	// Skills 配置
	Skills *SkillConfig `json:"skills,omitempty"` // 技能配置

	// MCP 配置
	MCP *MCPConfig `json:"mcp,omitempty"` // MCP 服务器配置

	// 流式输出配置
	Stream bool `json:"stream,omitempty"` // 是否启用流式输出

	// 输出配置
	OutputField string `json:"output_field"` // 输出结果写入的字段名，默认 ai_analysis

	// HTTP 配置
	SkipSSLVerify bool   `json:"skip_ssl_verify"`
	Proxy         string `json:"proxy"`

	// 内部使用
	llmClient        llm.LLM                     `json:"-"` // 多 Provider LLM 客户端
	skillRegistry    *SkillRegistry              `json:"-"`
	skillSelector    *LLMSkillSelector           `json:"-"`
	mcpClientManager *MCPClientManager           `json:"-"`
	mcpServers       map[string]*MCPServerConfig `json:"-"` // 服务器名 -> 配置
}

// MemoryConfig 记忆系统配置
type MemoryConfig struct {
	// 是否启用工作记忆
	Enabled bool `json:"enabled"`

	// === 短期记忆配置（Working Memory）===
	// 在单次执行中保留的关键信息数量
	MaxFindings   int `json:"max_findings,omitempty"`   // 最大关键发现数量，默认 10
	MaxHypotheses int `json:"max_hypotheses,omitempty"` // 最大假设数量，默认 5
	MaxEvidence   int `json:"max_evidence,omitempty"`   // 最大证据数量，默认 20

	// 是否在每轮迭代中向 LLM 发送工作记忆摘要
	IncludeInPrompt bool `json:"include_in_prompt,omitempty"` // 默认 true
}

// WorkingMemory 工作记忆（单次执行中的短期记忆）
type WorkingMemory struct {
	// 关键发现列表
	KeyFindings []KeyFinding `json:"key_findings"`
	// 已尝试的假设
	TestedHypotheses []Hypothesis `json:"tested_hypotheses"`
	// 收集到的证据
	Evidence []Evidence `json:"evidence"`
}

// initWorkingMemory 初始化工作记忆
func (c *AIAgentConfig) initWorkingMemory() *WorkingMemory {
	if c.Memory == nil || !c.Memory.Enabled {
		return nil
	}
	return &WorkingMemory{
		KeyFindings:      make([]KeyFinding, 0),
		TestedHypotheses: make([]Hypothesis, 0),
		Evidence:         make([]Evidence, 0),
	}
}

// KeyFinding 关键发现
type KeyFinding struct {
	Content   string `json:"content"`   // 发现内容
	Source    string `json:"source"`    // 来源（工具名）
	Relevance string `json:"relevance"` // 相关性：high, medium, low
	Timestamp int64  `json:"timestamp"` // 发现时间
}

// Hypothesis 假设
type Hypothesis struct {
	Description string `json:"description"` // 假设描述
	Status      string `json:"status"`      // 状态：testing, confirmed, rejected
	Evidence    string `json:"evidence"`    // 支持/反驳的证据
}

// Evidence 证据
type Evidence struct {
	Type    string `json:"type"`    // 类型：metric, log, trace, config
	Content string `json:"content"` // 证据内容
	Source  string `json:"source"`  // 来源
}

// AgentTool Agent 可用工具定义
type AgentTool struct {
	Name        string `json:"name"`        // 工具名称（供 AI 调用）
	Description string `json:"description"` // 工具描述（告诉 AI 何时使用、返回什么）

	// 工具类型：http, processor, skill, mcp
	Type string `json:"type"`

	// HTTP 工具配置（type=http）
	URL           string            `json:"url,omitempty"`
	Method        string            `json:"method,omitempty"` // GET, POST 等
	Headers       map[string]string `json:"headers,omitempty"`
	BodyTemplate  string            `json:"body_template,omitempty"` // 请求体模板
	Timeout       int               `json:"timeout,omitempty"`
	SkipSSLVerify bool              `json:"skip_ssl_verify,omitempty"`

	// 内部处理器工具配置（type=processor）
	ProcessorType   string                 `json:"processor_type,omitempty"`   // 处理器类型，如 callback, relabel 等
	ProcessorConfig map[string]interface{} `json:"processor_config,omitempty"` // 处理器配置

	SkillName string `json:"skill_name,omitempty"` // Skill 工具配置（type=skill）

	MCPConfig *MCPToolConfig `json:"mcp_config,omitempty"` // MCP 工具配置（type=mcp） todo

	// 参数定义（告诉 AI 如何调用此工具）
	Parameters []ToolParameter `json:"parameters,omitempty"`
}

// ToolParameter 工具参数定义
type ToolParameter struct {
	Name        string `json:"name"`        // 参数名
	Type        string `json:"type"`        // 类型：string, number, boolean, object, array
	Description string `json:"description"` // 参数说明
	Required    bool   `json:"required"`    // 是否必需
}

// ReActStep ReAct 循环中的一步
type ReActStep struct {
	Thought     string `json:"thought"`      // AI 的思考过程
	Action      string `json:"action"`       // 决定执行的动作（工具名或 Final Answer）
	ActionInput string `json:"action_input"` // 动作输入参数
	Observation string `json:"observation"`  // 执行结果/观察
}

// PlanStep Plan+ReAct 模式中的计划步骤
type PlanStep struct {
	StepNumber int    `json:"step_number"` // 步骤序号
	Goal       string `json:"goal"`        // 此步骤的目标（要调查什么）
	Approach   string `json:"approach"`    // 调查方法/策略
	Status     string `json:"status"`      // 状态：pending, executing, completed, failed, skipped
	Summary    string `json:"summary"`     // 步骤执行摘要
	Findings   string `json:"findings"`    // 此步骤的关键发现
	Error      string `json:"error"`       // 错误信息

	// ReAct 执行记录
	ReActSteps []ReActStep `json:"react_steps,omitempty"` // 此步骤的 ReAct 迭代记录
	Iterations int         `json:"iterations"`            // ReAct 迭代次数
}

// ExecutionPlan 执行计划
type ExecutionPlan struct {
	// 计划阶段
	TaskSummary string   `json:"task_summary"` // 任务摘要
	Goal        string   `json:"goal"`         // 总体目标
	FocusAreas  []string `json:"focus_areas"`  // 关注领域

	// 执行阶段
	Steps       []PlanStep `json:"steps"`        // 计划步骤
	CurrentStep int        `json:"current_step"` // 当前执行到的步骤
	ReplanCount int        `json:"replan_count"` // 重新规划次数

	// 综合阶段
	Synthesis string `json:"synthesis"` // 综合分析结果
}

// AgentResult Agent 执行结果
type AgentResult struct {
	Analysis   string      `json:"analysis"`   // 最终分析结果
	Steps      []ReActStep `json:"steps"`      // ReAct 步骤记录
	Iterations int         `json:"iterations"` // 迭代次数
	Success    bool        `json:"success"`    // 是否成功完成
	Error      string      `json:"error"`      // 错误信息（如果有）

	// Plan-Execute 模式专用
	Plan *ExecutionPlan `json:"plan,omitempty"` // 执行计划

	// 工作记忆（短期记忆）
	WorkingMemory *WorkingMemory `json:"working_memory,omitempty"` // 工作记忆
}

// ChatMessage OpenAI 格式的消息
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse OpenAI 格式的响应
type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func init() {
	models.RegisterProcessor("ai.agent", &AIAgentConfig{})
}

func (c *AIAgentConfig) Init(settings interface{}) (models.Processor, error) {
	b, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	var result *AIAgentConfig
	if err = json.Unmarshal(b, &result); err != nil {
		return nil, err
	}

	result.applyDefaults()
	return result, nil
}

// NewAgent creates a new AIAgentConfig with defaults applied.
// Used by the query-generator handler to create agent instances directly.
func NewAgent(cfg *AIAgentConfig) *AIAgentConfig {
	cfg.applyDefaults()
	return cfg
}

// applyDefaults sets default values for the agent configuration
func (c *AIAgentConfig) applyDefaults() {
	if c.MaxIterations <= 0 {
		c.MaxIterations = DefaultMaxIterations
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	if c.OutputField == "" {
		c.OutputField = "ai_analysis"
	}
	if c.AgentMode == "" {
		c.AgentMode = AgentModeReAct
	}
	if c.MaxPlanSteps <= 0 {
		c.MaxPlanSteps = DefaultMaxPlanSteps
	}
	if c.MaxReplanCount <= 0 {
		c.MaxReplanCount = DefaultMaxReplanCount
	}
	if c.MaxStepIterations <= 0 {
		c.MaxStepIterations = DefaultMaxStepIterations
	}

	if c.Memory != nil && c.Memory.Enabled {
		if c.Memory.MaxFindings <= 0 {
			c.Memory.MaxFindings = DefaultMaxFindings
		}
		if c.Memory.MaxHypotheses <= 0 {
			c.Memory.MaxHypotheses = DefaultMaxHypotheses
		}
		if c.Memory.MaxEvidence <= 0 {
			c.Memory.MaxEvidence = DefaultMaxEvidence
		}
		if !c.Memory.IncludeInPrompt {
			c.Memory.IncludeInPrompt = true
		}
	}

	// MCP initialization
	if c.MCP != nil && len(c.MCP.Servers) > 0 {
		c.mcpClientManager = NewMCPClientManager()
		c.mcpServers = make(map[string]*MCPServerConfig)
		for i := range c.MCP.Servers {
			server := &c.MCP.Servers[i]
			c.mcpServers[server.Name] = server
		}
		logger.Infof("AI Agent MCP initialized: %d servers configured", len(c.MCP.Servers))
	}
}

// InitSkills initializes the skill registry from the given skills directory path
func (c *AIAgentConfig) InitSkills(skillsPath string) {
	if c.Skills == nil || skillsPath == "" {
		return
	}
	c.skillRegistry = NewSkillRegistry(skillsPath)
	if c.Skills.AutoSelect {
		c.skillSelector = NewLLMSkillSelector(func(ctx context.Context, messages []ChatMessage) (string, error) {
			return c.callLLM(ctx, messages)
		})
	}
	logger.Infof("AI Agent Skills initialized: path=%s, auto_select=%v", skillsPath, c.Skills.AutoSelect)
}

// SetSkillRegistry 设置技能注册表（用于测试或独立运行时注入）
func (c *AIAgentConfig) SetSkillRegistry(registry *SkillRegistry) {
	c.skillRegistry = registry

	// 如果启用 LLM 自动选择，初始化技能选择器
	if c.Skills != nil && c.Skills.AutoSelect && c.skillSelector == nil {
		c.skillSelector = NewLLMSkillSelector(func(ctx context.Context, messages []ChatMessage) (string, error) {
			return c.callLLM(ctx, messages)
		})
	}
}

func (c *AIAgentConfig) Process(ctxObj *ctx.Context, wfCtx *models.WorkflowContext) (*models.WorkflowContext, string, error) {
	event := wfCtx.Event

	// 创建带超时的 context
	timeoutCtx, cancel := context.WithTimeout(context.Background(), time.Duration(c.Timeout)*time.Millisecond)

	// === Skills 选择与加载 ===
	var activeSkills []*SkillContent
	if c.Skills != nil && c.skillRegistry != nil {
		activeSkills = c.selectAndLoadSkills(timeoutCtx, wfCtx)
		if len(activeSkills) > 0 {
			// 注册 skill tools 到可用工具列表
			c.registerSkillTools(activeSkills)
			logger.Debugf("AI Agent loaded %d skills", len(activeSkills))
		}
	}

	// === MCP 工具自动发现 ===
	// 使用独立的短超时，避免 MCP 连接失败阻塞主请求
	if c.mcpClientManager != nil && len(c.mcpServers) > 0 {
		mcpCtx, mcpCancel := context.WithTimeout(context.Background(), 10*time.Second)
		c.discoverMCPTools(mcpCtx)
		mcpCancel()
	}

	// === 流式输出模式 ===
	// 优先使用 wfCtx.Stream（API 动态指定），其次使用 c.Stream（静态配置）
	if wfCtx.Stream || c.Stream {
		// 流式模式：cancel 由 goroutine 完成后调用，避免提前取消 context
		return c.processWithStream(timeoutCtx, cancel, wfCtx, activeSkills)
	}

	// 非流式模式：确保函数返回时取消 context
	defer cancel()

	var result *AgentResult

	// 根据模式执行不同的 Agent 架构
	switch c.AgentMode {
	case AgentModePlanReAct:
		result = c.executePlanReActAgentWithSkills(timeoutCtx, wfCtx, activeSkills)
	default:
		// 默认使用 ReAct 模式
		result = c.executeReActAgentWithSkills(timeoutCtx, wfCtx, activeSkills)
	}

	// 将结果写入事件
	if event.AnnotationsJSON == nil {
		event.AnnotationsJSON = make(map[string]string)
	}
	event.AnnotationsJSON[c.OutputField] = result.Analysis

	// 保存完整的分析过程（可选，用于调试）
	if len(result.Steps) > 0 {
		stepsJSON, _ := json.Marshal(result.Steps)
		event.AnnotationsJSON[c.OutputField+"_steps"] = string(stepsJSON)
	}

	// 保存计划（Plan-Execute 模式）
	if result.Plan != nil {
		planJSON, _ := json.Marshal(result.Plan)
		event.AnnotationsJSON[c.OutputField+"_plan"] = string(planJSON)
	}

	// 保存工作记忆摘要
	if result.WorkingMemory != nil && len(result.WorkingMemory.KeyFindings) > 0 {
		memoryJSON, _ := json.Marshal(result.WorkingMemory)
		event.AnnotationsJSON[c.OutputField+"_memory"] = string(memoryJSON)
	}

	// 更新 Annotations 字段
	b, err := json.Marshal(event.AnnotationsJSON)
	if err != nil {
		return wfCtx, "", fmt.Errorf("failed to marshal annotations: %v", err)
	}
	event.Annotations = string(b)

	msg := fmt.Sprintf("AI Agent (%s mode) completed: %d iterations, success=%v", c.AgentMode, result.Iterations, result.Success)
	if result.Error != "" {
		return wfCtx, msg, fmt.Errorf("agent error: %s", result.Error)
	}

	return wfCtx, msg, nil
}

// initLLMClient 初始化 LLM 客户端
func (c *AIAgentConfig) initLLMClient() error {
	if c.llmClient != nil {
		return nil
	}

	// 自动检测 Provider（如果未指定）
	provider := c.Provider
	if provider == "" {
		if c.LLMURL != "" {
			provider = llm.DetectProvider(c.LLMURL)
		} else if c.Model != "" {
			provider = llm.DetectProviderFromModel(c.Model)
		} else {
			provider = llm.ProviderOpenAI
		}
	}

	cfg := &llm.Config{
		Provider:      provider,
		BaseURL:       c.LLMURL,
		APIKey:        c.APIKey,
		Model:         c.Model,
		Headers:       c.Headers,
		Timeout:       c.Timeout,
		SkipSSLVerify: c.SkipSSLVerify,
		Proxy:         c.Proxy,
	}

	client, err := llm.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	c.llmClient = client
	logger.Infof("AI Agent LLM client initialized: provider=%s, model=%s", provider, c.Model)
	return nil
}

// buildUserMessage 构建用户消息
func (c *AIAgentConfig) buildUserMessage(wfCtx *models.WorkflowContext) (string, error) {
	if c.UserPromptTemplate == "" {
		// 默认模板：构建根因分析请求
		return c.buildDefaultUserMessage(wfCtx)
	}

	// 使用自定义模板渲染
	var defs = []string{
		"{{$event := .Event}}",
		"{{$inputs := .Inputs}}",
	}

	text := strings.Join(append(defs, c.UserPromptTemplate), "")
	t, err := template.New("user_prompt").Funcs(template.FuncMap(tplx.TemplateFuncMap)).Parse(text)
	if err != nil {
		return "", fmt.Errorf("failed to parse user prompt template: %v", err)
	}

	var body bytes.Buffer
	if err = t.Execute(&body, wfCtx); err != nil {
		return "", fmt.Errorf("failed to execute user prompt template: %v", err)
	}

	return body.String(), nil
}

// buildDefaultUserMessage 构建默认用户消息
func (c *AIAgentConfig) buildDefaultUserMessage(wfCtx *models.WorkflowContext) (string, error) {
	event := wfCtx.Event
	var sb strings.Builder

	sb.WriteString("Please complete the task based on your instructions.\n\n")

	// 展示告警事件信息
	if event != nil {
		sb.WriteString("**Alert Information**:\n")
		sb.WriteString(fmt.Sprintf("- Alert Name: %s\n", event.RuleName))
		sb.WriteString(fmt.Sprintf("- Severity: %d\n", event.Severity))
		sb.WriteString(fmt.Sprintf("- Trigger Value: %s\n", event.TriggerValue))
		sb.WriteString(fmt.Sprintf("- Group: %s\n", event.GroupName))

		if len(event.TagsMap) > 0 {
			sb.WriteString("\n**Tags**:\n")
			for k, v := range event.TagsMap {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
			}
		}

		if len(event.AnnotationsJSON) > 0 {
			sb.WriteString("\n**Annotations**:\n")
			for k, v := range event.AnnotationsJSON {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
			}
		}
	}

	// 展示输入参数信息
	if len(wfCtx.Inputs) > 0 {
		sb.WriteString("\n**Context**:\n")
		for k, v := range wfCtx.Inputs {
			// 跳过敏感信息
			if strings.Contains(strings.ToLower(k), "key") ||
				strings.Contains(strings.ToLower(k), "secret") ||
				strings.Contains(strings.ToLower(k), "password") ||
				strings.Contains(strings.ToLower(k), "token") {
				continue
			}
			sb.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
		}
	}

	sb.WriteString("\nUse the available tools to gather information and complete the task.")
	return sb.String(), nil
}

// buildReActSystemPrompt 构建 ReAct 系统提示词
func (c *AIAgentConfig) buildReActSystemPrompt() string {
	var sb strings.Builder

	// 基础提示词（从嵌入文件加载）
	sb.WriteString(prompts.ReactSystemPrompt)
	sb.WriteString("\n\n")

	// 工具说明
	if len(c.Tools) > 0 {
		tools := c.convertToolsToInfo()
		sb.WriteString(llm.BuildToolsSection(tools))
	}

	// 环境信息
	sb.WriteString(llm.BuildEnvSection())

	return sb.String()
}

// convertToolsToInfo 将 AgentTool 转换为 llm.ToolInfo
func (c *AIAgentConfig) convertToolsToInfo() []llm.ToolInfo {
	tools := make([]llm.ToolInfo, len(c.Tools))
	for i, tool := range c.Tools {
		params := make([]llm.ToolParamInfo, len(tool.Parameters))
		for j, param := range tool.Parameters {
			params[j] = llm.ToolParamInfo{
				Name:        param.Name,
				Type:        param.Type,
				Description: param.Description,
				Required:    param.Required,
			}
		}
		tools[i] = llm.ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  params,
		}
	}
	return tools
}

// parseReActResponse 解析 LLM 响应，提取 Thought、Action、Action Input
func (c *AIAgentConfig) parseReActResponse(response string) ReActStep {
	step := ReActStep{}

	lines := strings.Split(response, "\n")
	var currentField string
	var currentValue strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Thought:") {
			if currentField != "" {
				c.setStepField(&step, currentField, strings.TrimSpace(currentValue.String()))
			}
			currentField = "thought"
			currentValue.Reset()
			currentValue.WriteString(strings.TrimPrefix(line, "Thought:"))
		} else if strings.HasPrefix(line, "Action:") {
			if currentField != "" {
				c.setStepField(&step, currentField, strings.TrimSpace(currentValue.String()))
			}
			currentField = "action"
			currentValue.Reset()
			currentValue.WriteString(strings.TrimPrefix(line, "Action:"))
		} else if strings.HasPrefix(line, "Action Input:") {
			if currentField != "" {
				c.setStepField(&step, currentField, strings.TrimSpace(currentValue.String()))
			}
			currentField = "action_input"
			currentValue.Reset()
			currentValue.WriteString(strings.TrimPrefix(line, "Action Input:"))
		} else if currentField != "" {
			currentValue.WriteString("\n")
			currentValue.WriteString(line)
		}
	}

	// 处理最后一个字段
	if currentField != "" {
		c.setStepField(&step, currentField, strings.TrimSpace(currentValue.String()))
	}

	return step
}

func (c *AIAgentConfig) setStepField(step *ReActStep, field, value string) {
	switch field {
	case "thought":
		step.Thought = value
	case "action":
		step.Action = value
	case "action_input":
		step.ActionInput = value
	}
}

// callLLM 调用 LLM（使用多 Provider 统一接口）
func (c *AIAgentConfig) callLLM(ctx context.Context, messages []ChatMessage) (string, error) {
	// 确保 LLM 客户端已初始化
	if err := c.initLLMClient(); err != nil {
		return "", err
	}

	// 转换消息格式
	llmMessages := make([]llm.Message, len(messages))
	for i, msg := range messages {
		llmMessages[i] = llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// 调用 LLM
	resp, err := c.llmClient.Generate(ctx, &llm.GenerateRequest{
		Messages: llmMessages,
	})
	if err != nil {
		return "", fmt.Errorf("LLM generate error: %w", err)
	}

	return resp.Content, nil
}

// executeTool 执行工具
func (c *AIAgentConfig) executeTool(ctx context.Context, toolName string, input string, wfCtx *models.WorkflowContext) string {
	// 1. 优先检查并执行内置工具
	if result, handled, err := ExecuteBuiltinTool(ctx, toolName, wfCtx, input); handled {
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return result
	}

	// 2. 查找配置的工具定义
	var tool *AgentTool
	for i := range c.Tools {
		if c.Tools[i].Name == toolName {
			tool = &c.Tools[i]
			break
		}
	}

	if tool == nil {
		return fmt.Sprintf("Error: tool '%s' not found", toolName)
	}

	// 解析输入参数
	var args map[string]interface{}
	if input != "" {
		if err := json.Unmarshal([]byte(input), &args); err != nil {
			// 如果不是 JSON，作为字符串参数处理
			args = map[string]interface{}{"input": input}
		}
	}

	// 3. 根据工具类型执行
	switch tool.Type {
	case ToolTypeHTTP:
		return c.executeHTTPTool(ctx, tool, args, wfCtx)
	case ToolTypeProcessor:
		return c.executeProcessorTool(ctx, tool, args, wfCtx)
	case ToolTypeSkill:
		return c.executeSkillTool(ctx, tool, args, wfCtx)
	case ToolTypeMCP:
		return c.executeMCPTool(ctx, tool, args, wfCtx)
	default:
		return fmt.Sprintf("Error: unsupported tool type '%s'", tool.Type)
	}
}

// executeHTTPTool 执行 HTTP 工具
func (c *AIAgentConfig) executeHTTPTool(ctx context.Context, tool *AgentTool, args map[string]interface{}, wfCtx *models.WorkflowContext) string {
	if tool.URL == "" {
		return "Error: tool URL not configured"
	}

	method := tool.Method
	if method == "" {
		method = "GET"
	}

	// 构建请求体
	var bodyReader io.Reader
	if tool.BodyTemplate != "" {
		// 使用模板渲染请求体
		templateData := map[string]interface{}{
			"args":   args,
			"event":  wfCtx.Event,
			"inputs": wfCtx.Inputs,
			"vars":   wfCtx.Vars,
		}

		t, err := template.New("tool_body").Funcs(template.FuncMap(tplx.TemplateFuncMap)).Parse(tool.BodyTemplate)
		if err != nil {
			return fmt.Sprintf("Error: failed to parse body template: %v", err)
		}

		var body bytes.Buffer
		if err = t.Execute(&body, templateData); err != nil {
			return fmt.Sprintf("Error: failed to execute body template: %v", err)
		}
		bodyReader = &body
	} else if method == "POST" || method == "PUT" {
		jsonBody, _ := json.Marshal(args)
		bodyReader = bytes.NewBuffer(jsonBody)
	}

	// 创建 HTTP 客户端
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: tool.SkipSSLVerify},
	}

	timeout := tool.Timeout
	if timeout <= 0 {
		timeout = 30000
	}

	client := &http.Client{
		Timeout:   time.Duration(timeout) * time.Millisecond,
		Transport: transport,
	}

	req, err := http.NewRequestWithContext(ctx, method, tool.URL, bodyReader)
	if err != nil {
		return fmt.Sprintf("Error: failed to create request: %v", err)
	}

	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range tool.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("Error: HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("Error: failed to read response: %v", err)
	}

	if resp.StatusCode > HTTPStatusSuccessMax {
		return fmt.Sprintf("HTTP error (status %d): %s", resp.StatusCode, string(body))
	}

	// 限制响应长度，避免上下文过长
	result := string(body)
	if len(result) > 4000 {
		result = result[:4000] + "\n... (truncated)"
	}

	return result
}

// executeProcessorTool 执行夜莺内部处理器工具
func (c *AIAgentConfig) executeProcessorTool(bgCtx context.Context, tool *AgentTool, args map[string]interface{}, wfCtx *models.WorkflowContext) string {
	if tool.ProcessorType == "" {
		return "Error: processor_type not configured"
	}

	// 合并配置和动态参数
	config := make(map[string]interface{})
	for k, v := range tool.ProcessorConfig {
		config[k] = v
	}
	for k, v := range args {
		config[k] = v
	}

	// 获取处理器
	processor, err := models.GetProcessorByType(tool.ProcessorType, config)
	if err != nil {
		return fmt.Sprintf("Error: failed to get processor '%s': %v", tool.ProcessorType, err)
	}

	// 复制事件和上下文，避免修改原数据
	event := wfCtx.Event
	eventCopy := *event
	wfCtxCopy := &models.WorkflowContext{
		Event:    &eventCopy,
		Inputs:   wfCtx.Inputs,
		Vars:     wfCtx.Vars,
		Metadata: wfCtx.Metadata,
	}
	ctxObj := &ctx.Context{}

	// 执行处理器
	resultWfCtx, msg, err := processor.Process(ctxObj, wfCtxCopy)
	if err != nil {
		return fmt.Sprintf("Error: processor execution failed: %v", err)
	}

	// 构建返回结果
	var result strings.Builder
	if msg != "" {
		result.WriteString(fmt.Sprintf("Message: %s\n", msg))
	}

	// 返回处理后事件的关键变化
	if resultWfCtx != nil && resultWfCtx.Event != nil {
		if resultWfCtx.Event.Annotations != event.Annotations {
			result.WriteString(fmt.Sprintf("Updated annotations: %s\n", resultWfCtx.Event.Annotations))
		}
		if resultWfCtx.Event.Tags != event.Tags {
			result.WriteString(fmt.Sprintf("Updated tags: %s\n", resultWfCtx.Event.Tags))
		}
	}

	if result.Len() == 0 {
		return "Processor executed successfully (no visible changes)"
	}

	return result.String()
}

// executeSkillTool 执行 Skill 专用工具（Level 3 按需加载）
func (c *AIAgentConfig) executeSkillTool(bgCtx context.Context, tool *AgentTool, args map[string]interface{}, wfCtx *models.WorkflowContext) string {
	if c.skillRegistry == nil {
		return "Error: skill registry not initialized"
	}

	if tool.SkillName == "" {
		return "Error: skill_name not specified for skill tool"
	}

	// Level 3: 按需加载工具定义
	skillTool, err := c.skillRegistry.LoadSkillTool(tool.SkillName, tool.Name)
	if err != nil {
		return fmt.Sprintf("Error: failed to load skill tool '%s': %v", tool.Name, err)
	}

	// 合并配置和动态参数
	config := make(map[string]interface{})
	for k, v := range skillTool.Config {
		config[k] = v
	}
	for k, v := range args {
		config[k] = v
	}

	// 使用加载的配置执行处理器
	processor, err := models.GetProcessorByType(skillTool.Type, config)
	if err != nil {
		return fmt.Sprintf("Error: failed to get processor '%s': %v", skillTool.Type, err)
	}

	// 复制事件和上下文，避免修改原数据
	event := wfCtx.Event
	eventCopy := *event
	wfCtxCopy := &models.WorkflowContext{
		Event:    &eventCopy,
		Inputs:   wfCtx.Inputs,
		Vars:     wfCtx.Vars,
		Metadata: wfCtx.Metadata,
	}
	ctxObj := &ctx.Context{}

	// 执行处理器
	resultWfCtx, msg, err := processor.Process(ctxObj, wfCtxCopy)
	if err != nil {
		return fmt.Sprintf("Error: skill tool execution failed: %v", err)
	}

	// 构建返回结果
	var result strings.Builder
	if msg != "" {
		result.WriteString(fmt.Sprintf("Message: %s\n", msg))
	}

	// 返回处理后事件的关键变化
	if resultWfCtx != nil && resultWfCtx.Event != nil {
		if resultWfCtx.Event.Annotations != event.Annotations {
			result.WriteString(fmt.Sprintf("Updated annotations: %s\n", resultWfCtx.Event.Annotations))
		}
		if resultWfCtx.Event.Tags != event.Tags {
			result.WriteString(fmt.Sprintf("Updated tags: %s\n", resultWfCtx.Event.Tags))
		}
	}

	if result.Len() == 0 {
		return "Skill tool executed successfully (no visible changes)"
	}

	return result.String()
}

// discoverMCPTools 自动发现 MCP 服务器提供的工具
func (c *AIAgentConfig) discoverMCPTools(ctx context.Context) {
	// 收集已存在的工具名
	existingTools := make(map[string]bool)
	for _, tool := range c.Tools {
		existingTools[tool.Name] = true
	}

	// 遍历所有 MCP 服务器
	for serverName, serverConfig := range c.mcpServers {
		// 获取或创建客户端
		client, err := c.mcpClientManager.GetOrCreateClient(ctx, serverConfig)
		if err != nil {
			logger.Warningf("Failed to connect to MCP server '%s': %v", serverName, err)
			continue
		}

		// 获取工具列表
		tools, err := client.ListTools(ctx)
		if err != nil {
			logger.Warningf("Failed to list tools from MCP server '%s': %v", serverName, err)
			continue
		}

		// 注册工具
		for _, mcpTool := range tools {
			// 跳过已存在的工具
			if existingTools[mcpTool.Name] {
				logger.Debugf("Skipping MCP tool '%s' (already exists)", mcpTool.Name)
				continue
			}

			// 构建参数列表
			var params []ToolParameter
			if mcpTool.InputSchema != nil {
				params = c.convertMCPSchemaToParams(mcpTool.InputSchema)
			}

			// 注册为 MCP 工具
			tool := AgentTool{
				Name:        mcpTool.Name,
				Description: mcpTool.Description,
				Type:        ToolTypeMCP,
				MCPConfig: &MCPToolConfig{
					ServerName: serverName,
					ToolName:   mcpTool.Name,
				},
				Parameters: params,
			}
			c.Tools = append(c.Tools, tool)
			existingTools[mcpTool.Name] = true

			logger.Debugf("Discovered MCP tool: %s (from server: %s)", mcpTool.Name, serverName)
		}

		logger.Infof("Discovered %d tools from MCP server '%s'", len(tools), serverName)
	}
}

// convertMCPSchemaToParams 将 MCP 工具的 JSON Schema 转换为 ToolParameter
func (c *AIAgentConfig) convertMCPSchemaToParams(schema map[string]interface{}) []ToolParameter {
	var params []ToolParameter

	// 获取 properties
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return params
	}

	// 获取必需字段
	requiredFields := make(map[string]bool)
	if required, ok := schema["required"].([]interface{}); ok {
		for _, r := range required {
			if name, ok := r.(string); ok {
				requiredFields[name] = true
			}
		}
	}

	// 遍历属性
	for name, propRaw := range properties {
		prop, ok := propRaw.(map[string]interface{})
		if !ok {
			continue
		}

		param := ToolParameter{
			Name:     name,
			Required: requiredFields[name],
		}

		// 获取类型
		if t, ok := prop["type"].(string); ok {
			param.Type = t
		}

		// 获取描述
		if desc, ok := prop["description"].(string); ok {
			param.Description = desc
		}

		params = append(params, param)
	}

	return params
}

// executeMCPTool 执行 MCP 工具
func (c *AIAgentConfig) executeMCPTool(ctx context.Context, tool *AgentTool, args map[string]interface{}, wfCtx *models.WorkflowContext) string {
	if tool.MCPConfig == nil {
		return "Error: mcp_config not configured"
	}

	if c.mcpClientManager == nil {
		return "Error: MCP client manager not initialized"
	}

	// 获取服务器配置
	serverConfig, ok := c.mcpServers[tool.MCPConfig.ServerName]
	if !ok {
		return fmt.Sprintf("Error: MCP server '%s' not found", tool.MCPConfig.ServerName)
	}

	// 获取或创建 MCP 客户端
	client, err := c.mcpClientManager.GetOrCreateClient(ctx, serverConfig)
	if err != nil {
		return fmt.Sprintf("Error: failed to connect to MCP server '%s': %v", tool.MCPConfig.ServerName, err)
	}

	// 调用 MCP 工具
	result, err := client.CallTool(ctx, tool.MCPConfig.ToolName, args)
	if err != nil {
		return fmt.Sprintf("Error: MCP tool call failed: %v", err)
	}

	// 格式化返回结果
	return c.formatMCPResult(result)
}

// formatMCPResult 格式化 MCP 工具调用结果
func (c *AIAgentConfig) formatMCPResult(result *MCPToolsCallResult) string {
	if result == nil {
		return "No result returned"
	}

	if result.IsError {
		var errMsg strings.Builder
		errMsg.WriteString("Error: ")
		for _, content := range result.Content {
			if content.Text != "" {
				errMsg.WriteString(content.Text)
			}
		}
		return errMsg.String()
	}

	var output strings.Builder
	for _, content := range result.Content {
		switch content.Type {
		case "text":
			output.WriteString(content.Text)
		case "image":
			// 对于图像，只返回 MIME 类型信息
			output.WriteString(fmt.Sprintf("[Image: %s]", content.MimeType))
		case "resource":
			output.WriteString(fmt.Sprintf("[Resource: %s]", content.MimeType))
		default:
			if content.Text != "" {
				output.WriteString(content.Text)
			}
		}
		output.WriteString("\n")
	}

	// 限制结果长度
	outputStr := strings.TrimSpace(output.String())
	if len(outputStr) > 4000 {
		outputStr = outputStr[:4000] + "\n... (truncated)"
	}

	return outputStr
}

// ============================================
// Working Memory（短期记忆）相关方法
// ============================================

// appendMemoryInstructions 在系统提示词中添加工作记忆说明
func (c *AIAgentConfig) appendMemoryInstructions(systemPrompt string) string {
	memoryInstructions := `
## Working Memory

As you investigate, I will help you track key findings. After each tool observation, you will see a "Working Memory Summary" section that contains:

1. **Key Findings**: Important discoveries from tool results (metrics anomalies, error patterns, etc.)
2. **Hypotheses**: Your working theories about the root cause and their status (testing/confirmed/rejected)
3. **Evidence**: Supporting data you've collected

Use this working memory to:
- Avoid re-querying information you've already obtained
- Build upon previous findings
- Track which hypotheses have been tested
- Remember important values and patterns

When you identify important information in an observation, it will be automatically added to your working memory for future reference.
`
	return systemPrompt + memoryInstructions
}

// updateWorkingMemory 从 ReAct 步骤中提取关键信息并更新工作记忆
func (c *AIAgentConfig) updateWorkingMemory(memory *WorkingMemory, step ReActStep) {
	if memory == nil {
		return
	}

	now := time.Now().Unix()

	// 1. 从 Thought 中提取假设
	hypothesis := c.extractHypothesis(step.Thought)
	if hypothesis != nil {
		c.addHypothesis(memory, hypothesis)
	}

	// 2. 从 Observation 中提取关键发现和证据
	if step.Observation != "" && !strings.HasPrefix(step.Observation, "Error:") {
		// 提取关键发现
		findings := c.extractKeyFindings(step.Action, step.Observation)
		for _, finding := range findings {
			finding.Timestamp = now
			c.addKeyFinding(memory, &finding)
		}

		// 提取证据
		evidence := c.extractEvidence(step.Action, step.Observation)
		for _, ev := range evidence {
			c.addEvidence(memory, &ev)
		}
	}
}

// extractHypothesis 从思考中提取假设
func (c *AIAgentConfig) extractHypothesis(thought string) *Hypothesis {
	thoughtLower := strings.ToLower(thought)

	// 检测假设性语句
	hypothesisKeywords := []string{
		"i suspect", "might be", "could be", "possibly", "hypothesis",
		"i think", "it seems", "appears to be", "likely", "probably",
		"可能是", "怀疑", "猜测", "似乎", "看起来",
	}

	for _, keyword := range hypothesisKeywords {
		if strings.Contains(thoughtLower, keyword) {
			return &Hypothesis{
				Description: thought,
				Status:      "testing",
			}
		}
	}

	return nil
}

// extractKeyFindings 从观察结果中提取关键发现
func (c *AIAgentConfig) extractKeyFindings(toolName, observation string) []KeyFinding {
	var findings []KeyFinding

	// 限制观察内容长度用于分析
	obsPreview := observation
	if len(obsPreview) > 2000 {
		obsPreview = obsPreview[:2000]
	}

	obsLower := strings.ToLower(obsPreview)

	// 检测异常模式
	anomalyPatterns := map[string]string{
		"high":      "high",
		"low":       "low",
		"error":     "high",
		"exception": "high",
		"failed":    "high",
		"timeout":   "high",
		"spike":     "high",
		"100%":      "high",
		"0%":        "medium",
		"critical":  "high",
		"warning":   "medium",
		"异常":        "high",
		"错误":        "high",
		"失败":        "high",
		"超时":        "high",
	}

	for pattern, relevance := range anomalyPatterns {
		if strings.Contains(obsLower, pattern) {
			// 提取包含关键词的上下文（前后 100 个字符）
			idx := strings.Index(obsLower, pattern)
			start := idx - 100
			if start < 0 {
				start = 0
			}
			end := idx + len(pattern) + 100
			if end > len(obsPreview) {
				end = len(obsPreview)
			}

			context := strings.TrimSpace(obsPreview[start:end])
			if context != "" {
				findings = append(findings, KeyFinding{
					Content:   context,
					Source:    toolName,
					Relevance: relevance,
				})
			}
			break // 每个观察只提取一个主要发现
		}
	}

	// 如果没有检测到异常，但观察结果较短，直接保存
	if len(findings) == 0 && len(observation) < 500 && len(observation) > 10 {
		findings = append(findings, KeyFinding{
			Content:   observation,
			Source:    toolName,
			Relevance: "low",
		})
	}

	return findings
}

// extractEvidence 从观察结果中提取证据
func (c *AIAgentConfig) extractEvidence(toolName, observation string) []Evidence {
	var evidenceList []Evidence

	// 根据工具类型推断证据类型
	evidenceType := "other"
	toolNameLower := strings.ToLower(toolName)

	if strings.Contains(toolNameLower, "metric") || strings.Contains(toolNameLower, "prometheus") {
		evidenceType = "metric"
	} else if strings.Contains(toolNameLower, "log") || strings.Contains(toolNameLower, "loki") {
		evidenceType = "log"
	} else if strings.Contains(toolNameLower, "trace") || strings.Contains(toolNameLower, "jaeger") {
		evidenceType = "trace"
	} else if strings.Contains(toolNameLower, "config") || strings.Contains(toolNameLower, "cmdb") {
		evidenceType = "config"
	}

	// 限制证据长度
	content := observation
	if len(content) > 1000 {
		content = content[:1000] + "... (truncated)"
	}

	if content != "" && !strings.HasPrefix(content, "Error:") {
		evidenceList = append(evidenceList, Evidence{
			Type:    evidenceType,
			Content: content,
			Source:  toolName,
		})
	}

	return evidenceList
}

// addKeyFinding 添加关键发现到工作记忆（带去重和限制）
func (c *AIAgentConfig) addKeyFinding(memory *WorkingMemory, finding *KeyFinding) {
	// 检查重复
	for _, existing := range memory.KeyFindings {
		if existing.Content == finding.Content {
			return
		}
	}

	// 检查数量限制
	maxFindings := c.Memory.MaxFindings
	if maxFindings <= 0 {
		maxFindings = DefaultMaxFindings
	}

	if len(memory.KeyFindings) >= maxFindings {
		// 移除最旧的低相关性发现
		for i, existing := range memory.KeyFindings {
			if existing.Relevance == "low" {
				memory.KeyFindings = append(memory.KeyFindings[:i], memory.KeyFindings[i+1:]...)
				break
			}
		}
		// 如果还是满了，移除最旧的
		if len(memory.KeyFindings) >= maxFindings {
			memory.KeyFindings = memory.KeyFindings[1:]
		}
	}

	memory.KeyFindings = append(memory.KeyFindings, *finding)
}

// addHypothesis 添加假设到工作记忆
func (c *AIAgentConfig) addHypothesis(memory *WorkingMemory, hypothesis *Hypothesis) {
	maxHypotheses := c.Memory.MaxHypotheses
	if maxHypotheses <= 0 {
		maxHypotheses = DefaultMaxHypotheses
	}

	// 检查数量限制，移除已拒绝的假设
	if len(memory.TestedHypotheses) >= maxHypotheses {
		for i, existing := range memory.TestedHypotheses {
			if existing.Status == "rejected" {
				memory.TestedHypotheses = append(memory.TestedHypotheses[:i], memory.TestedHypotheses[i+1:]...)
				break
			}
		}
		if len(memory.TestedHypotheses) >= maxHypotheses {
			memory.TestedHypotheses = memory.TestedHypotheses[1:]
		}
	}

	memory.TestedHypotheses = append(memory.TestedHypotheses, *hypothesis)
}

// addEvidence 添加证据到工作记忆
func (c *AIAgentConfig) addEvidence(memory *WorkingMemory, evidence *Evidence) {
	maxEvidence := c.Memory.MaxEvidence
	if maxEvidence <= 0 {
		maxEvidence = DefaultMaxEvidence
	}

	if len(memory.Evidence) >= maxEvidence {
		memory.Evidence = memory.Evidence[1:]
	}

	memory.Evidence = append(memory.Evidence, *evidence)
}

// formatWorkingMemorySummary 格式化工作记忆摘要（用于包含在对话中）
func (c *AIAgentConfig) formatWorkingMemorySummary(memory *WorkingMemory) string {
	if memory == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Working Memory Summary\n\n")

	// 关键发现
	if len(memory.KeyFindings) > 0 {
		sb.WriteString("### Key Findings\n")
		for i, finding := range memory.KeyFindings {
			relevanceIcon := "📋"
			if finding.Relevance == "high" {
				relevanceIcon = "🔴"
			} else if finding.Relevance == "medium" {
				relevanceIcon = "🟡"
			}
			// 限制每个发现的显示长度
			content := finding.Content
			if len(content) > 200 {
				content = content[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("%d. %s [%s] %s\n", i+1, relevanceIcon, finding.Source, content))
		}
		sb.WriteString("\n")
	}

	// 假设
	if len(memory.TestedHypotheses) > 0 {
		sb.WriteString("### Hypotheses\n")
		for i, hyp := range memory.TestedHypotheses {
			statusIcon := "🔍"
			if hyp.Status == "confirmed" {
				statusIcon = "✅"
			} else if hyp.Status == "rejected" {
				statusIcon = "❌"
			}
			// 限制假设描述长度
			desc := hyp.Description
			if len(desc) > 150 {
				desc = desc[:150] + "..."
			}
			sb.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, statusIcon, desc))
		}
		sb.WriteString("\n")
	}

	// 证据数量摘要
	if len(memory.Evidence) > 0 {
		sb.WriteString(fmt.Sprintf("### Evidence Collected: %d items\n", len(memory.Evidence)))
		// 按类型统计
		typeCounts := make(map[string]int)
		for _, ev := range memory.Evidence {
			typeCounts[ev.Type]++
		}
		for evType, count := range typeCounts {
			sb.WriteString(fmt.Sprintf("- %s: %d\n", evType, count))
		}
	}

	return sb.String()
}

// ============================================
// Plan + ReAct 混合模式实现
// ============================================

// executePlanReActAgent 执行 Plan + ReAct 混合模式
// 流程：Planning → Step Execution (ReAct) → Synthesis
func (c *AIAgentConfig) executePlanReActAgent(ctx context.Context, wfCtx *models.WorkflowContext) *AgentResult {
	result := &AgentResult{
		Steps: []ReActStep{},
		Plan:  &ExecutionPlan{},
	}

	// 初始化工作记忆
	memoryEnabled := c.Memory != nil && c.Memory.Enabled
	if memoryEnabled {
		result.WorkingMemory = &WorkingMemory{
			KeyFindings:      []KeyFinding{},
			TestedHypotheses: []Hypothesis{},
			Evidence:         []Evidence{},
		}
	}

	// 构建初始用户消息
	userMessage, err := c.buildUserMessage(wfCtx)
	if err != nil {
		result.Error = fmt.Sprintf("failed to build user message: %v", err)
		return result
	}

	// ========== Phase 1: Planning ==========
	logger.Debugf("Plan+ReAct: Starting planning phase")

	plan, err := c.generatePlan(ctx, wfCtx, userMessage)
	if err != nil {
		result.Error = fmt.Sprintf("planning failed: %v", err)
		return result
	}
	result.Plan = plan

	logger.Debugf("Plan+ReAct: Generated plan with %d steps", len(plan.Steps))

	// ========== Phase 2: Step Execution (ReAct for each step) ==========
	totalIterations := 0
	allFindings := []string{}

	for i := range plan.Steps {
		select {
		case <-ctx.Done():
			result.Error = "agent execution timeout during step execution"
			result.Iterations = totalIterations
			return result
		default:
		}

		step := &plan.Steps[i]
		step.Status = "executing"
		plan.CurrentStep = i

		logger.Debugf("Plan+ReAct: Executing step %d: %s", i+1, step.Goal)

		// 为此步骤执行 ReAct 循环
		stepResult := c.executeStepWithReAct(ctx, wfCtx, step, result.WorkingMemory, allFindings)

		step.ReActSteps = stepResult.Steps
		step.Iterations = stepResult.Iterations
		totalIterations += stepResult.Iterations

		if stepResult.Success {
			step.Status = "completed"
			step.Summary = stepResult.Analysis
			step.Findings = c.extractStepFindings(stepResult)
			if step.Findings != "" {
				allFindings = append(allFindings, fmt.Sprintf("Step %d (%s): %s", i+1, step.Goal, step.Findings))
			}
		} else {
			step.Status = "failed"
			step.Error = stepResult.Error

			// 检查是否需要重新规划
			if stepResult.Error != "" && plan.ReplanCount < c.MaxReplanCount {
				logger.Debugf("Plan+ReAct: Step %d failed, considering replan", i+1)
				// todo 可以在这里实现重新规划逻辑
			}
		}

		// 将步骤的 ReAct 记录添加到总记录
		result.Steps = append(result.Steps, stepResult.Steps...)
	}

	result.Iterations = totalIterations

	// ========== Phase 3: Synthesis ==========
	logger.Debugf("Plan+ReAct: Starting synthesis phase")

	synthesis, err := c.synthesizeResults(ctx, wfCtx, plan, allFindings)
	if err != nil {
		result.Error = fmt.Sprintf("synthesis failed: %v", err)
		return result
	}

	plan.Synthesis = synthesis
	result.Analysis = synthesis
	result.Success = true

	return result
}

// generatePlan 生成分析计划
func (c *AIAgentConfig) generatePlan(ctx context.Context, wfCtx *models.WorkflowContext, userMessage string) (*ExecutionPlan, error) {
	plan := &ExecutionPlan{}

	// 构建规划提示词
	systemPrompt := c.buildPlanningPrompt()

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	response, err := c.callLLM(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed during planning: %v", err)
	}

	// 解析计划
	parsedPlan := c.parsePlanResponse(response)
	plan.TaskSummary = parsedPlan.TaskSummary
	plan.Goal = parsedPlan.Goal
	plan.FocusAreas = parsedPlan.FocusAreas
	plan.Steps = parsedPlan.Steps

	// 限制步骤数量
	if len(plan.Steps) > c.MaxPlanSteps {
		plan.Steps = plan.Steps[:c.MaxPlanSteps]
	}

	return plan, nil
}

// buildPlanningPrompt 构建规划阶段的系统提示词
func (c *AIAgentConfig) buildPlanningPrompt() string {
	var sb strings.Builder

	// 基础提示词（从嵌入文件加载）
	sb.WriteString(prompts.PlanSystemPrompt)
	sb.WriteString("\n\n")

	// 工具说明
	if len(c.Tools) > 0 {
		tools := c.convertToolsToInfo()
		sb.WriteString(llm.BuildToolsListBrief(tools))
	}

	return sb.String()
}

// parsePlanResponse 解析 LLM 返回的计划
func (c *AIAgentConfig) parsePlanResponse(response string) *ExecutionPlan {
	plan := &ExecutionPlan{}

	// 尝试从 JSON 代码块中提取
	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")

	if jsonStart >= 0 && jsonEnd > jsonStart {
		jsonStr := response[jsonStart : jsonEnd+1]

		var parsed struct {
			TaskSummary string   `json:"task_summary"`
			Goal        string   `json:"goal"`
			FocusAreas  []string `json:"focus_areas"`
			Steps       []struct {
				StepNumber int    `json:"step_number"`
				Goal       string `json:"goal"`
				Approach   string `json:"approach"`
			} `json:"steps"`
		}

		if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil {
			plan.TaskSummary = parsed.TaskSummary
			plan.Goal = parsed.Goal
			plan.FocusAreas = parsed.FocusAreas

			for _, s := range parsed.Steps {
				plan.Steps = append(plan.Steps, PlanStep{
					StepNumber: s.StepNumber,
					Goal:       s.Goal,
					Approach:   s.Approach,
					Status:     "pending",
				})
			}
		}
	}

	// 如果解析失败，创建默认计划
	if len(plan.Steps) == 0 {
		plan.Goal = "Investigate the alert"
		plan.Steps = []PlanStep{
			{
				StepNumber: 1,
				Goal:       "Gather initial metrics and logs",
				Approach:   "Query relevant monitoring data",
				Status:     "pending",
			},
			{
				StepNumber: 2,
				Goal:       "Identify anomalies",
				Approach:   "Look for unusual patterns",
				Status:     "pending",
			},
			{
				StepNumber: 3,
				Goal:       "Determine root cause",
				Approach:   "Correlate findings",
				Status:     "pending",
			},
		}
	}

	return plan
}

// executeStepWithReAct 使用 ReAct 循环执行单个计划步骤
func (c *AIAgentConfig) executeStepWithReAct(ctx context.Context, wfCtx *models.WorkflowContext, step *PlanStep, memory *WorkingMemory, previousFindings []string) *AgentResult {
	result := &AgentResult{
		Steps: []ReActStep{},
	}

	// 构建步骤特定的系统提示词
	systemPrompt := c.buildStepExecutionPrompt(step, previousFindings)

	// 构建用户消息
	userMessage := c.buildStepUserMessage(wfCtx, step)

	// 初始化对话历史
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	// ReAct 循环（使用步骤特定的迭代限制）
	maxIterations := c.MaxStepIterations
	if maxIterations <= 0 {
		maxIterations = DefaultMaxStepIterations
	}

	for iteration := 0; iteration < maxIterations; iteration++ {
		select {
		case <-ctx.Done():
			result.Error = "step execution timeout"
			result.Iterations = iteration
			return result
		default:
		}

		response, err := c.callLLM(ctx, messages)
		if err != nil {
			result.Error = fmt.Sprintf("LLM call failed: %v", err)
			result.Iterations = iteration
			return result
		}

		reactStep := c.parseReActResponse(response)
		result.Steps = append(result.Steps, reactStep)

		logger.Debugf("Plan+ReAct Step %d, iteration %d: action=%s", step.StepNumber, iteration, reactStep.Action)

		// 检查是否完成此步骤
		if reactStep.Action == ActionFinalAnswer || reactStep.Action == "Step Complete" {
			result.Analysis = reactStep.ActionInput
			result.Iterations = iteration + 1
			result.Success = true
			return result
		}

		// 执行工具
		observation := c.executeTool(ctx, reactStep.Action, reactStep.ActionInput, wfCtx)
		reactStep.Observation = observation
		result.Steps[len(result.Steps)-1] = reactStep

		// 更新工作记忆
		if memory != nil {
			c.updateWorkingMemory(memory, reactStep)
		}

		// 添加到对话历史
		messages = append(messages, ChatMessage{Role: "assistant", Content: response})
		messages = append(messages, ChatMessage{
			Role:    "user",
			Content: fmt.Sprintf("Observation: %s", observation),
		})
	}

	// 达到最大迭代次数
	result.Error = fmt.Sprintf("step reached max iterations (%d)", maxIterations)
	result.Iterations = maxIterations

	// 尝试提取部分结果
	if len(result.Steps) > 0 {
		result.Analysis = fmt.Sprintf("Step incomplete. Last thought: %s", result.Steps[len(result.Steps)-1].Thought)
	}

	return result
}

// buildStepExecutionPrompt 构建步骤执行的系统提示词
func (c *AIAgentConfig) buildStepExecutionPrompt(step *PlanStep, previousFindings []string) string {
	var sb strings.Builder

	sb.WriteString("You are executing a specific step in an investigation plan.\n\n")

	sb.WriteString("## Current Step\n")
	sb.WriteString(fmt.Sprintf("**Goal**: %s\n", step.Goal))
	sb.WriteString(fmt.Sprintf("**Approach**: %s\n\n", step.Approach))

	// 添加之前步骤的发现
	if len(previousFindings) > 0 {
		sb.WriteString("## Previous Findings\n")
		for _, finding := range previousFindings {
			sb.WriteString(fmt.Sprintf("- %s\n", finding))
		}
		sb.WriteString("\n")
	}

	// ReAct 格式说明
	sb.WriteString(`## Response Format

Respond in this format:

` + "```" + `
Thought: [Your reasoning about what to do next]
Action: [Tool name, or "Step Complete" when you have enough information for this step]
Action Input: [Tool parameters as JSON, or your findings summary for "Step Complete"]
` + "```" + `

## Available Tools

`)

	for _, tool := range c.Tools {
		sb.WriteString(fmt.Sprintf("### %s\n%s\n", tool.Name, tool.Description))
		if len(tool.Parameters) > 0 {
			sb.WriteString("Parameters:\n")
			for _, p := range tool.Parameters {
				req := ""
				if p.Required {
					req = " (required)"
				}
				sb.WriteString(fmt.Sprintf("- %s (%s)%s: %s\n", p.Name, p.Type, req, p.Description))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString(`## Important

- Focus ONLY on this step's goal
- Use "Step Complete" when you have gathered enough information for THIS step
- Summarize your findings clearly in the Action Input
`)

	return sb.String()
}

// buildStepUserMessage 构建步骤的用户消息
func (c *AIAgentConfig) buildStepUserMessage(wfCtx *models.WorkflowContext, step *PlanStep) string {
	event := wfCtx.Event
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Investigation Step %d\n\n", step.StepNumber))
	sb.WriteString(fmt.Sprintf("**Goal**: %s\n", step.Goal))
	sb.WriteString(fmt.Sprintf("**Suggested Approach**: %s\n\n", step.Approach))

	sb.WriteString("## Alert Context\n")
	sb.WriteString(fmt.Sprintf("- **Alert**: %s\n", event.RuleName))
	sb.WriteString(fmt.Sprintf("- **Severity**: %d\n", event.Severity))
	sb.WriteString(fmt.Sprintf("- **Value**: %s\n", event.TriggerValue))

	if len(event.TagsMap) > 0 {
		sb.WriteString("- **Tags**: ")
		tags := []string{}
		for k, v := range event.TagsMap {
			tags = append(tags, fmt.Sprintf("%s=%s", k, v))
		}
		sb.WriteString(strings.Join(tags, ", "))
		sb.WriteString("\n")
	}

	sb.WriteString("\nPlease investigate according to this step's goal.")

	return sb.String()
}

// extractStepFindings 从步骤结果中提取关键发现
func (c *AIAgentConfig) extractStepFindings(stepResult *AgentResult) string {
	if stepResult.Analysis != "" {
		// 限制长度
		findings := stepResult.Analysis
		if len(findings) > 500 {
			findings = findings[:500] + "..."
		}
		return findings
	}
	return ""
}

// synthesizeResults 综合所有步骤的结果
func (c *AIAgentConfig) synthesizeResults(ctx context.Context, wfCtx *models.WorkflowContext, plan *ExecutionPlan, allFindings []string) (string, error) {
	// 构建综合提示词
	systemPrompt := `You are an expert SRE. Based on the investigation findings, provide a comprehensive root cause analysis.

## Response Format

Provide your analysis in this format:

## Root Cause Analysis

### Summary
[Brief summary of the root cause]

### Evidence
[Key evidence supporting your conclusion]

### Root Cause
[Detailed explanation of the root cause]

### Impact
[What was affected and how]

### Recommendations
[Actions to resolve and prevent recurrence]
`

	// 构建用户消息
	var userMsg strings.Builder
	userMsg.WriteString(fmt.Sprintf("## Alert: %s\n\n", wfCtx.Event.RuleName))

	userMsg.WriteString("## Investigation Findings\n\n")
	for _, finding := range allFindings {
		userMsg.WriteString(fmt.Sprintf("- %s\n", finding))
	}

	userMsg.WriteString("\n## Step Summaries\n\n")
	for _, step := range plan.Steps {
		if step.Status == "completed" && step.Summary != "" {
			userMsg.WriteString(fmt.Sprintf("### Step %d: %s\n", step.StepNumber, step.Goal))
			userMsg.WriteString(fmt.Sprintf("%s\n\n", step.Summary))
		}
	}

	userMsg.WriteString("\nPlease synthesize these findings into a comprehensive root cause analysis.")

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMsg.String()},
	}

	response, err := c.callLLM(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("synthesis LLM call failed: %v", err)
	}

	return response, nil
}

// ============================================
// Skills 相关方法
// ============================================

// selectAndLoadSkills 选择并加载技能
func (c *AIAgentConfig) selectAndLoadSkills(ctx context.Context, wfCtx *models.WorkflowContext) []*SkillContent {
	if c.Skills == nil || c.skillRegistry == nil {
		return nil
	}

	var selectedSkills []*SkillMetadata

	// 优先级：手动指定 > LLM 选择 > 默认技能
	if len(c.Skills.SkillNames) > 0 {
		// 手动指定多个技能
		for _, name := range c.Skills.SkillNames {
			if skill := c.skillRegistry.GetByName(name); skill != nil {
				selectedSkills = append(selectedSkills, skill)
			} else {
				logger.Warningf("Skill '%s' not found", name)
			}
		}
	} else if c.Skills.AutoSelect && c.skillSelector != nil {
		// LLM 自动选择（可选多个）
		taskContext := c.buildTaskContext(wfCtx)
		availableSkills := c.skillRegistry.ListAll()
		maxSkills := c.Skills.MaxSkills
		if maxSkills <= 0 {
			maxSkills = DefaultMaxSkills
		}

		selected, err := c.skillSelector.SelectMultiple(ctx, taskContext, availableSkills, maxSkills)
		if err != nil {
			logger.Warningf("Skill selection failed: %v", err)
		} else {
			selectedSkills = selected
		}
	}

	// 兜底：使用默认技能
	if len(selectedSkills) == 0 && len(c.Skills.DefaultSkills) > 0 {
		for _, name := range c.Skills.DefaultSkills {
			if skill := c.skillRegistry.GetByName(name); skill != nil {
				selectedSkills = append(selectedSkills, skill)
			}
		}
	}

	// 加载所有选中技能的内容（Level 2）
	var activeSkills []*SkillContent
	for _, skill := range selectedSkills {
		content, err := c.skillRegistry.LoadContent(skill)
		if err != nil {
			logger.Warningf("Failed to load skill content for '%s': %v", skill.Name, err)
			continue
		}
		activeSkills = append(activeSkills, content)
		logger.Debugf("Loaded skill: %s", skill.Name)
	}

	return activeSkills
}

// buildTaskContext 构建任务上下文（用于技能选择）
// 支持告警事件触发和 API/Cron 等非告警场景
func (c *AIAgentConfig) buildTaskContext(wfCtx *models.WorkflowContext) string {
	var sb strings.Builder

	// 场景1: 告警事件触发
	if wfCtx.Event != nil {
		event := wfCtx.Event
		sb.WriteString("## 告警信息\n")
		sb.WriteString(fmt.Sprintf("告警名称: %s\n", event.RuleName))
		sb.WriteString(fmt.Sprintf("严重级别: %d\n", event.Severity))
		sb.WriteString(fmt.Sprintf("触发值: %s\n", event.TriggerValue))

		if event.GroupName != "" {
			sb.WriteString(fmt.Sprintf("业务组: %s\n", event.GroupName))
		}

		if len(event.TagsMap) > 0 {
			sb.WriteString("标签:\n")
			for k, v := range event.TagsMap {
				sb.WriteString(fmt.Sprintf("  - %s: %s\n", k, v))
			}
		}
		return sb.String()
	}

	// 场景2: 非告警场景（API/Cron 触发）
	sb.WriteString("## 任务上下文\n")

	// 从 Metadata 提取触发信息
	if wfCtx.Metadata != nil {
		if mode := wfCtx.Metadata["trigger_mode"]; mode != "" {
			sb.WriteString(fmt.Sprintf("触发模式: %s\n", mode))
		}
		if requestID := wfCtx.Metadata["request_id"]; requestID != "" {
			sb.WriteString(fmt.Sprintf("请求ID: %s\n", requestID))
		}
	}

	// 从 Vars 提取任务相关变量
	if len(wfCtx.Vars) > 0 {
		sb.WriteString("\n变量:\n")
		for k, v := range wfCtx.Vars {
			sb.WriteString(fmt.Sprintf("  - %s: %v\n", k, v))
		}
	}

	// 从 Inputs 提取输入参数
	if len(wfCtx.Inputs) > 0 {
		sb.WriteString("\n输入参数:\n")
		for k, v := range wfCtx.Inputs {
			// 跳过敏感信息
			if strings.Contains(strings.ToLower(k), "key") ||
				strings.Contains(strings.ToLower(k), "secret") ||
				strings.Contains(strings.ToLower(k), "password") ||
				strings.Contains(strings.ToLower(k), "token") {
				continue
			}
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", k, v))
		}
	}

	if sb.Len() == 0 {
		return "无可用上下文信息"
	}

	return sb.String()
}

// registerSkillTools 注册 Skill 工具到可用工具列表
func (c *AIAgentConfig) registerSkillTools(skills []*SkillContent) {
	// 收集所有全局工具名称
	globalTools := make(map[string]bool)
	for _, tool := range c.Tools {
		globalTools[tool.Name] = true
	}

	// 遍历所有技能
	for _, skill := range skills {
		// 1. 首先注册内置工具（builtin_tools）
		for _, builtinToolName := range skill.Metadata.BuiltinTools {
			if globalTools[builtinToolName] {
				continue
			}
			if toolDef, ok := GetBuiltinToolDef(builtinToolName); ok {
				c.Tools = append(c.Tools, toolDef)
				globalTools[builtinToolName] = true
				logger.Debugf("Registered builtin tool: %s (from skill: %s)", builtinToolName, skill.Metadata.Name)
			} else {
				logger.Warningf("Builtin tool '%s' not found (referenced in skill: %s)", builtinToolName, skill.Metadata.Name)
			}
		}

		// 2. 然后注册 skill_tools（外部工具）
		toolDescriptions, err := c.skillRegistry.LoadAllSkillToolDescriptions(skill.Metadata.Name)
		if err != nil {
			logger.Warningf("Failed to load skill tool descriptions for '%s': %v", skill.Metadata.Name, err)
			toolDescriptions = make(map[string]string)
		}

		// 如果 RecommendedTools 为空，注册 skill_tools/ 目录下的所有工具
		toolNames := skill.Metadata.RecommendedTools
		if len(toolNames) == 0 {
			// 从 toolDescriptions 获取所有工具名
			for name := range toolDescriptions {
				toolNames = append(toolNames, name)
			}
		}

		for _, toolName := range toolNames {
			// 跳过全局工具（已存在）
			if globalTools[toolName] {
				continue
			}

			// 获取真正的工具描述
			description := toolDescriptions[toolName]
			if description == "" {
				// 如果在 skill_tools/ 目录中没有找到，尝试单独加载
				if desc, err := c.skillRegistry.LoadSkillToolDescription(skill.Metadata.Name, toolName); err == nil {
					description = desc
				} else {
					// 兜底：使用占位符描述
					description = fmt.Sprintf("[Skill: %s] 专用工具", skill.Metadata.Name)
					logger.Debugf("Using fallback description for skill tool '%s'", toolName)
				}
			}

			// 注册为延迟加载的 skill tool（但使用真正的 description）
			tool := AgentTool{
				Name:        toolName,
				Description: description,
				Type:        ToolTypeSkill,
				SkillName:   skill.Metadata.Name,
			}
			c.Tools = append(c.Tools, tool)
			globalTools[toolName] = true // 避免重复添加

			logger.Debugf("Registered skill tool: %s (from skill: %s)", toolName, skill.Metadata.Name)
		}
	}
}

// initAgentResult 初始化 Agent 执行结果和工作记忆
func (c *AIAgentConfig) initAgentResult(includePlan bool) (*AgentResult, bool) {
	result := &AgentResult{
		Steps: []ReActStep{},
	}

	if includePlan {
		result.Plan = &ExecutionPlan{}
	}

	// 初始化工作记忆（如果启用）
	memoryEnabled := c.Memory != nil && c.Memory.Enabled
	if memoryEnabled {
		result.WorkingMemory = &WorkingMemory{
			KeyFindings:      []KeyFinding{},
			TestedHypotheses: []Hypothesis{},
			Evidence:         []Evidence{},
		}
	}

	return result, memoryEnabled
}

// ReActLoopConfig ReAct 循环配置
type ReActLoopConfig struct {
	MaxIterations         int
	Memory                *WorkingMemory
	MemoryEnabled         bool
	IncludeMemoryInPrompt bool
	TimeoutMessage        string
	LogPrefix             string
	// IsComplete 判断是否完成的函数
	IsComplete func(action string) bool
	// ExtractPartialResult 达到最大迭代时是否提取部分结果
	ExtractPartialResult bool
}

// runReActLoop 执行 ReAct 循环的核心逻辑
func (c *AIAgentConfig) runReActLoop(ctx context.Context, wfCtx *models.WorkflowContext, messages []ChatMessage, config *ReActLoopConfig) *AgentResult {
	result := &AgentResult{
		Steps:         []ReActStep{},
		WorkingMemory: config.Memory,
	}

	for iteration := 0; iteration < config.MaxIterations; iteration++ {
		select {
		case <-ctx.Done():
			result.Error = config.TimeoutMessage
			result.Iterations = iteration
			return result
		default:
		}

		// 调用 LLM 获取下一步思考和动作
		response, err := c.callLLM(ctx, messages)
		if err != nil {
			result.Error = fmt.Sprintf("LLM call failed at iteration %d: %v", iteration, err)
			result.Iterations = iteration
			return result
		}

		// 解析 LLM 响应，提取 Thought、Action、Action Input
		step := c.parseReActResponse(response)
		result.Steps = append(result.Steps, step)

		logger.Debugf("%s iteration %d: thought=%s, action=%s", config.LogPrefix, iteration, step.Thought, step.Action)

		// 检查是否完成
		if config.IsComplete(step.Action) {
			result.Analysis = step.ActionInput
			result.Iterations = iteration + 1
			result.Success = true
			return result
		}

		// 检查 Action 是否为空（LLM 响应格式不正确）
		if step.Action == "" {
			// 将原始响应作为 Final Answer 处理
			result.Analysis = response
			result.Iterations = iteration + 1
			result.Success = true
			return result
		}

		// 执行工具并获取观察结果
		observation := c.executeTool(ctx, step.Action, step.ActionInput, wfCtx)
		step.Observation = observation
		result.Steps[len(result.Steps)-1] = step

		// 更新工作记忆
		if config.Memory != nil {
			c.updateWorkingMemory(config.Memory, step)
		}

		// 构建观察消息（可选包含工作记忆摘要）
		observationMsg := fmt.Sprintf("Observation: %s", observation)
		if config.MemoryEnabled && config.IncludeMemoryInPrompt && config.Memory != nil && len(config.Memory.KeyFindings) > 0 {
			memorySummary := c.formatWorkingMemorySummary(config.Memory)
			observationMsg = fmt.Sprintf("%s\n\n%s", observationMsg, memorySummary)
		}

		// 将 LLM 响应和工具结果添加到对话历史
		messages = append(messages, ChatMessage{Role: "assistant", Content: response})
		messages = append(messages, ChatMessage{Role: "user", Content: observationMsg})
	}

	// 达到最大迭代次数
	result.Error = fmt.Sprintf("reached maximum iterations (%d)", config.MaxIterations)
	result.Iterations = config.MaxIterations

	// 尝试从最后的步骤中提取部分结果
	if config.ExtractPartialResult && len(result.Steps) > 0 {
		lastStep := result.Steps[len(result.Steps)-1]
		result.Analysis = fmt.Sprintf("Analysis incomplete (max iterations reached). Last thought: %s", lastStep.Thought)
	}
	return result
}

// executeReActAgentWithSkills 执行带 Skills 的 ReAct 循环
func (c *AIAgentConfig) executeReActAgentWithSkills(ctx context.Context, wfCtx *models.WorkflowContext, skills []*SkillContent) *AgentResult {
	_, memoryEnabled := c.initAgentResult(false)

	// 构建初始用户消息
	userMessage, err := c.buildUserMessage(wfCtx)
	if err != nil {
		return &AgentResult{Error: fmt.Sprintf("failed to build user message: %v", err)}
	}

	// 构建系统提示词（包含 ReAct 指导、工具说明和 Skills 知识）
	systemPrompt := c.buildSkillEnhancedReActPrompt(skills)

	// 如果启用工作记忆，在系统提示词中添加说明
	var memory *WorkingMemory
	if memoryEnabled {
		systemPrompt = c.appendMemoryInstructions(systemPrompt)
		memory = &WorkingMemory{
			KeyFindings:      []KeyFinding{},
			TestedHypotheses: []Hypothesis{},
			Evidence:         []Evidence{},
		}
	}

	// 初始化对话历史
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	return c.runReActLoop(ctx, wfCtx, messages, &ReActLoopConfig{
		MaxIterations:         c.MaxIterations,
		Memory:                memory,
		MemoryEnabled:         memoryEnabled,
		IncludeMemoryInPrompt: memoryEnabled && c.Memory != nil && c.Memory.IncludeInPrompt,
		TimeoutMessage:        "agent execution timeout",
		LogPrefix:             "AI Agent",
		IsComplete:            func(action string) bool { return action == ActionFinalAnswer },
		ExtractPartialResult:  true,
	})
}

// executePlanReActAgentWithSkills 执行带 Skills 的 Plan+ReAct 模式
func (c *AIAgentConfig) executePlanReActAgentWithSkills(ctx context.Context, wfCtx *models.WorkflowContext, skills []*SkillContent) *AgentResult {
	result, _ := c.initAgentResult(true)

	// 构建初始用户消息
	userMessage, err := c.buildUserMessage(wfCtx)
	if err != nil {
		result.Error = fmt.Sprintf("failed to build user message: %v", err)
		return result
	}

	// ========== Phase 1: Planning (带 Skills 知识) ==========
	logger.Debugf("Plan+ReAct with Skills: Starting planning phase")

	plan, err := c.generatePlanWithSkills(ctx, wfCtx, userMessage, skills)
	if err != nil {
		result.Error = fmt.Sprintf("planning failed: %v", err)
		return result
	}
	result.Plan = plan

	logger.Debugf("Plan+ReAct with Skills: Generated plan with %d steps", len(plan.Steps))

	// ========== Phase 2: Step Execution (ReAct for each step，带 Skills) ==========
	totalIterations := 0
	allFindings := []string{}

	for i := range plan.Steps {
		select {
		case <-ctx.Done():
			result.Error = "agent execution timeout during step execution"
			result.Iterations = totalIterations
			return result
		default:
		}

		step := &plan.Steps[i]
		step.Status = "executing"
		plan.CurrentStep = i

		logger.Debugf("Plan+ReAct with Skills: Executing step %d: %s", i+1, step.Goal)

		// 为此步骤执行 ReAct 循环（带 Skills 增强）
		stepResult := c.executeStepWithReActAndSkills(ctx, wfCtx, step, result.WorkingMemory, allFindings, skills)

		step.ReActSteps = stepResult.Steps
		step.Iterations = stepResult.Iterations
		totalIterations += stepResult.Iterations

		if stepResult.Success {
			step.Status = "completed"
			step.Summary = stepResult.Analysis
			step.Findings = c.extractStepFindings(stepResult)
			if step.Findings != "" {
				allFindings = append(allFindings, fmt.Sprintf("Step %d (%s): %s", i+1, step.Goal, step.Findings))
			}
		} else {
			step.Status = "failed"
			step.Error = stepResult.Error

			// 检查是否需要重新规划
			if stepResult.Error != "" && plan.ReplanCount < c.MaxReplanCount {
				logger.Debugf("Plan+ReAct with Skills: Step %d failed, considering replan", i+1)
			}
		}

		// 将步骤的 ReAct 记录添加到总记录
		result.Steps = append(result.Steps, stepResult.Steps...)
	}

	result.Iterations = totalIterations

	// ========== Phase 3: Synthesis (带 Skills 知识) ==========
	logger.Debugf("Plan+ReAct with Skills: Starting synthesis phase")

	synthesis, err := c.synthesizeResultsWithSkills(ctx, wfCtx, plan, allFindings, skills)
	if err != nil {
		result.Error = fmt.Sprintf("synthesis failed: %v", err)
		return result
	}

	plan.Synthesis = synthesis
	result.Analysis = synthesis
	result.Success = true

	return result
}

// generatePlanWithSkills 生成带 Skills 知识的调查计划
func (c *AIAgentConfig) generatePlanWithSkills(ctx context.Context, wfCtx *models.WorkflowContext, userMessage string, skills []*SkillContent) (*ExecutionPlan, error) {
	// 构建带 Skills 的规划提示词
	systemPrompt := c.buildPlanningPromptWithSkills(skills)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	response, err := c.callLLM(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("planning LLM call failed: %v", err)
	}

	// 解析计划 - 复用原有解析逻辑
	plan := c.parsePlanResponse(response)
	return plan, nil
}

// buildPlanningPromptWithSkills 构建带 Skills 的规划提示词
func (c *AIAgentConfig) buildPlanningPromptWithSkills(skills []*SkillContent) string {
	var sb strings.Builder

	// 基础提示词（从嵌入文件加载）
	sb.WriteString(prompts.PlanSystemPrompt)
	sb.WriteString("\n\n")

	// Skills 知识（Level 2 内容）
	if len(skills) > 0 {
		skillContents := make([]string, len(skills))
		for i, skill := range skills {
			if len(skills) > 1 {
				skillContents[i] = fmt.Sprintf("### %s\n\n%s", skill.Metadata.Name, skill.MainContent)
			} else {
				skillContents[i] = skill.MainContent
			}
		}
		sb.WriteString(llm.BuildSkillsSection(skillContents))

		sb.WriteString("**Important**: Use the workflow from the loaded Skills above to guide your planning.\n")
		sb.WriteString("The steps in your plan should follow the phases and methods defined in the Skills.\n\n")
	}

	// 工具说明
	if len(c.Tools) > 0 {
		tools := c.convertToolsToInfo()
		sb.WriteString(llm.BuildToolsListBrief(tools))
	}

	return sb.String()
}

// executeStepWithReActAndSkills 使用 ReAct 循环执行单个计划步骤（带 Skills）
func (c *AIAgentConfig) executeStepWithReActAndSkills(ctx context.Context, wfCtx *models.WorkflowContext, step *PlanStep, memory *WorkingMemory, previousFindings []string, skills []*SkillContent) *AgentResult {
	// 构建步骤特定的系统提示词（带 Skills 增强）
	systemPrompt := c.buildStepExecutionPromptWithSkills(step, previousFindings, skills)

	// 构建用户消息
	userMessage := c.buildStepUserMessage(wfCtx, step)

	// 初始化对话历史
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	// 确定最大迭代次数
	maxIterations := c.MaxStepIterations
	if maxIterations <= 0 {
		maxIterations = DefaultMaxStepIterations
	}

	return c.runReActLoop(ctx, wfCtx, messages, &ReActLoopConfig{
		MaxIterations:         maxIterations,
		Memory:                memory,
		MemoryEnabled:         memory != nil,
		IncludeMemoryInPrompt: false,
		TimeoutMessage:        "step execution timeout",
		LogPrefix:             fmt.Sprintf("Plan+ReAct Step %d,", step.StepNumber),
		IsComplete:            func(action string) bool { return action == ActionFinalAnswer || action == "Step Complete" },
		ExtractPartialResult:  false,
	})
}

// buildStepExecutionPromptWithSkills 构建带 Skills 的步骤执行提示词
func (c *AIAgentConfig) buildStepExecutionPromptWithSkills(step *PlanStep, previousFindings []string, skills []*SkillContent) string {
	var sb strings.Builder

	// 基础提示词（从嵌入文件加载）
	sb.WriteString(prompts.StepExecutionPrompt)
	sb.WriteString("\n\n")

	// Skills 知识
	if len(skills) > 0 {
		skillContents := make([]string, len(skills))
		for i, skill := range skills {
			skillContents[i] = skill.MainContent
		}
		sb.WriteString(llm.BuildSkillsSection(skillContents))
	}

	// 当前步骤目标
	sb.WriteString(llm.BuildCurrentStepSection(step.Goal, step.Approach))

	// 之前的发现
	sb.WriteString(llm.BuildPreviousFindingsSection(previousFindings))

	// 工具说明
	if len(c.Tools) > 0 {
		tools := c.convertToolsToInfo()
		sb.WriteString(llm.BuildToolsListBrief(tools))
	}

	sb.WriteString("\nUse 'Step Complete' as the Action when you have gathered enough information for this step.\n")

	return sb.String()
}

// synthesizeResultsWithSkills 综合结果（带 Skills 知识）
func (c *AIAgentConfig) synthesizeResultsWithSkills(ctx context.Context, wfCtx *models.WorkflowContext, plan *ExecutionPlan, allFindings []string, skills []*SkillContent) (string, error) {
	systemPrompt := c.buildSynthesisPromptWithSkills(skills)

	var userMsg strings.Builder
	userMsg.WriteString(fmt.Sprintf("## Alert: %s\n\n", wfCtx.Event.RuleName))

	userMsg.WriteString("## Investigation Findings\n\n")
	for _, finding := range allFindings {
		userMsg.WriteString(fmt.Sprintf("- %s\n", finding))
	}

	userMsg.WriteString("\n## Step Summaries\n\n")
	for _, step := range plan.Steps {
		if step.Status == "completed" && step.Summary != "" {
			userMsg.WriteString(fmt.Sprintf("### Step %d: %s\n", step.StepNumber, step.Goal))
			userMsg.WriteString(fmt.Sprintf("%s\n\n", step.Summary))
		}
	}

	userMsg.WriteString("\nPlease synthesize these findings into a comprehensive root cause analysis.")

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMsg.String()},
	}

	response, err := c.callLLM(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("synthesis LLM call failed: %v", err)
	}

	return response, nil
}

// buildSkillEnhancedReActPrompt 构建带 Skills 增强的 ReAct 系统提示词
func (c *AIAgentConfig) buildSkillEnhancedReActPrompt(skills []*SkillContent) string {
	var sb strings.Builder

	// 基础提示词（从嵌入文件加载）
	sb.WriteString(prompts.ReactSystemPrompt)
	sb.WriteString("\n\n")

	// Skills 知识（Level 2 内容）
	if len(skills) > 0 {
		skillContents := make([]string, len(skills))
		for i, skill := range skills {
			if len(skills) > 1 {
				skillContents[i] = fmt.Sprintf("### %s\n\n%s", skill.Metadata.Name, skill.MainContent)
			} else {
				skillContents[i] = skill.MainContent
			}
		}
		sb.WriteString(llm.BuildSkillsSection(skillContents))

		sb.WriteString("**Important**: Follow the workflow and guidelines defined in the loaded Skills when applicable.\n\n")
	}

	// 工具说明
	if len(c.Tools) > 0 {
		tools := c.convertToolsToInfo()
		sb.WriteString(llm.BuildToolsSection(tools))
	}

	// 环境信息
	sb.WriteString(llm.BuildEnvSection())

	return sb.String()
}

// ========== 流式输出支持 ==========

// processWithStream 流式处理 - 立即返回 StreamChan
// 支持 ReAct 和 Plan+ReAct 两种模式
func (c *AIAgentConfig) processWithStream(ctx context.Context, cancel context.CancelFunc, wfCtx *models.WorkflowContext, activeSkills []*SkillContent) (*models.WorkflowContext, string, error) {
	// 复用已有的 StreamChan（如果调用方已创建），避免 channel 替换导致的死锁
	streamChan := wfCtx.StreamChan
	if streamChan == nil {
		streamChan = make(chan *models.StreamChunk, 100)
	}

	// 获取 request_id（用于日志追踪）
	requestID := ""
	if wfCtx.Metadata != nil {
		requestID = wfCtx.Metadata["request_id"]
	}

	// 根据模式选择执行逻辑
	go func() {
		defer close(streamChan)
		// goroutine 完成后调用 cancel，释放 context 资源
		if cancel != nil {
			defer cancel()
		}
		switch c.AgentMode {
		case AgentModePlanReAct:
			c.executePlanReActWithStream(ctx, wfCtx, activeSkills, streamChan, requestID)
		default:
			// 默认使用 ReAct 模式
			c.executeReActWithStream(ctx, wfCtx, activeSkills, streamChan, requestID)
		}
	}()

	// 设置流式输出标记
	wfCtx.Stream = true
	wfCtx.StreamChan = streamChan

	// 立即返回，不阻塞
	return wfCtx, "streaming", nil
}

// executeReActWithStream 带流式输出的 ReAct 执行
func (c *AIAgentConfig) executeReActWithStream(ctx context.Context, wfCtx *models.WorkflowContext, activeSkills []*SkillContent, streamChan chan *models.StreamChunk, requestID string) {
	var fullAnalysis strings.Builder

	// 初始化工作记忆
	workingMemory := c.initWorkingMemory()

	// 构建系统提示词
	systemPrompt := c.buildSkillEnhancedReActPrompt(activeSkills)

	// 构建用户消息
	userMessage, err := c.buildUserMessage(wfCtx)
	if err != nil {
		streamChan <- &models.StreamChunk{
			Type:      models.StreamTypeError,
			Error:     "failed to build user message: " + err.Error(),
			Done:      true,
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}
		return
	}

	// 初始化消息列表
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	steps := make([]ReActStep, 0)

	for iteration := 0; iteration < c.MaxIterations; iteration++ {
		// 检查 context 是否取消
		select {
		case <-ctx.Done():
			streamChan <- &models.StreamChunk{
				Type:      models.StreamTypeError,
				Error:     "context cancelled: " + ctx.Err().Error(),
				Done:      true,
				RequestID: requestID,
				Timestamp: time.Now().UnixMilli(),
			}
			return
		default:
		}

		// 调用 LLM（流式）
		responseContent, err := c.callLLMWithStreamOutput(ctx, messages, streamChan, requestID)
		if err != nil {
			streamChan <- &models.StreamChunk{
				Type:      models.StreamTypeError,
				Error:     err.Error(),
				Done:      true,
				RequestID: requestID,
				Timestamp: time.Now().UnixMilli(),
			}
			return
		}

		// 解析 ReAct 响应
		step := c.parseReActResponse(responseContent)
		steps = append(steps, step)

		// 发送 thinking
		if step.Thought != "" {
			streamChan <- &models.StreamChunk{
				Type:      models.StreamTypeThinking,
				Content:   step.Thought,
				RequestID: requestID,
				Timestamp: time.Now().UnixMilli(),
			}
		}

		// 检查是否是 Final Answer
		if step.Action == ActionFinalAnswer {
			fullAnalysis.WriteString(step.ActionInput)

			// 更新事件 annotations 或 wfCtx.Output
			c.updateEventWithAnalysis(wfCtx, fullAnalysis.String(), steps, nil, workingMemory)

			// 发送完成
			streamChan <- &models.StreamChunk{
				Type:      models.StreamTypeDone,
				Content:   fullAnalysis.String(),
				Done:      true,
				RequestID: requestID,
				Timestamp: time.Now().UnixMilli(),
			}
			return
		}

		// 检查 Action 是否为空（LLM 响应格式不正确）
		if step.Action == "" {
			// 将原始响应作为 Final Answer 处理
			fullAnalysis.WriteString(responseContent)
			c.updateEventWithAnalysis(wfCtx, fullAnalysis.String(), steps, nil, workingMemory)
			streamChan <- &models.StreamChunk{
				Type:      models.StreamTypeDone,
				Content:   fullAnalysis.String(),
				Done:      true,
				RequestID: requestID,
				Timestamp: time.Now().UnixMilli(),
			}
			return
		}

		// 发送 tool_call
		streamChan <- &models.StreamChunk{
			Type:      models.StreamTypeToolCall,
			Content:   step.Action,
			Metadata:  map[string]interface{}{"input": step.ActionInput},
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}

		// 执行工具
		observation := c.executeTool(ctx, step.Action, step.ActionInput, wfCtx)
		step.Observation = observation

		// 发送 tool_result
		streamChan <- &models.StreamChunk{
			Type:      models.StreamTypeToolResult,
			Content:   observation,
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}

		// 更新工作记忆
		if workingMemory != nil {
			c.updateWorkingMemory(workingMemory, step)
		}

		// 更新消息历史
		messages = append(messages,
			ChatMessage{Role: "assistant", Content: responseContent},
			ChatMessage{Role: "user", Content: "Observation: " + observation},
		)

		// 如果启用了工作记忆且需要在 prompt 中包含
		if workingMemory != nil && c.Memory.IncludeInPrompt && len(workingMemory.KeyFindings) > 0 {
			memorySummary := c.formatWorkingMemorySummary(workingMemory)
			if memorySummary != "" {
				messages[len(messages)-1].Content += "\n\n" + memorySummary
			}
		}
	}

	// 达到最大迭代次数
	streamChan <- &models.StreamChunk{
		Type:      models.StreamTypeError,
		Error:     "max iterations reached without final answer",
		Done:      true,
		RequestID: requestID,
		Timestamp: time.Now().UnixMilli(),
	}
}

// ========== Plan+ReAct 流式模式 ==========

// executePlanReActWithStream 执行带流式输出的 Plan+ReAct 模式
// 流程：Planning → Step Execution (ReAct) → Synthesis
func (c *AIAgentConfig) executePlanReActWithStream(ctx context.Context, wfCtx *models.WorkflowContext, activeSkills []*SkillContent, streamChan chan *models.StreamChunk, requestID string) {
	var fullAnalysis strings.Builder

	// 初始化工作记忆
	workingMemory := c.initWorkingMemory()

	// 初始化执行计划
	plan := &ExecutionPlan{}
	allSteps := make([]ReActStep, 0)

	// 构建用户消息
	userMessage, err := c.buildUserMessage(wfCtx)
	if err != nil {
		streamChan <- &models.StreamChunk{
			Type:      models.StreamTypeError,
			Error:     "failed to build user message: " + err.Error(),
			Done:      true,
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}
		return
	}

	// ========== Phase 1: Planning ==========
	logger.Debugf("Plan+ReAct Stream: Starting planning phase")

	// 发送规划阶段开始通知
	streamChan <- &models.StreamChunk{
		Type:      StreamTypePlan,
		Content:   "开始制定调查计划...",
		Metadata:  map[string]interface{}{"phase": "planning", "status": "started"},
		RequestID: requestID,
		Timestamp: time.Now().UnixMilli(),
	}

	// 生成计划（流式输出）
	generatedPlan, err := c.generatePlanWithStream(ctx, wfCtx, userMessage, activeSkills, streamChan, requestID)
	if err != nil {
		streamChan <- &models.StreamChunk{
			Type:      models.StreamTypeError,
			Error:     "planning failed: " + err.Error(),
			Done:      true,
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}
		return
	}
	plan = generatedPlan

	// 发送计划生成完成通知
	planJSON, _ := json.Marshal(plan)
	streamChan <- &models.StreamChunk{
		Type:      StreamTypePlan,
		Content:   fmt.Sprintf("计划制定完成，共 %d 个步骤", len(plan.Steps)),
		Metadata:  map[string]interface{}{"phase": "planning", "status": "completed", "plan": json.RawMessage(planJSON)},
		RequestID: requestID,
		Timestamp: time.Now().UnixMilli(),
	}

	logger.Debugf("Plan+ReAct Stream: Generated plan with %d steps", len(plan.Steps))

	// ========== Phase 2: Step Execution (ReAct for each step) ==========
	totalIterations := 0
	allFindings := []string{}

	for i := range plan.Steps {
		// 检查 context 是否取消
		select {
		case <-ctx.Done():
			streamChan <- &models.StreamChunk{
				Type:      models.StreamTypeError,
				Error:     "context cancelled during step execution: " + ctx.Err().Error(),
				Done:      true,
				RequestID: requestID,
				Timestamp: time.Now().UnixMilli(),
			}
			return
		default:
		}

		step := &plan.Steps[i]
		step.Status = "executing"
		plan.CurrentStep = i

		// 发送步骤开始通知
		streamChan <- &models.StreamChunk{
			Type:    StreamTypeStep,
			Content: fmt.Sprintf("开始执行步骤 %d/%d: %s", i+1, len(plan.Steps), step.Goal),
			Metadata: map[string]interface{}{
				"phase":       "execution",
				"step_number": i + 1,
				"total_steps": len(plan.Steps),
				"goal":        step.Goal,
				"status":      "started",
			},
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}

		logger.Debugf("Plan+ReAct Stream: Executing step %d: %s", i+1, step.Goal)

		// 为此步骤执行 ReAct 循环（流式输出）
		stepResult := c.executeStepWithReActStream(ctx, wfCtx, step, workingMemory, allFindings, activeSkills, streamChan, requestID)

		step.ReActSteps = stepResult.Steps
		step.Iterations = stepResult.Iterations
		totalIterations += stepResult.Iterations

		if stepResult.Success {
			step.Status = "completed"
			step.Summary = stepResult.Analysis
			step.Findings = c.extractStepFindings(stepResult)
			if step.Findings != "" {
				allFindings = append(allFindings, fmt.Sprintf("Step %d (%s): %s", i+1, step.Goal, step.Findings))
			}
		} else {
			step.Status = "failed"
			step.Error = stepResult.Error

			// 检查是否需要重新规划
			if stepResult.Error != "" && plan.ReplanCount < c.MaxReplanCount {
				logger.Debugf("Plan+ReAct Stream: Step %d failed, considering replan", i+1)
				// todo 可以在这里实现重新规划逻辑
			}
		}

		// 发送步骤完成通知
		stepMetadata := map[string]interface{}{
			"phase":       "execution",
			"step_number": i + 1,
			"status":      step.Status,
		}
		if step.Status == "completed" {
			stepMetadata["summary"] = step.Summary
			stepMetadata["findings"] = step.Findings
		} else if step.Status == "failed" {
			stepMetadata["error"] = step.Error
		}
		streamChan <- &models.StreamChunk{
			Type:      StreamTypeStep,
			Content:   fmt.Sprintf("步骤 %d 完成: %s", i+1, step.Status),
			Metadata:  stepMetadata,
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}

		// 将步骤的 ReAct 记录添加到总记录
		allSteps = append(allSteps, stepResult.Steps...)
	}

	// ========== Phase 3: Synthesis ==========
	logger.Debugf("Plan+ReAct Stream: Starting synthesis phase")

	// 发送综合阶段开始通知
	streamChan <- &models.StreamChunk{
		Type:      StreamTypeSynthesis,
		Content:   "开始综合分析所有发现...",
		Metadata:  map[string]interface{}{"phase": "synthesis", "status": "started"},
		RequestID: requestID,
		Timestamp: time.Now().UnixMilli(),
	}

	// 综合分析（流式输出）
	synthesis, err := c.synthesizeResultsWithStream(ctx, wfCtx, plan, allFindings, activeSkills, streamChan, requestID)
	if err != nil {
		streamChan <- &models.StreamChunk{
			Type:      models.StreamTypeError,
			Error:     "synthesis failed: " + err.Error(),
			Done:      true,
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}
		return
	}

	plan.Synthesis = synthesis
	fullAnalysis.WriteString(synthesis)

	// 更新事件 annotations 或 wfCtx.Output
	c.updateEventWithAnalysis(wfCtx, fullAnalysis.String(), allSteps, plan, workingMemory)

	// 发送完成通知
	streamChan <- &models.StreamChunk{
		Type:      models.StreamTypeDone,
		Content:   fullAnalysis.String(),
		Done:      true,
		RequestID: requestID,
		Timestamp: time.Now().UnixMilli(),
	}
}

// generatePlanWithStream 流式生成调查计划
func (c *AIAgentConfig) generatePlanWithStream(ctx context.Context, wfCtx *models.WorkflowContext, userMessage string, skills []*SkillContent, streamChan chan *models.StreamChunk, requestID string) (*ExecutionPlan, error) {
	// 构建带 Skills 的规划提示词
	systemPrompt := c.buildPlanningPromptWithSkills(skills)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	// 调用 LLM（流式输出）
	response, err := c.callLLMWithStreamOutput(ctx, messages, streamChan, requestID)
	if err != nil {
		return nil, fmt.Errorf("planning LLM call failed: %v", err)
	}

	// 解析计划
	plan := c.parsePlanResponse(response)
	return plan, nil
}

// executeStepWithReActStream 使用流式 ReAct 循环执行单个计划步骤
func (c *AIAgentConfig) executeStepWithReActStream(ctx context.Context, wfCtx *models.WorkflowContext, step *PlanStep, memory *WorkingMemory, previousFindings []string, skills []*SkillContent, streamChan chan *models.StreamChunk, requestID string) *AgentResult {
	result := &AgentResult{
		Steps: []ReActStep{},
	}

	// 构建步骤执行的系统提示词
	systemPrompt := c.buildStepExecutionPromptWithSkills(step, previousFindings, skills)

	// 构建步骤的用户消息
	userMsg := c.buildStepUserMessage(wfCtx, step)

	// 初始化消息列表
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMsg},
	}

	// 确定最大迭代次数
	maxIterations := c.MaxStepIterations
	if maxIterations <= 0 {
		maxIterations = DefaultMaxStepIterations
	}

	// 执行 ReAct 循环
	for iteration := 0; iteration < maxIterations; iteration++ {
		// 检查 context 是否取消
		select {
		case <-ctx.Done():
			result.Error = "step execution timeout"
			return result
		default:
		}

		// 调用 LLM（流式）
		responseContent, err := c.callLLMWithStreamOutput(ctx, messages, streamChan, requestID)
		if err != nil {
			result.Error = err.Error()
			return result
		}

		// 解析 ReAct 响应
		reactStep := c.parseReActResponse(responseContent)
		result.Steps = append(result.Steps, reactStep)
		result.Iterations = iteration + 1

		logger.Debugf("Plan+ReAct Stream Step %d, iteration %d: action=%s", step.StepNumber, iteration, reactStep.Action)

		// 发送 thinking
		if reactStep.Thought != "" {
			streamChan <- &models.StreamChunk{
				Type:      models.StreamTypeThinking,
				Content:   reactStep.Thought,
				RequestID: requestID,
				Timestamp: time.Now().UnixMilli(),
			}
		}

		// 检查是否完成此步骤
		if reactStep.Action == ActionFinalAnswer || reactStep.Action == "Step Complete" {
			result.Analysis = reactStep.ActionInput
			result.Success = true
			return result
		}

		// 发送 tool_call
		streamChan <- &models.StreamChunk{
			Type:      models.StreamTypeToolCall,
			Content:   reactStep.Action,
			Metadata:  map[string]interface{}{"input": reactStep.ActionInput},
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}

		// 执行工具
		observation := c.executeTool(ctx, reactStep.Action, reactStep.ActionInput, wfCtx)
		reactStep.Observation = observation

		// 发送 tool_result
		streamChan <- &models.StreamChunk{
			Type:      models.StreamTypeToolResult,
			Content:   observation,
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}

		// 更新工作记忆
		if memory != nil {
			c.updateWorkingMemory(memory, reactStep)
		}

		// 更新消息历史
		messages = append(messages,
			ChatMessage{Role: "assistant", Content: responseContent},
			ChatMessage{Role: "user", Content: "Observation: " + observation},
		)
	}

	// 达到最大迭代次数，提取部分结果
	result.Error = "max step iterations reached"
	if len(result.Steps) > 0 {
		lastStep := result.Steps[len(result.Steps)-1]
		if lastStep.Thought != "" {
			result.Analysis = lastStep.Thought
		}
	}
	return result
}

// buildSynthesisPromptWithSkills 构建带 Skills 的综合分析提示词
func (c *AIAgentConfig) buildSynthesisPromptWithSkills(skills []*SkillContent) string {
	var sb strings.Builder

	// 基础提示词（从嵌入文件加载）
	sb.WriteString(prompts.SynthesisPrompt)
	sb.WriteString("\n\n")

	// Skills 知识
	if len(skills) > 0 {
		skillContents := make([]string, len(skills))
		for i, skill := range skills {
			skillContents[i] = skill.MainContent
		}
		sb.WriteString(llm.BuildSkillsSection(skillContents))
	}

	return sb.String()
}

// synthesizeResultsWithStream 流式综合分析结果
func (c *AIAgentConfig) synthesizeResultsWithStream(ctx context.Context, wfCtx *models.WorkflowContext, plan *ExecutionPlan, allFindings []string, skills []*SkillContent, streamChan chan *models.StreamChunk, requestID string) (string, error) {
	systemPrompt := c.buildSynthesisPromptWithSkills(skills)

	var userMsg strings.Builder
	userMsg.WriteString(fmt.Sprintf("## Alert: %s\n\n", wfCtx.Event.RuleName))

	userMsg.WriteString("## Investigation Findings\n\n")
	for _, finding := range allFindings {
		userMsg.WriteString(fmt.Sprintf("- %s\n", finding))
	}

	userMsg.WriteString("\n## Step Summaries\n\n")
	for _, step := range plan.Steps {
		if step.Status == "completed" && step.Summary != "" {
			userMsg.WriteString(fmt.Sprintf("### Step %d: %s\n", step.StepNumber, step.Goal))
			userMsg.WriteString(fmt.Sprintf("%s\n\n", step.Summary))
		}
	}

	userMsg.WriteString("\nPlease synthesize these findings into a comprehensive root cause analysis.")

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMsg.String()},
	}

	// 调用 LLM（流式输出）
	response, err := c.callLLMWithStreamOutput(ctx, messages, streamChan, requestID)
	if err != nil {
		return "", fmt.Errorf("synthesis LLM call failed: %v", err)
	}

	return response, nil
}

// callLLMWithStreamOutput 调用 LLM 并将流式输出转发到 streamChan
// 返回完整的响应内容（使用多 Provider 统一接口）
func (c *AIAgentConfig) callLLMWithStreamOutput(ctx context.Context, messages []ChatMessage, streamChan chan *models.StreamChunk, requestID string) (string, error) {
	// 确保 LLM 客户端已初始化
	if err := c.initLLMClient(); err != nil {
		return "", err
	}

	// 转换消息格式
	llmMessages := make([]llm.Message, len(messages))
	for i, msg := range messages {
		llmMessages[i] = llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// 调用 LLM 流式接口
	stream, err := c.llmClient.GenerateStream(ctx, &llm.GenerateRequest{
		Messages: llmMessages,
	})
	if err != nil {
		return "", fmt.Errorf("LLM stream error: %w", err)
	}

	// 处理流式响应
	var fullContent strings.Builder

	for chunk := range stream {
		// 检查错误
		if chunk.Error != nil {
			return fullContent.String(), fmt.Errorf("stream error: %w", chunk.Error)
		}

		// 处理内容
		if chunk.Content != "" {
			fullContent.WriteString(chunk.Content)

			// 转发文本 chunk 到 ReAct 流程的 streamChan
			streamChan <- &models.StreamChunk{
				Type:      models.StreamTypeText,
				Delta:     chunk.Content,
				Content:   fullContent.String(),
				RequestID: requestID,
				Timestamp: time.Now().UnixMilli(),
			}
		}

		// 检查是否结束
		if chunk.Done {
			break
		}
	}

	return fullContent.String(), nil
}

// updateEventWithAnalysis 更新事件的分析结果
// 如果 event 为 nil（用户输入场景），则将结果写入 wfCtx.Output
func (c *AIAgentConfig) updateEventWithAnalysis(wfCtx *models.WorkflowContext, analysis string, steps []ReActStep, plan *ExecutionPlan, memory *WorkingMemory) {
	event := wfCtx.Event

	// 如果 event 为 nil，将结果写入 wfCtx.Output
	if event == nil {
		if wfCtx.Output == nil {
			wfCtx.Output = make(map[string]interface{})
		}
		wfCtx.Output[c.OutputField] = analysis

		if len(steps) > 0 {
			wfCtx.Output[c.OutputField+"_steps"] = steps
		}

		if plan != nil {
			wfCtx.Output[c.OutputField+"_plan"] = plan
		}

		if memory != nil && len(memory.KeyFindings) > 0 {
			wfCtx.Output[c.OutputField+"_memory"] = memory
		}
		return
	}

	// event 不为 nil，写入 event.AnnotationsJSON
	if event.AnnotationsJSON == nil {
		event.AnnotationsJSON = make(map[string]string)
	}

	event.AnnotationsJSON[c.OutputField] = analysis

	if len(steps) > 0 {
		stepsJSON, _ := json.Marshal(steps)
		event.AnnotationsJSON[c.OutputField+"_steps"] = string(stepsJSON)
	}

	if plan != nil {
		planJSON, _ := json.Marshal(plan)
		event.AnnotationsJSON[c.OutputField+"_plan"] = string(planJSON)
	}

	if memory != nil && len(memory.KeyFindings) > 0 {
		memoryJSON, _ := json.Marshal(memory)
		event.AnnotationsJSON[c.OutputField+"_memory"] = string(memoryJSON)
	}

	// 更新 Annotations 字段
	b, _ := json.Marshal(event.AnnotationsJSON)
	event.Annotations = string(b)
}
