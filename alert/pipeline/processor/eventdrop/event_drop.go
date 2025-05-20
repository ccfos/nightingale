package eventdrop

import (
	"bytes"
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
	models.RegisterProcessor("eventdrop", &EventDropConfig{})
}

func (c *EventDropConfig) Init(settings interface{}) (models.Processor, error) {
	result, err := common.InitProcessor[*EventDropConfig](settings)
	return result, err
}

func (c *EventDropConfig) Process(ctx *ctx.Context, event *models.AlertCurEvent) {
	// 使用背景是可以根据此处理器，实现对事件进行更加灵活的过滤的逻辑
	// 在标签过滤和属性过滤都不满足需求时可以使用
	// 如果模板执行结果为 true，则删除该事件

	var defs = []string{
		"{{ $event := . }}",
		"{{ $labels := .TagsMap }}",
		"{{ $value := .TriggerValue }}",
	}

	text := strings.Join(append(defs, c.Content), "")

	tpl, err := texttemplate.New("eventdrop").Funcs(tplx.TemplateFuncMap).Parse(text)
	if err != nil {
		logger.Errorf("failed to parse template: %v event: %v", err, event)
		return
	}

	var body bytes.Buffer
	if err = tpl.Execute(&body, event); err != nil {
		logger.Errorf("failed to execute template: %v event: %v", err, event)
		return
	}

	result := strings.TrimSpace(body.String())
	if result == "true" {
		event = nil
	}
}
