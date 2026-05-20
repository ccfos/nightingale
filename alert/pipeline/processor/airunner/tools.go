package airunner

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// AI Runner 中唯一能改写 event 的工具。其余 skill tool 都在 deepcopy 上跑，
// 副作用不持久化——这是"未明确要求则不动 event"产品契约的代码兜底。
func setEventAnnotationToolDef() aiagent.AgentTool {
	return aiagent.AgentTool{
		Name:        setEventAnnotationTool,
		Type:        aiagent.ToolTypeProcessor, // 借此类型让 Agent dispatcher 把调用转发到 ExternalToolHandler；handler 按 Name 派发。
		Description: "把分析结论写入告警事件的 annotations 字段。仅当任务描述明确要求把结果写入 annotations 的某个 key 时调用；未要求则不要调用，保持事件原样。",
		Parameters: []aiagent.ToolParameter{
			{Name: "key", Type: "string", Description: "annotations 的字段名，例如 ai_runner_result", Required: true},
			{Name: "value", Type: "string", Description: "要写入的字符串内容", Required: true},
		},
	}
}

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

// AI auto-select 选中的 skill 暴露给 LLM 的工具，统一通过这里桥接到夜莺
// processor 注册表。在 event 的 deepcopy 上执行，确保 skill tool 没法绕过
// set_event_annotation 偷偷改 event。
func executeSkillTool(ctxObj *ctx.Context, tool *aiagent.AgentTool, args map[string]interface{}, wfCtx *models.WorkflowContext, registry *aiagent.SkillRegistry) (string, error) {
	if registry == nil {
		return "", errors.New("skill registry not initialized")
	}
	if tool.SkillName == "" {
		return "", errors.New("skill_name not specified for skill tool")
	}
	skillTool, err := registry.LoadSkillTool(tool.SkillName, tool.Name)
	if err != nil {
		return "", fmt.Errorf("load skill tool %q: %w", tool.Name, err)
	}

	config := make(map[string]interface{}, len(skillTool.Config)+len(args))
	for k, v := range skillTool.Config {
		config[k] = v
	}
	for k, v := range args {
		config[k] = v
	}

	processor, err := models.GetProcessorByType(skillTool.Type, config)
	if err != nil {
		return "", fmt.Errorf("get processor %q: %w", skillTool.Type, err)
	}

	wfCtxCopy := &models.WorkflowContext{
		Inputs:   wfCtx.Inputs,
		Vars:     wfCtx.Vars,
		Metadata: wfCtx.Metadata,
	}
	if wfCtx.Event != nil {
		wfCtxCopy.Event = wfCtx.Event.DeepCopy()
	}

	_, msg, err := processor.Process(ctxObj, wfCtxCopy)
	if err != nil {
		return "", fmt.Errorf("skill tool %q failed: %w", tool.Name, err)
	}
	if msg == "" {
		msg = fmt.Sprintf("skill tool %q executed", tool.Name)
	}
	return msg, nil
}
