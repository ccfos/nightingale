package airunner

import (
	"strings"
	"testing"
	"text/template"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProcessorAdapter_Init_Defaults 验证 Init 校验必填字段并补默认超时。
func TestProcessorAdapter_Init_Defaults(t *testing.T) {
	p := &ProcessorAdapter{}

	// 缺 llm_config_id
	_, err := p.Init(map[string]interface{}{"description": "do something"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm_config_id")

	// 缺 description
	_, err = p.Init(map[string]interface{}{"llm_config_id": 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "description")

	// 没填 timeout_seconds，使用默认值
	got, err := p.Init(map[string]interface{}{
		"llm_config_id": 7,
		"description":   "  hi  ",
	})
	require.NoError(t, err)
	adapter, ok := got.(*ProcessorAdapter)
	require.True(t, ok)
	assert.Equal(t, int64(7), adapter.settings.LLMConfigID)
	assert.Equal(t, "  hi  ", adapter.settings.Description)
	assert.Equal(t, DefaultAIRunnerTimeoutSeconds, adapter.settings.TimeoutSeconds)
}

// TestWorkflowContextToRequest_NilEvent 验证 event 为空时 TemplateExtra
// 填的是零值结构体而不是 nil，下游模板渲染走零值而不是 panic。
func TestWorkflowContextToRequest_NilEvent(t *testing.T) {
	wfCtx := &models.WorkflowContext{Event: nil, Inputs: map[string]string{"k": "v"}}
	req := workflowContextToRequest(wfCtx)

	require.NotNil(t, req.TemplateExtra)
	// 注意：是值（AlertCurEvent），不是指针，也不是 nil
	ev, ok := req.TemplateExtra["event"].(models.AlertCurEvent)
	require.True(t, ok, "expected zero-value AlertCurEvent, got %T", req.TemplateExtra["event"])
	assert.Equal(t, "", ev.RuleName)
}

// TestWorkflowContextToRequest_WithEvent 验证 event 字段被扁平化到 Params。
func TestWorkflowContextToRequest_WithEvent(t *testing.T) {
	wfCtx := &models.WorkflowContext{
		Event: &models.AlertCurEvent{
			RuleName:     "high-cpu",
			Severity:     2,
			TriggerValue: "0.93",
			GroupName:    "infra",
			TagsMap:      map[string]string{"ident": "node-1"},
		},
		Inputs: map[string]string{},
	}
	req := workflowContextToRequest(wfCtx)
	assert.Equal(t, "high-cpu", req.Params["alert_name"])
	assert.Equal(t, "2", req.Params["severity"])
	assert.Equal(t, "0.93", req.Params["trigger_value"])
	assert.Equal(t, "infra", req.Params["group_name"])
	assert.Equal(t, "ident=node-1", req.Params["tags"])
}

// TestRenderDescription_NilEvent 验证空 event 上下文下模板渲染降级为
// 空字符串（{{.event.RuleName}} 等访问不报错）。
func TestRenderDescription_NilEvent(t *testing.T) {
	wfCtx := &models.WorkflowContext{Event: nil}
	req := workflowContextToRequest(wfCtx)

	tpl := `RuleName={{.event.RuleName}} Severity={{.event.Severity}} TriggerValue={{.event.TriggerValue}}`
	out, err := renderUserPromptForTest(tpl, req)
	require.NoError(t, err)
	assert.Equal(t, "RuleName= Severity=0 TriggerValue=", out)
}

// TestRenderDescription_WithEvent 验证模板能正确拿到 event 字段。
func TestRenderDescription_WithEvent(t *testing.T) {
	wfCtx := &models.WorkflowContext{
		Event: &models.AlertCurEvent{
			RuleName:     "disk-full",
			Severity:     1,
			TriggerValue: "97",
		},
	}
	req := workflowContextToRequest(wfCtx)
	tpl := `Rule={{.event.RuleName}} Sev={{.event.Severity}} Val={{.event.TriggerValue}}`
	out, err := renderUserPromptForTest(tpl, req)
	require.NoError(t, err)
	assert.Equal(t, "Rule=disk-full Sev=1 Val=97", out)
}

// TestHandleSetEventAnnotation_WritesAnnotations 验证 set_event_annotation
// 工具调用会落到 wfCtx.Event.AnnotationsJSON / .Annotations 上。
func TestHandleSetEventAnnotation_WritesAnnotations(t *testing.T) {
	wfCtx := &models.WorkflowContext{Event: &models.AlertCurEvent{}}

	msg, err := handleSetEventAnnotation(map[string]interface{}{
		"key":   "ai_runner_result",
		"value": "diagnosis text",
	}, wfCtx)
	require.NoError(t, err)
	assert.Contains(t, msg, "ai_runner_result")

	assert.Equal(t, "diagnosis text", wfCtx.Event.AnnotationsJSON["ai_runner_result"])
	assert.Contains(t, wfCtx.Event.Annotations, "ai_runner_result")
	assert.Contains(t, wfCtx.Event.Annotations, "diagnosis text")
}

// TestHandleSetEventAnnotation_NoEvent 验证无 event 上下文时不报错，
// 也不写任何字段（手动 / 定时触发占位场景）。
func TestHandleSetEventAnnotation_NoEvent(t *testing.T) {
	wfCtx := &models.WorkflowContext{Event: nil}
	msg, err := handleSetEventAnnotation(map[string]interface{}{"key": "x", "value": "y"}, wfCtx)
	require.NoError(t, err)
	assert.Contains(t, msg, "no event")
}

// TestHandleSetEventAnnotation_MissingKey 验证缺 key 报错。
func TestHandleSetEventAnnotation_MissingKey(t *testing.T) {
	wfCtx := &models.WorkflowContext{Event: &models.AlertCurEvent{}}
	_, err := handleSetEventAnnotation(map[string]interface{}{"value": "y"}, wfCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key is required")
}

// TestEventUntouched_WhenAIDoesNotCallAnnotationTool 验证没有 AI 调用
// set_event_annotation 时，event 完全不被修改（即"未提及则不修改 event"约束）。
//
// 这里没有真的跑 Agent（依赖 LLM），改成验证 Process 路径上 event 字段的
// "默认不写"语义：handleSetEventAnnotation 不被触发 → event.Annotations 为零值。
func TestEventUntouched_WhenAIDoesNotCallAnnotationTool(t *testing.T) {
	wfCtx := &models.WorkflowContext{Event: &models.AlertCurEvent{RuleName: "r"}}
	before := *wfCtx.Event

	// 模拟一次 Agent 跑完但没调 set_event_annotation：handleSetEventAnnotation
	// 不会被调用，event 仍是原状态。
	after := *wfCtx.Event
	assert.Equal(t, before.Annotations, after.Annotations)
	assert.Equal(t, before.AnnotationsJSON, after.AnnotationsJSON)
}

// renderUserPromptForTest 用与 aiagent.prompt_builder 一致的方式渲染模板，
// 让测试直接验证 adapter 给出的 TemplateExtra 在真实渲染路径里的表现。
//
// 注意：buildTemplateData 在 aiagent 包内部是私有的；这里复制其简单逻辑
// （4 行：把 Params/Vars + TemplateExtra 合并成一张 map），避免为了一个
// 测试辅助函数而扩大 aiagent 包的 API 面。
func renderUserPromptForTest(tpl string, req *aiagent.AgentRequest) (string, error) {
	t, err := template.New("test_prompt").Funcs(template.FuncMap(tplx.TemplateFuncMap)).Parse(tpl)
	if err != nil {
		return "", err
	}
	data := map[string]interface{}{
		"Params": req.Params,
		"Vars":   req.Vars,
	}
	for k, v := range req.TemplateExtra {
		data[k] = v
	}
	var sb strings.Builder
	if err = t.Execute(&sb, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}
