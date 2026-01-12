package logic

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	alertCommon "github.com/ccfos/nightingale/v6/alert/common"
	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/common"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
)

// 判断模式常量
const (
	ConditionModeExpression = "expression" // 表达式模式（默认）
	ConditionModeTags       = "tags"       // 标签/属性模式
)

// IfConfig If 条件处理器配置
type IfConfig struct {
	// 判断模式：expression（表达式）或 tags（标签/属性）
	Mode string `json:"mode,omitempty"`

	// 表达式模式配置
	// 条件表达式（支持 Go 模板语法）
	// 例如：{{ if eq .Severity 1 }}true{{ end }}
	Condition string `json:"condition,omitempty"`

	// 标签/属性模式配置
	LabelKeys  []models.TagFilter `json:"label_keys,omitempty"` // 适用标签
	Attributes []models.TagFilter `json:"attributes,omitempty"` // 适用属性

	// 内部使用，解析后的过滤器
	parsedLabelKeys  []models.TagFilter `json:"-"`
	parsedAttributes []models.TagFilter `json:"-"`
}

func init() {
	models.RegisterProcessor("logic.if", &IfConfig{})
}

func (c *IfConfig) Init(settings interface{}) (models.Processor, error) {
	result, err := common.InitProcessor[*IfConfig](settings)
	if err != nil {
		return nil, err
	}

	// 解析标签过滤器
	if len(result.LabelKeys) > 0 {
		// Deep copy to avoid concurrent map writes on cached objects
		labelKeysCopy := make([]models.TagFilter, len(result.LabelKeys))
		copy(labelKeysCopy, result.LabelKeys)
		for i := range labelKeysCopy {
			if labelKeysCopy[i].Func == "" {
				labelKeysCopy[i].Func = labelKeysCopy[i].Op
			}
		}
		result.parsedLabelKeys, err = models.ParseTagFilter(labelKeysCopy)
		if err != nil {
			return nil, fmt.Errorf("failed to parse label_keys: %v", err)
		}
	}

	// 解析属性过滤器
	if len(result.Attributes) > 0 {
		// Deep copy to avoid concurrent map writes on cached objects
		attributesCopy := make([]models.TagFilter, len(result.Attributes))
		copy(attributesCopy, result.Attributes)
		for i := range attributesCopy {
			if attributesCopy[i].Func == "" {
				attributesCopy[i].Func = attributesCopy[i].Op
			}
		}
		result.parsedAttributes, err = models.ParseTagFilter(attributesCopy)
		if err != nil {
			return nil, fmt.Errorf("failed to parse attributes: %v", err)
		}
	}

	return result, nil
}

// Process 实现 Processor 接口（兼容旧模式）
func (c *IfConfig) Process(ctx *ctx.Context, wfCtx *models.WorkflowContext) (*models.WorkflowContext, string, error) {
	result, err := c.evaluateCondition(wfCtx)
	if err != nil {
		return wfCtx, "", fmt.Errorf("if processor: failed to evaluate condition: %v", err)
	}

	if result {
		return wfCtx, "condition matched (true branch)", nil
	}
	return wfCtx, "condition not matched (false branch)", nil
}

// ProcessWithBranch 实现 BranchProcessor 接口
func (c *IfConfig) ProcessWithBranch(ctx *ctx.Context, wfCtx *models.WorkflowContext) (*models.NodeOutput, error) {
	result, err := c.evaluateCondition(wfCtx)
	if err != nil {
		return nil, fmt.Errorf("if processor: failed to evaluate condition: %v", err)
	}

	output := &models.NodeOutput{
		WfCtx: wfCtx,
	}

	if result {
		// 条件为 true，走输出 0（true 分支）
		branchIndex := 0
		output.BranchIndex = &branchIndex
		output.Message = "condition matched (true branch)"
	} else {
		// 条件为 false，走输出 1（false 分支）
		branchIndex := 1
		output.BranchIndex = &branchIndex
		output.Message = "condition not matched (false branch)"
	}

	return output, nil
}

// evaluateCondition 评估条件
func (c *IfConfig) evaluateCondition(wfCtx *models.WorkflowContext) (bool, error) {
	mode := c.Mode
	if mode == "" {
		mode = ConditionModeExpression // 默认表达式模式
	}

	switch mode {
	case ConditionModeTags:
		return c.evaluateTagsCondition(wfCtx.Event)
	default:
		return c.evaluateExpressionCondition(wfCtx)
	}
}

// evaluateExpressionCondition 评估表达式条件
func (c *IfConfig) evaluateExpressionCondition(wfCtx *models.WorkflowContext) (bool, error) {
	if c.Condition == "" {
		return true, nil
	}

	// 构建模板数据
	var defs = []string{
		"{{ $event := .Event }}",
		"{{ $labels := .Event.TagsMap }}",
		"{{ $value := .Event.TriggerValue }}",
		"{{ $env := .Env }}",
	}

	text := strings.Join(append(defs, c.Condition), "")

	tpl, err := template.New("if_condition").Funcs(tplx.TemplateFuncMap).Parse(text)
	if err != nil {
		return false, err
	}

	var buf bytes.Buffer
	if err = tpl.Execute(&buf, wfCtx); err != nil {
		return false, err
	}

	result := strings.TrimSpace(strings.ToLower(buf.String()))
	return result == "true" || result == "1", nil
}

// evaluateTagsCondition 评估标签/属性条件
func (c *IfConfig) evaluateTagsCondition(event *models.AlertCurEvent) (bool, error) {
	// 如果没有配置任何过滤条件，默认返回 true
	if len(c.parsedLabelKeys) == 0 && len(c.parsedAttributes) == 0 {
		return true, nil
	}

	// 匹配标签 (TagsMap)
	if len(c.parsedLabelKeys) > 0 {
		tagsMap := event.TagsMap
		if tagsMap == nil {
			tagsMap = make(map[string]string)
		}
		if !alertCommon.MatchTags(tagsMap, c.parsedLabelKeys) {
			return false, nil
		}
	}

	// 匹配属性 (JsonTagsAndValue - 所有 JSON 字段)
	if len(c.parsedAttributes) > 0 {
		attributesMap := event.JsonTagsAndValue()
		if !alertCommon.MatchTags(attributesMap, c.parsedAttributes) {
			return false, nil
		}
	}

	return true, nil
}
