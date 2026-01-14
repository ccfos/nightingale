package eventdrop

import (
	"bytes"
	"fmt"
	"strings"
	texttemplate "text/template"

	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/common"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
	"github.com/toolkits/pkg/logger"
)

type EventDropConfig struct {
	Content string `json:"content"`
}

func init() {
	models.RegisterProcessor("event_drop", &EventDropConfig{})
}

func (c *EventDropConfig) Init(settings interface{}) (models.Processor, error) {
	result, err := common.InitProcessor[*EventDropConfig](settings)
	return result, err
}

func (c *EventDropConfig) Process(ctx *ctx.Context, wfCtx *models.WorkflowContext) (*models.WorkflowContext, string, error) {
	// 使用背景是可以根据此处理器，实现对事件进行更加灵活的过滤的逻辑
	// 在标签过滤和属性过滤都不满足需求时可以使用
	// 如果模板执行结果为 true，则删除该事件
	event := wfCtx.Event

	var defs = []string{
		"{{ $event := .Event }}",
		"{{ $labels := .Event.TagsMap }}",
		"{{ $value := .Event.TriggerValue }}",
		"{{ $env := .Env }}",
	}

	text := strings.Join(append(defs, c.Content), "")

	tpl, err := texttemplate.New("eventdrop").Funcs(tplx.TemplateFuncMap).Parse(text)
	if err != nil {
		return wfCtx, "", fmt.Errorf("processor failed to parse template: %v processor: %v", err, c)
	}

	var body bytes.Buffer
	if err = tpl.Execute(&body, wfCtx); err != nil {
		return wfCtx, "", fmt.Errorf("processor failed to execute template: %v processor: %v", err, c)
	}

	result := strings.TrimSpace(body.String())
	logger.Infof("processor eventdrop result: %v", result)
	if result == "true" {
		wfCtx.Event = nil
		logger.Infof("processor eventdrop drop event: %v", event)
		return wfCtx, "drop event success", nil
	}

	return wfCtx, "drop event failed", nil
}
