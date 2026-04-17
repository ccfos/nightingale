package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// processorSettings 用于从 Processor 的 JSON settings 中拆分出 LLM 配置和 Agent 配置
// JSON 形如：{"llm": {...llm.Config 字段...}, ...AgentConfig 字段...}
type processorSettings struct {
	LLM         llm.Config `json:"llm"`
	AgentConfig            // 嵌入：Agent 行为字段继续在 JSON 顶层
}

// ==================== Processor 适配器 ====================
// 将通用 Agent 适配为 models.Processor 接口，用于事件处理器场景

func init() {
	models.RegisterProcessor("ai.agent", &ProcessorAdapter{})
}

// ProcessorAdapter 将 Agent 适配为 models.Processor
type ProcessorAdapter struct {
	agent *Agent
}

func (p *ProcessorAdapter) Init(settings interface{}) (models.Processor, error) {
	b, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	var s processorSettings
	if err = json.Unmarshal(b, &s); err != nil {
		return nil, err
	}

	return &ProcessorAdapter{
		agent: NewAgent(&s.AgentConfig, WithLLMConfig(&s.LLM)),
	}, nil
}

func (p *ProcessorAdapter) Process(ctxObj *ctx.Context, wfCtx *models.WorkflowContext) (*models.WorkflowContext, string, error) {
	// 1. 转换 WorkflowContext → AgentRequest
	req := workflowContextToRequest(wfCtx)

	// 2. 注入外部工具处理器（用于 processor/skill 类型工具）
	p.agent.SetExternalToolHandler(func(bgCtx context.Context, tool *AgentTool, args map[string]interface{}, agentReq *AgentRequest) (string, error) {
		switch tool.Type {
		case ToolTypeProcessor:
			return executeProcessorTool(ctxObj, tool, args, wfCtx)
		case ToolTypeSkill:
			return executeSkillTool(ctxObj, tool, args, wfCtx, p.agent.skillRegistry)
		}
		return "", fmt.Errorf("unsupported external tool type: %s", tool.Type)
	})

	// 3. 流式模式：转换 channel 类型
	// adapter 路径下流式完全由调用方（workflow engine）控制，不看 cfg.Stream
	if wfCtx.Stream || wfCtx.StreamChan != nil {
		agentStreamChan := make(chan *StreamChunk, 100)
		req.StreamChan = agentStreamChan

		// 复用或创建 models.StreamChunk channel
		modelsStreamChan := wfCtx.StreamChan
		if modelsStreamChan == nil {
			modelsStreamChan = make(chan *models.StreamChunk, 100)
		}

		// 启动转换 goroutine：aiagent.StreamChunk → models.StreamChunk
		go convertStreamChunks(agentStreamChan, modelsStreamChan)

		wfCtx.Stream = true
		wfCtx.StreamChan = modelsStreamChan
	}

	// 4. 执行 Agent
	resp, err := p.agent.Run(context.Background(), req)
	if err != nil {
		return wfCtx, "", err
	}

	// 流式模式：立即返回
	if req.StreamChan != nil {
		return wfCtx, "streaming", nil
	}

	// 5. 非流式模式：将结果写回 WorkflowContext
	writeResponseToWorkflowContext(wfCtx, resp, p.agent.cfg.OutputField)

	msg := fmt.Sprintf("AI Agent (%s mode) completed: %d iterations, success=%v",
		p.agent.cfg.AgentMode, resp.Iterations, resp.Success)
	if resp.Error != "" {
		return wfCtx, msg, fmt.Errorf("agent error: %s", resp.Error)
	}

	return wfCtx, msg, nil
}

// ==================== 转换函数 ====================

// workflowContextToRequest 将 WorkflowContext 转换为 AgentRequest
func workflowContextToRequest(wfCtx *models.WorkflowContext) *AgentRequest {
	// 深拷贝 Inputs，避免后续写入污染原始 map
	params := make(map[string]string, len(wfCtx.Inputs)+8)
	for k, v := range wfCtx.Inputs {
		params[k] = v
	}

	req := &AgentRequest{
		Params:    params,
		Vars:      wfCtx.Vars,
		Metadata:  wfCtx.Metadata,
		ParentCtx: wfCtx.ParentCtx,
		// 注入兼容模板数据，保证旧模板（引用 .Event / .Inputs / .event / .inputs）继续工作
		TemplateExtra: map[string]interface{}{
			"Event":  wfCtx.Event,
			"Inputs": wfCtx.Inputs,
			// 小写别名，兼容 HTTP body template 中 {{.event}} / {{.inputs}} 写法
			"event":  wfCtx.Event,
			"inputs": wfCtx.Inputs,
		},
	}

	// 将 Event 关键信息扁平化写入 Params（供 builtin tools 和 prompt 使用）
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

// writeResponseToWorkflowContext 将 AgentResponse 写回 WorkflowContext
func writeResponseToWorkflowContext(wfCtx *models.WorkflowContext, resp *AgentResponse, outputField string) {
	if resp == nil {
		return
	}

	event := wfCtx.Event

	// 如果 event 为 nil，写入 wfCtx.Output
	if event == nil {
		if wfCtx.Output == nil {
			wfCtx.Output = make(map[string]interface{})
		}
		wfCtx.Output[outputField] = resp.Content
		if len(resp.Steps) > 0 {
			wfCtx.Output[outputField+"_steps"] = resp.Steps
		}
		if resp.Plan != nil {
			wfCtx.Output[outputField+"_plan"] = resp.Plan
		}
		return
	}

	// event 不为 nil，写入 event.AnnotationsJSON
	if event.AnnotationsJSON == nil {
		event.AnnotationsJSON = make(map[string]string)
	}

	event.AnnotationsJSON[outputField] = resp.Content

	if len(resp.Steps) > 0 {
		stepsJSON, _ := json.Marshal(resp.Steps)
		event.AnnotationsJSON[outputField+"_steps"] = string(stepsJSON)
	}
	if resp.Plan != nil {
		planJSON, _ := json.Marshal(resp.Plan)
		event.AnnotationsJSON[outputField+"_plan"] = string(planJSON)
	}

	b, _ := json.Marshal(event.AnnotationsJSON)
	event.Annotations = string(b)
}

// convertStreamChunks 将 aiagent.StreamChunk 转换为 models.StreamChunk
func convertStreamChunks(agentChan <-chan *StreamChunk, modelsChan chan<- *models.StreamChunk) {
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
func executeProcessorTool(ctxObj *ctx.Context, tool *AgentTool, args map[string]interface{}, wfCtx *models.WorkflowContext) (string, error) {
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

	// 复制事件和上下文
	var eventCopy models.AlertCurEvent
	if wfCtx.Event != nil {
		eventCopy = *wfCtx.Event
	}
	wfCtxCopy := &models.WorkflowContext{
		Event:    &eventCopy,
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
func executeSkillTool(ctxObj *ctx.Context, tool *AgentTool, args map[string]interface{}, wfCtx *models.WorkflowContext, skillRegistry *SkillRegistry) (string, error) {
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

	var eventCopy models.AlertCurEvent
	if wfCtx.Event != nil {
		eventCopy = *wfCtx.Event
	}
	wfCtxCopy := &models.WorkflowContext{
		Event:    &eventCopy,
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

