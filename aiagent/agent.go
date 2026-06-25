package aiagent

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/ccfos/nightingale/v6/aiagent/mcp"
	"github.com/toolkits/pkg/logger"
)

// 包级 LLM Client 缓存（供 adapter 等无法从外部注入缓存的路径使用）
var defaultClientCache = llm.NewClientCache()

// AgentOption 用于在创建 Agent 时注入可选依赖
type AgentOption func(*Agent)

// WithLLMClient 注入已有的 LLM 客户端（跳过内部创建，复用连接池）
func WithLLMClient(c llm.LLM) AgentOption {
	return func(a *Agent) { a.llmClient = c }
}

// WithLLMConfig 根据 llm.Config 从包级缓存获取或创建 LLM 客户端
func WithLLMConfig(cfg *llm.Config) AgentOption {
	return func(a *Agent) {
		client, err := defaultClientCache.GetOrCreate(cfg)
		if err != nil {
			logger.Errorf("Failed to create LLM client from config: %v %v", cfg, err)
			return
		}
		a.llmClient = client
	}
}

// WithToolDeps 注入内置工具的运行期依赖（DBCtx、数据源获取器等）
func WithToolDeps(d *ToolDeps) AgentOption {
	return func(a *Agent) { a.toolDeps = d }
}

// NewAgent 创建 Agent 实例
func NewAgent(cfg *AgentConfig, opts ...AgentOption) *Agent {
	a := &Agent{cfg: cfg}
	for _, opt := range opts {
		opt(a)
	}
	a.applyDefaults()
	return a
}

// SetExternalToolHandler 设置外部工具处理器（用于 processor/skill 类型工具）
func (a *Agent) SetExternalToolHandler(h ExternalToolHandler) {
	a.externalToolHandler = h
}

// SetSkillRegistry 设置技能注册表
func (a *Agent) SetSkillRegistry(registry *SkillRegistry) {
	a.skillRegistry = registry
}

// InitSkills 初始化技能
func (a *Agent) InitSkills(skillsPath string) {
	if a.cfg.Skills == nil || skillsPath == "" {
		return
	}
	// Normalize to an absolute path so downstream consumers (skill registry +
	// the list_files / read_file builtin tools' resolveBasePath security check)
	// don't have to deal with "." vs "skill" vs "/abs/path" ambiguity.
	if abs, err := filepath.Abs(skillsPath); err == nil {
		skillsPath = abs
	}
	// skillsPath 被 list_files / read_file / grep_files 这几个 builtin tool
	// 读（用作 resolveBasePath 的安全基准），经由 a.toolDeps 透传。
	if a.toolDeps == nil {
		a.toolDeps = &ToolDeps{}
	}
	a.toolDeps.SkillsPath = skillsPath
	// 内置 skill 的磁盘落地由进程启动期的一次性 ExtractBuiltin 负责（见
	// center/router/router.go 中 runAISkillSyncLoop 之前的调用），这里不再
	// 每条消息都 destructive re-extract，避免多 chat 并发时 read_file /
	// SkillRegistry 在"删目录—重写"间隙读到空目录的竞态。
	a.skillRegistry = NewSkillRegistry(skillsPath)
	logger.Infof("AI Agent Skills initialized: path=%s, pinned=%d", skillsPath, len(a.cfg.Skills.SkillNames))
}

// Run 执行 Agent（唯一公开入口）
//
// 非流式模式（req.StreamChan == nil）：阻塞直到完成，返回完整结果
// 流式模式（req.StreamChan != nil）：启动 goroutine 后立即返回 nil，
// 结果通过 StreamChan 发送，goroutine 完成后关闭 channel
func (a *Agent) Run(ctx context.Context, req *AgentRequest) (*AgentResponse, error) {
	// 创建带超时的 context
	parentCtx := req.ParentCtx
	if parentCtx == nil {
		parentCtx = ctx
	}
	timeoutCtx, cancel := context.WithTimeout(parentCtx, time.Duration(a.cfg.Timeout)*time.Millisecond)

	// 预载显式指定的 Skills（目录 + load_skill 自取路径不在此预载）
	tSkillStart := time.Now()
	activeSkills := a.loadPinnedSkills()
	if len(activeSkills) > 0 {
		logger.Debugf("AI Agent loaded %d skills", len(activeSkills))
	}
	tSkillElapsed := time.Since(tSkillStart)

	// 构造本次 Run 的工具表：cfg.Tools 作只读种子 → 追加 skill 工具 → 追加 MCP 工具
	tToolStart := time.Now()
	tools := append([]AgentTool(nil), a.cfg.Tools...)
	tools = a.appendSkillTools(tools, activeSkills)
	mcpToolCount := 0
	if a.mcpClientManager != nil && len(a.mcpServers) > 0 {
		mcpCtx, mcpCancel := context.WithTimeout(context.Background(), 10*time.Second)
		before := len(tools)
		tools = a.appendMCPTools(mcpCtx, tools)
		mcpCancel()
		mcpToolCount = len(tools) - before
	}
	// 按需技能加载：技能子系统开启时挂 load_skill 工具，配合系统提示词里常驻的
	// 「可用技能目录」（appendSkillCatalog），agent 可在运行中自取所需技能；
	// 加载结果经结构化 transcript 自动跨轮持久。
	if a.cfg.Skills != nil && a.skillRegistry != nil {
		if def, ok := GetBuiltinToolDef("load_skill"); ok {
			tools = appendToolIfAbsent(tools, def)
		}
		// 技能脚本执行：只有当 sandbox 控制器存在且在本宿主可执行时才挂
		// run_skill_script，避免在无法执行的环境里给模型一个必然失败的工具。
		if a.toolDeps != nil && a.toolDeps.Sandbox != nil && a.toolDeps.Sandbox.Enabled() {
			if def, ok := GetBuiltinToolDef("run_skill_script"); ok {
				tools = appendToolIfAbsent(tools, def)
			}
		}
		// 跨轮回放（工具渐进披露，见 skill_tools_inject.go）：history 里已加载
		// 技能的声明工具重新注入——上一轮 load_skill 进来的工具本轮继续可用。
		if len(req.History) > 0 {
			tools = a.appendToolsFromLoadedSkills(tools, req.History)
		}
	}

	logger.Infof("[Agent] preparation: skills=%dms (n=%d) tools=%dms (mcp_added=%d total=%d)",
		tSkillElapsed.Milliseconds(), len(activeSkills),
		time.Since(tToolStart).Milliseconds(), mcpToolCount, len(tools))

	rc := &runCtx{skills: activeSkills, tools: tools}

	// 流式模式：启动 goroutine，立即返回
	// cfg.Stream 仅作为"默认创建 channel"的开关；如果调用方已传 StreamChan 则直接用
	if req.StreamChan != nil {
		return a.runWithStream(timeoutCtx, cancel, req, rc)
	}
	if a.cfg.Stream {
		// 直接调用方（如 router）设了 cfg.Stream 但没传 StreamChan，自动创建
		req.StreamChan = make(chan *StreamChunk, 100)
		return a.runWithStream(timeoutCtx, cancel, req, rc)
	}

	// 非流式模式
	defer cancel()

	return a.executeNative(timeoutCtx, req, rc), nil
}

// runWithStream 流式执行 - 启动 goroutine 后立即返回
// 前置条件：req.StreamChan 不为 nil（由 Run 保证）
func (a *Agent) runWithStream(ctx context.Context, cancel context.CancelFunc, req *AgentRequest, rc *runCtx) (*AgentResponse, error) {
	streamChan := req.StreamChan

	go func() {
		defer close(streamChan)
		if cancel != nil {
			defer cancel()
		}

		logger.Infof("[Agent] Stream goroutine started")
		a.executeNativeWithDone(ctx, req, rc)
		logger.Infof("[Agent] Stream goroutine finished")
	}()

	return nil, nil
}

// ==================== 内部辅助方法 ====================

// appendToolIfAbsent 按 Name 去重追加工具（action 的 SelectTools 可能已含同名工具）。
func appendToolIfAbsent(tools []AgentTool, def AgentTool) []AgentTool {
	for i := range tools {
		if tools[i].Name == def.Name {
			return tools
		}
	}
	return append(tools, def)
}

func truncStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// applyDefaults 设置默认值
func (a *Agent) applyDefaults() {
	cfg := a.cfg
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = DefaultMaxIterations
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "ai_analysis"
	}

	// MCP 初始化
	if cfg.MCP != nil && len(cfg.MCP.Servers) > 0 {
		a.mcpClientManager = mcp.NewClientManager()
		a.mcpServers = make(map[string]*mcp.ServerConfig)
		for i := range cfg.MCP.Servers {
			server := &cfg.MCP.Servers[i]
			a.mcpServers[server.Name] = server
		}
		logger.Infof("AI Agent MCP initialized: %d servers configured", len(cfg.MCP.Servers))
	}
}

// loadPinnedSkills 加载显式预载的技能（SkillNames：action RequiredSkills / agent
// 显式绑定）。SkillNames 为空时不预载——目录常驻 + load_skill 自取路径不经过这里，
// 由模型在循环里按需加载（无 LLM 预选环节，见 SkillConfig 注释）。
func (a *Agent) loadPinnedSkills() []*SkillContent {
	if a.cfg.Skills == nil || a.skillRegistry == nil || len(a.cfg.Skills.SkillNames) == 0 {
		return nil
	}

	var activeSkills []*SkillContent
	for _, name := range a.cfg.Skills.SkillNames {
		skill := a.skillRegistry.GetByName(name)
		if skill == nil {
			logger.Warningf("Skill '%s' not found", name)
			continue
		}
		content, err := a.skillRegistry.LoadContent(skill)
		if err != nil {
			logger.Warningf("Failed to load skill content for '%s': %v", name, err)
			continue
		}
		activeSkills = append(activeSkills, content)
		logger.Debugf("Loaded skill: %s", name)
	}

	return activeSkills
}

// appendSkillTools 基于 base 工具表追加 skill 关联的 builtin / skill_tool
// 纯函数：不写 a.cfg，返回新切片（供 runCtx 使用）
func (a *Agent) appendSkillTools(base []AgentTool, skills []*SkillContent) []AgentTool {
	if len(skills) == 0 {
		return base
	}

	seen := make(map[string]bool, len(base))
	for _, t := range base {
		seen[t.Name] = true
	}
	result := base

	for _, skill := range skills {
		// 追加内置工具
		for _, builtinToolName := range skill.Metadata.BuiltinTools {
			if seen[builtinToolName] {
				continue
			}
			if toolDef, ok := GetBuiltinToolDef(builtinToolName); ok {
				result = append(result, toolDef)
				seen[builtinToolName] = true
				logger.Debugf("Registered builtin tool: %s (from skill: %s)", builtinToolName, skill.Metadata.Name)
			}
		}

		// 追加 skill_tools
		toolDescriptions, err := a.skillRegistry.LoadAllSkillToolDescriptions(skill.Metadata.Name)
		if err != nil {
			logger.Warningf("Failed to load skill tool descriptions for '%s': %v", skill.Metadata.Name, err)
			toolDescriptions = make(map[string]string)
		}

		toolNames := skill.Metadata.RecommendedTools
		if len(toolNames) == 0 {
			for name := range toolDescriptions {
				toolNames = append(toolNames, name)
			}
		}

		for _, toolName := range toolNames {
			if seen[toolName] {
				continue
			}

			description := toolDescriptions[toolName]
			if description == "" {
				if desc, err := a.skillRegistry.LoadSkillToolDescription(skill.Metadata.Name, toolName); err == nil {
					description = desc
				} else {
					description = fmt.Sprintf("[Skill: %s] 专用工具", skill.Metadata.Name)
				}
			}

			result = append(result, AgentTool{
				Name:        toolName,
				Description: description,
				Type:        ToolTypeSkill,
				SkillName:   skill.Metadata.Name,
			})
			seen[toolName] = true

			logger.Debugf("Registered skill tool: %s (from skill: %s)", toolName, skill.Metadata.Name)
		}
	}

	return result
}
