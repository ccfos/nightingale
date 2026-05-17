package airunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/llm"
	// Blank import 触发 builtin tools 的 init 注册。center 已通过
	// router_ai_assistant.go 引入了同一个包，但 alert/edge 进程在没有 chat
	// 路由的情况下只走 ai_runner 处理器路径，必须在这里 import 一次，
	// 否则 skill 声明的 builtin_tools 在边缘节点完全不可用。
	_ "github.com/ccfos/nightingale/v6/aiagent/tools"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// ==================== AI Runner Processor 适配器 ====================
//
// AI Runner 是事件处理器中的一种特殊类型：让运维用一段自然语言描述任务，
// 处理器把任务描述渲染成 prompt 交给 Agent 自主调用 skill / 内置 tool。
//
// 与旧的 ai.agent 处理器（未上线）的差异：
//   - 不再让用户自填 LLM 字段，只引用 AILLMConfig 的 ID
//   - 仅暴露三个配置：llm_config_id、description、timeout_seconds
//   - 写回 event annotations 完全由 AI 通过 set_event_annotation 工具决定
//   - event 为空时（手动 / 定时触发场景占位）模板渲染降级为空字符串
//
// 设计取舍：处理器 Init 阶段没有 *ctx.Context，无法读 AILLMConfig；
// 因此延迟到 Process 阶段才解析 LLM 配置和构造 Agent，处理器实例本身只
// 缓存原始配置三元组。
//
// 为什么放在 alert/pipeline/processor 而不是 aiagent 包：
//   - alert/edge 进程在事件流水线里执行 ai_runner 时需要 processor 被注册；
//     放在 alert/pipeline/processor 下，由 alert/pipeline/pipeline.go 的
//     `_ import` 统一触发，alert / center 两端注册路径对称
//   - 运行期依赖（DBCtx、PromClients 等）由宿主进程在启动时通过 Setup 注入

const (
	// ProcessorTypeAIRunner 事件处理器类型标识。
	ProcessorTypeAIRunner = "ai_runner"

	// DefaultAIRunnerTimeoutSeconds 任务描述未指定时使用的默认超时（秒）。
	DefaultAIRunnerTimeoutSeconds = 180

	// AIRunnerOutputField 通用 AI 输出字段名（当且仅当 wfCtx.Event 为 nil
	// 时写入 wfCtx.Output；Event 存在时按规约不主动写 annotations）。
	AIRunnerOutputField = "ai_runner_result"

	// setEventAnnotationTool 是 AI Runner 单独注入的、可写 event annotations
	// 的特殊工具名。借用 ToolTypeProcessor 走 ExternalToolHandler，由本文件
	// 内的闭包完成实际写入。
	setEventAnnotationTool = "set_event_annotation"
)

// aiRunnerSettings 是处理器从 EventPipeline JSON 反序列化出的配置三元组。
type aiRunnerSettings struct {
	LLMConfigID    int64  `json:"llm_config_id"`
	Description    string `json:"description"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func init() {
	models.RegisterProcessor(ProcessorTypeAIRunner, &ProcessorAdapter{})
}

// ProcessorAdapter 实现 AI Runner 事件处理器。
type ProcessorAdapter struct {
	settings aiRunnerSettings
}

// Init 解析 JSON 配置并校验必填项。
func (p *ProcessorAdapter) Init(settings interface{}) (models.Processor, error) {
	b, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	var s aiRunnerSettings
	if err = json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.LLMConfigID <= 0 {
		return nil, errors.New("ai_runner: llm_config_id is required")
	}
	if strings.TrimSpace(s.Description) == "" {
		return nil, errors.New("ai_runner: description is required")
	}
	if s.TimeoutSeconds <= 0 {
		s.TimeoutSeconds = DefaultAIRunnerTimeoutSeconds
	}
	return &ProcessorAdapter{settings: s}, nil
}

// Process 执行 AI Runner。
//
// 错误返回严格遵循：Process 返回 error 时引擎会把当前节点标记为 failed 但
// 不会影响兄弟节点。超时由 Agent 内部 ctx 控制，最终也以 error 形式落地。
func (p *ProcessorAdapter) Process(ctxObj *ctx.Context, wfCtx *models.WorkflowContext) (*models.WorkflowContext, string, error) {
	// 0. 未注入运行期 → 当前进程未显式启用 ai_runner，直接报错。
	// 避免边缘/alert 节点在未开启 AIAgent 的情况下仍读取 LLM 配置并发起
	// 外部调用造成数据外发。center 进程通过 alert.Start / router 启动期
	// 调用 SetRuntime；alert/edge 通过 alertc.AIAgent.Enable 控制是否调用。
	deps, skillsPath, enabled := GetRuntime()
	if !enabled {
		return wfCtx, "", errors.New("ai_runner: runtime not initialized; AIAgent disabled on this process")
	}

	// 1. 解析 LLM 配置
	llmCfg, err := models.AILLMConfigGetById(ctxObj, p.settings.LLMConfigID)
	if err != nil {
		return wfCtx, "", fmt.Errorf("ai_runner: load LLM config id=%d failed: %v", p.settings.LLMConfigID, err)
	}
	if llmCfg == nil {
		return wfCtx, "", fmt.Errorf("ai_runner: LLM config id=%d not found", p.settings.LLMConfigID)
	}
	if !llmCfg.Enabled {
		return wfCtx, "", fmt.Errorf("ai_runner: LLM config id=%d is disabled", p.settings.LLMConfigID)
	}

	timeoutMs := p.settings.TimeoutSeconds * 1000

	// 2. 转 llm.Config（沿用 chat 路径同款映射）
	extra := llmCfg.ExtraConfig
	llmConfig := &llm.Config{
		Provider:      llmCfg.APIType,
		BaseURL:       llmCfg.APIURL,
		APIKey:        llmCfg.APIKey,
		Model:         llmCfg.Model,
		Headers:       extra.CustomHeaders,
		Timeout:       timeoutMs,
		SkipSSLVerify: extra.SkipTLSVerify,
		Proxy:         extra.Proxy,
		Temperature:   extra.Temperature,
		MaxTokens:     extra.MaxTokens,
		ExtraBody:     extra.CustomParams,
	}

	// 3. 构造 AgentConfig：任务描述作为 UserPromptTemplate；启用 AutoSelect
	// 让 LLM 自己挑 skill；OutputField 仅用于无 event 场景的兜底输出。
	agentCfg := &aiagent.AgentConfig{
		AgentMode:          aiagent.AgentModeReAct,
		Timeout:            timeoutMs,
		UserPromptTemplate: p.settings.Description,
		OutputField:        AIRunnerOutputField,
		Skills:             &aiagent.SkillConfig{AutoSelect: true, MaxSkills: 2},
		// 单独注入 set_event_annotation 工具：AI 视任务描述自行决定是否调用。
		Tools: []aiagent.AgentTool{newSetEventAnnotationToolDef()},
	}

	// 4. 注入运行期依赖（toolDeps、skillsPath），由宿主进程启动期写入。
	// 这里必须拷贝一份 ToolDeps 再传入：agent.InitSkills 会写入
	// a.toolDeps.SkillsPath，多个 ai_runner 并发执行会同时改同一个共享
	// 指针，触发数据竞争且文件类 builtin tool 可能读到中间态。
	opts := []aiagent.AgentOption{aiagent.WithLLMConfig(llmConfig)}
	if deps != nil {
		depsCopy := *deps
		opts = append(opts, aiagent.WithToolDeps(&depsCopy))
	}
	agent := aiagent.NewAgent(agentCfg, opts...)
	if skillsPath != "" {
		agent.InitSkills(skillsPath)
	}

	// 5. 构造 Request；event 为空时填充零值结构体让模板渲染降级为空字符串
	req := workflowContextToRequest(wfCtx)

	// 6. ExternalToolHandler：先识别 set_event_annotation，再 fallthrough
	// 到既有的 processor / skill 工具执行逻辑。
	agent.SetExternalToolHandler(func(bgCtx context.Context, tool *aiagent.AgentTool, args map[string]interface{}, agentReq *aiagent.AgentRequest) (string, error) {
		if tool.Name == setEventAnnotationTool {
			return handleSetEventAnnotation(args, wfCtx)
		}
		switch tool.Type {
		case aiagent.ToolTypeProcessor:
			return executeProcessorTool(ctxObj, tool, args, wfCtx)
		case aiagent.ToolTypeSkill:
			return executeSkillTool(ctxObj, tool, args, wfCtx, agent.SkillRegistry())
		}
		return "", fmt.Errorf("ai_runner: unsupported external tool type: %s", tool.Type)
	})

	// 7. 流式模式：转换 channel（沿用旧 adapter 的写法）
	if wfCtx.Stream || wfCtx.StreamChan != nil {
		agentStreamChan := make(chan *aiagent.StreamChunk, 100)
		req.StreamChan = agentStreamChan

		modelsStreamChan := wfCtx.StreamChan
		if modelsStreamChan == nil {
			modelsStreamChan = make(chan *models.StreamChunk, 100)
		}
		go convertStreamChunks(agentStreamChan, modelsStreamChan)

		wfCtx.Stream = true
		wfCtx.StreamChan = modelsStreamChan
	}

	// 8. 执行 Agent（Agent.Run 内部已 WithTimeout）
	resp, err := agent.Run(context.Background(), req)
	if err != nil {
		return wfCtx, "", fmt.Errorf("ai_runner: agent run failed: %v", err)
	}

	// 流式：channel 已交给上层
	if req.StreamChan != nil {
		return wfCtx, "streaming", nil
	}

	// 9. Agent 内部错误（含超时）→ 节点失败
	if resp != nil && resp.Error != "" {
		return wfCtx, "", fmt.Errorf("ai_runner: %s", resp.Error)
	}

	// 10. event 为空时把最终回答写到 wfCtx.Output 兜底；event 存在时
	// 不主动写 annotations——是否写入完全由 AI 通过 set_event_annotation 决定。
	if wfCtx.Event == nil && resp != nil {
		if wfCtx.Output == nil {
			wfCtx.Output = make(map[string]interface{})
		}
		wfCtx.Output[AIRunnerOutputField] = resp.Content
	}

	msg := ""
	if resp != nil {
		msg = fmt.Sprintf("AI Runner completed: %d iterations, success=%v", resp.Iterations, resp.Success)
	}
	return wfCtx, msg, nil
}

// newSetEventAnnotationToolDef 构造 set_event_annotation 工具定义。
// 选用 ToolTypeProcessor 是为了走 ExternalToolHandler 分支；实际由 adapter
// 自己处理而不是真的去 GetProcessorByType。
func newSetEventAnnotationToolDef() aiagent.AgentTool {
	return aiagent.AgentTool{
		Name:        setEventAnnotationTool,
		Type:        aiagent.ToolTypeProcessor,
		Description: "把分析结论写入告警事件的 annotations 字段。仅当任务描述明确要求把结果写入 annotations 的某个 key 时调用；未要求则不要调用，保持事件原样。",
		Parameters: []aiagent.ToolParameter{
			{Name: "key", Type: "string", Description: "annotations 的字段名，例如 ai_runner_result", Required: true},
			{Name: "value", Type: "string", Description: "要写入的字符串内容", Required: true},
		},
	}
}

// handleSetEventAnnotation 把 AI 调用 set_event_annotation 的结果落到 event 上。
// event 为空（手动/定时触发占位）时返回友好提示，不报错。
func handleSetEventAnnotation(args map[string]interface{}, wfCtx *models.WorkflowContext) (string, error) {
	key, _ := args["key"].(string)
	value, _ := args["value"].(string)
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("set_event_annotation: key is required")
	}
	if wfCtx == nil || wfCtx.Event == nil {
		return "no event in context, annotation not written", nil
	}
	event := wfCtx.Event
	if event.AnnotationsJSON == nil {
		event.AnnotationsJSON = make(map[string]string)
	}
	event.AnnotationsJSON[key] = value
	b, _ := json.Marshal(event.AnnotationsJSON)
	event.Annotations = string(b)
	return fmt.Sprintf("annotation %q written", key), nil
}

// ==================== 转换函数 ====================

// workflowContextToRequest 将 WorkflowContext 转换为 AgentRequest。
//
// 当 wfCtx.Event 为空时，TemplateExtra 里的 Event/event 改成零值结构体而不是
// nil，让 {{ .event.RuleName }} 这类模板写法走零值（空字符串）路径，不报错
// 也不中断。
func workflowContextToRequest(wfCtx *models.WorkflowContext) *aiagent.AgentRequest {
	params := make(map[string]string, len(wfCtx.Inputs)+8)
	for k, v := range wfCtx.Inputs {
		params[k] = v
	}

	// 选择模板上下文使用的 event：空时退化为零值结构体（注意：是值类型，
	// 不是指针；Go 模板对 nil 指针字段访问会失败，零值结构体可以正常返回
	// 零值字段）。
	var tplEvent interface{}
	if wfCtx.Event != nil {
		tplEvent = wfCtx.Event
	} else {
		tplEvent = models.AlertCurEvent{}
	}

	req := &aiagent.AgentRequest{
		Params:    params,
		Vars:      wfCtx.Vars,
		Metadata:  wfCtx.Metadata,
		ParentCtx: wfCtx.ParentCtx,
		TemplateExtra: map[string]interface{}{
			"Event":  tplEvent,
			"Inputs": wfCtx.Inputs,
			"event":  tplEvent,
			"inputs": wfCtx.Inputs,
		},
	}

	if wfCtx.Event != nil {
		event := wfCtx.Event
		req.Params["alert_name"] = event.RuleName
		req.Params["severity"] = fmt.Sprintf("%d", event.Severity)
		req.Params["trigger_value"] = event.TriggerValue
		if event.GroupName != "" {
			req.Params["group_name"] = event.GroupName
		}
		if len(event.TagsMap) > 0 {
			tags := make([]string, 0, len(event.TagsMap))
			for k, v := range event.TagsMap {
				tags = append(tags, fmt.Sprintf("%s=%s", k, v))
			}
			req.Params["tags"] = strings.Join(tags, ",")
		}
		if len(event.AnnotationsJSON) > 0 {
			for k, v := range event.AnnotationsJSON {
				req.Params["annotation_"+k] = v
			}
		}
	}

	return req
}

// convertStreamChunks 将 aiagent.StreamChunk 转成 models.StreamChunk。
func convertStreamChunks(agentChan <-chan *aiagent.StreamChunk, modelsChan chan<- *models.StreamChunk) {
	defer close(modelsChan)

	for chunk := range agentChan {
		mc := &models.StreamChunk{
			Type:      chunk.Type,
			Content:   chunk.Content,
			Delta:     chunk.Delta,
			Metadata:  chunk.Metadata,
			RequestID: chunk.RequestID,
			Timestamp: chunk.Timestamp,
			Done:      chunk.Done,
			Error:     chunk.Error,
		}
		modelsChan <- mc
	}
}

// ==================== 外部工具执行（processor/skill） ====================

// executeProcessorTool 执行夜莺内部处理器工具
func executeProcessorTool(ctxObj *ctx.Context, tool *aiagent.AgentTool, args map[string]interface{}, wfCtx *models.WorkflowContext) (string, error) {
	if tool.ProcessorType == "" {
		return "", fmt.Errorf("processor_type not configured")
	}

	config := make(map[string]interface{})
	for k, v := range tool.ProcessorConfig {
		config[k] = v
	}
	for k, v := range args {
		config[k] = v
	}

	processor, err := models.GetProcessorByType(tool.ProcessorType, config)
	if err != nil {
		return "", fmt.Errorf("failed to get processor '%s': %v", tool.ProcessorType, err)
	}

	// 深拷贝 event：浅拷贝会让 TagsMap / AnnotationsJSON / 切片等仍与原事件
	// 共享，processor 在副本上的 map 改动会泄漏到真实事件，破坏“只有
	// set_event_annotation 才写回 event”的约束。
	var eventForTool *models.AlertCurEvent
	if wfCtx.Event != nil {
		eventForTool = wfCtx.Event.DeepCopy()
	}
	wfCtxCopy := &models.WorkflowContext{
		Event:    eventForTool,
		Inputs:   wfCtx.Inputs,
		Vars:     wfCtx.Vars,
		Metadata: wfCtx.Metadata,
	}

	resultWfCtx, msg, err := processor.Process(ctxObj, wfCtxCopy)
	if err != nil {
		return "", fmt.Errorf("processor execution failed: %v", err)
	}

	var result strings.Builder
	if msg != "" {
		result.WriteString(fmt.Sprintf("Message: %s\n", msg))
	}
	if resultWfCtx != nil && resultWfCtx.Event != nil && wfCtx.Event != nil {
		if resultWfCtx.Event.Annotations != wfCtx.Event.Annotations {
			result.WriteString(fmt.Sprintf("Updated annotations: %s\n", resultWfCtx.Event.Annotations))
		}
		if resultWfCtx.Event.Tags != wfCtx.Event.Tags {
			result.WriteString(fmt.Sprintf("Updated tags: %s\n", resultWfCtx.Event.Tags))
		}
	}
	if result.Len() == 0 {
		return "Processor executed successfully (no visible changes)", nil
	}
	return result.String(), nil
}

// executeSkillTool 执行 Skill 专用工具
func executeSkillTool(ctxObj *ctx.Context, tool *aiagent.AgentTool, args map[string]interface{}, wfCtx *models.WorkflowContext, skillRegistry *aiagent.SkillRegistry) (string, error) {
	if skillRegistry == nil {
		return "", fmt.Errorf("skill registry not initialized")
	}
	if tool.SkillName == "" {
		return "", fmt.Errorf("skill_name not specified for skill tool")
	}

	skillTool, err := skillRegistry.LoadSkillTool(tool.SkillName, tool.Name)
	if err != nil {
		return "", fmt.Errorf("failed to load skill tool '%s': %v", tool.Name, err)
	}

	config := make(map[string]interface{})
	for k, v := range skillTool.Config {
		config[k] = v
	}
	for k, v := range args {
		config[k] = v
	}

	processor, err := models.GetProcessorByType(skillTool.Type, config)
	if err != nil {
		return "", fmt.Errorf("failed to get processor '%s': %v", skillTool.Type, err)
	}

	// 深拷贝 event：理由同 executeProcessorTool。
	var eventForTool *models.AlertCurEvent
	if wfCtx.Event != nil {
		eventForTool = wfCtx.Event.DeepCopy()
	}
	wfCtxCopy := &models.WorkflowContext{
		Event:    eventForTool,
		Inputs:   wfCtx.Inputs,
		Vars:     wfCtx.Vars,
		Metadata: wfCtx.Metadata,
	}

	resultWfCtx, msg, err := processor.Process(ctxObj, wfCtxCopy)
	if err != nil {
		return "", fmt.Errorf("skill tool execution failed: %v", err)
	}

	var result strings.Builder
	if msg != "" {
		result.WriteString(fmt.Sprintf("Message: %s\n", msg))
	}
	if resultWfCtx != nil && resultWfCtx.Event != nil && wfCtx.Event != nil {
		if resultWfCtx.Event.Annotations != wfCtx.Event.Annotations {
			result.WriteString(fmt.Sprintf("Updated annotations: %s\n", resultWfCtx.Event.Annotations))
		}
		if resultWfCtx.Event.Tags != wfCtx.Event.Tags {
			result.WriteString(fmt.Sprintf("Updated tags: %s\n", resultWfCtx.Event.Tags))
		}
	}
	if result.Len() == 0 {
		return "Skill tool executed successfully (no visible changes)", nil
	}
	return result.String(), nil
}
