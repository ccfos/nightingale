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

// SwitchCase Switch 分支定义
type SwitchCase struct {
	// 判断模式：expression（表达式）或 tags（标签/属性）
	Mode string `json:"mode,omitempty"`

	// 表达式模式配置
	// 条件表达式（支持 Go 模板语法）
	Condition string `json:"condition,omitempty"`

	// 标签/属性模式配置
	LabelKeys  []models.TagFilter `json:"label_keys,omitempty"` // 适用标签
	Attributes []models.TagFilter `json:"attributes,omitempty"` // 适用属性

	// 分支名称（可选，用于日志）
	Name string `json:"name,omitempty"`

	// 内部使用，解析后的过滤器
	parsedLabelKeys  []models.TagFilter `json:"-"`
	parsedAttributes []models.TagFilter `json:"-"`
}

// SwitchConfig Switch 多分支处理器配置
type SwitchConfig struct {
	// 分支条件列表
	// 按顺序匹配，第一个为 true 的分支将被选中
	Cases []SwitchCase `json:"cases"`
	// 是否允许多个分支同时匹配（默认 false，只走第一个匹配的）
	AllowMultiple bool `json:"allow_multiple,omitempty"`
}

func init() {
	models.RegisterProcessor("logic.switch", &SwitchConfig{})
}

func (c *SwitchConfig) Init(settings interface{}) (models.Processor, error) {
	result, err := common.InitProcessor[*SwitchConfig](settings)
	if err != nil {
		return nil, err
	}

	// 解析每个 case 的标签和属性过滤器
	for i := range result.Cases {
		if len(result.Cases[i].LabelKeys) > 0 {
			// Deep copy to avoid concurrent map writes on cached objects
			labelKeysCopy := make([]models.TagFilter, len(result.Cases[i].LabelKeys))
			copy(labelKeysCopy, result.Cases[i].LabelKeys)
			for j := range labelKeysCopy {
				if labelKeysCopy[j].Func == "" {
					labelKeysCopy[j].Func = labelKeysCopy[j].Op
				}
			}
			result.Cases[i].parsedLabelKeys, err = models.ParseTagFilter(labelKeysCopy)
			if err != nil {
				return nil, fmt.Errorf("failed to parse label_keys for case[%d]: %v", i, err)
			}
		}

		if len(result.Cases[i].Attributes) > 0 {
			// Deep copy to avoid concurrent map writes on cached objects
			attributesCopy := make([]models.TagFilter, len(result.Cases[i].Attributes))
			copy(attributesCopy, result.Cases[i].Attributes)
			for j := range attributesCopy {
				if attributesCopy[j].Func == "" {
					attributesCopy[j].Func = attributesCopy[j].Op
				}
			}
			result.Cases[i].parsedAttributes, err = models.ParseTagFilter(attributesCopy)
			if err != nil {
				return nil, fmt.Errorf("failed to parse attributes for case[%d]: %v", i, err)
			}
		}
	}

	return result, nil
}

// Process 实现 Processor 接口（兼容旧模式）
func (c *SwitchConfig) Process(ctx *ctx.Context, wfCtx *models.WorkflowContext) (*models.WorkflowContext, string, error) {
	index, caseName, err := c.evaluateCases(wfCtx)
	if err != nil {
		return wfCtx, "", fmt.Errorf("switch processor: failed to evaluate cases: %v", err)
	}

	if index >= 0 {
		if caseName != "" {
			return wfCtx, fmt.Sprintf("matched case[%d]: %s", index, caseName), nil
		}
		return wfCtx, fmt.Sprintf("matched case[%d]", index), nil
	}

	// 走默认分支（最后一个输出）
	return wfCtx, "no case matched, using default branch", nil
}

// ProcessWithBranch 实现 BranchProcessor 接口
func (c *SwitchConfig) ProcessWithBranch(ctx *ctx.Context, wfCtx *models.WorkflowContext) (*models.NodeOutput, error) {
	index, caseName, err := c.evaluateCases(wfCtx)
	if err != nil {
		return nil, fmt.Errorf("switch processor: failed to evaluate cases: %v", err)
	}

	output := &models.NodeOutput{
		WfCtx: wfCtx,
	}

	if index >= 0 {
		output.BranchIndex = &index
		if caseName != "" {
			output.Message = fmt.Sprintf("matched case[%d]: %s", index, caseName)
		} else {
			output.Message = fmt.Sprintf("matched case[%d]", index)
		}
	} else {
		// 默认分支的索引是 cases 数量（即最后一个输出端口）
		defaultIndex := len(c.Cases)
		output.BranchIndex = &defaultIndex
		output.Message = "no case matched, using default branch"
	}

	return output, nil
}

// evaluateCases 评估所有分支条件
// 返回匹配的分支索引和分支名称，如果没有匹配返回 -1
func (c *SwitchConfig) evaluateCases(wfCtx *models.WorkflowContext) (int, string, error) {
	for i := range c.Cases {
		matched, err := c.evaluateCaseCondition(&c.Cases[i], wfCtx)
		if err != nil {
			return -1, "", fmt.Errorf("case[%d] evaluation error: %v", i, err)
		}
		if matched {
			return i, c.Cases[i].Name, nil
		}
	}
	return -1, "", nil
}

// evaluateCaseCondition 评估单个分支条件
func (c *SwitchConfig) evaluateCaseCondition(caseItem *SwitchCase, wfCtx *models.WorkflowContext) (bool, error) {
	mode := caseItem.Mode
	if mode == "" {
		mode = ConditionModeExpression // 默认表达式模式
	}

	switch mode {
	case ConditionModeTags:
		return c.evaluateTagsCondition(caseItem, wfCtx.Event)
	default:
		return c.evaluateExpressionCondition(caseItem.Condition, wfCtx)
	}
}

// evaluateExpressionCondition 评估表达式条件
func (c *SwitchConfig) evaluateExpressionCondition(condition string, wfCtx *models.WorkflowContext) (bool, error) {
	if condition == "" {
		return false, nil
	}

	var defs = []string{
		"{{ $event := .Event }}",
		"{{ $labels := .Event.TagsMap }}",
		"{{ $value := .Event.TriggerValue }}",
		"{{ $env := .Env }}",
	}

	text := strings.Join(append(defs, condition), "")

	tpl, err := template.New("switch_condition").Funcs(tplx.TemplateFuncMap).Parse(text)
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
func (c *SwitchConfig) evaluateTagsCondition(caseItem *SwitchCase, event *models.AlertCurEvent) (bool, error) {
	// 如果没有配置任何过滤条件，默认返回 false（不匹配）
	if len(caseItem.parsedLabelKeys) == 0 && len(caseItem.parsedAttributes) == 0 {
		return false, nil
	}

	// 匹配标签 (TagsMap)
	if len(caseItem.parsedLabelKeys) > 0 {
		tagsMap := event.TagsMap
		if tagsMap == nil {
			tagsMap = make(map[string]string)
		}
		if !alertCommon.MatchTags(tagsMap, caseItem.parsedLabelKeys) {
			return false, nil
		}
	}

	// 匹配属性 (JsonTagsAndValue - 所有 JSON 字段)
	if len(caseItem.parsedAttributes) > 0 {
		attributesMap := event.JsonTagsAndValue()
		if !alertCommon.MatchTags(attributesMap, caseItem.parsedAttributes) {
			return false, nil
		}
	}

	return true, nil
}
