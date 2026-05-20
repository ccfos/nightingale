package airunner

import (
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
)

// event 为空时用零值结构体（不是 nil 指针）填进模板上下文，让
// {{.event.RuleName}} 这类写法渲染成空字符串而不是 panic。
func workflowContextToRequest(wfCtx *models.WorkflowContext) *aiagent.AgentRequest {
	params := make(map[string]string, len(wfCtx.Inputs)+8)
	for k, v := range wfCtx.Inputs {
		params[k] = v
	}

	var tplEvent interface{} = models.AlertCurEvent{}
	if wfCtx.Event != nil {
		tplEvent = wfCtx.Event
		flattenEvent(params, wfCtx.Event)
	}

	return &aiagent.AgentRequest{
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
}

func flattenEvent(params map[string]string, event *models.AlertCurEvent) {
	params["alert_name"] = event.RuleName
	params["severity"] = fmt.Sprintf("%d", event.Severity)
	params["trigger_value"] = event.TriggerValue
	if event.GroupName != "" {
		params["group_name"] = event.GroupName
	}
	if len(event.TagsMap) > 0 {
		tags := make([]string, 0, len(event.TagsMap))
		for k, v := range event.TagsMap {
			tags = append(tags, fmt.Sprintf("%s=%s", k, v))
		}
		params["tags"] = strings.Join(tags, ",")
	}
	for k, v := range event.AnnotationsJSON {
		params["annotation_"+k] = v
	}
}
