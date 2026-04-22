package aiagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/ccfos/nightingale/v6/aiagent/prompts"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
)

// buildUserMessage 构建用户消息
//
// 三条路径，按优先级：
//  1. UserPromptRendered 非空：已渲染好，直接返回（chat 路径，避免误把用户原文里的
//     {{...}} 当模板语法解析）。
//  2. UserPromptTemplate 非空：按 Go 模板解析并渲染（adapter 路径，用户 JSON
//     config 里显式写模板）。
//  3. 都为空：走 buildDefaultUserMessage 兜底。
func (a *Agent) buildUserMessage(req *AgentRequest) (string, error) {
	if a.cfg.UserPromptRendered != "" {
		return a.cfg.UserPromptRendered, nil
	}
	if a.cfg.UserPromptTemplate == "" {
		return a.buildDefaultUserMessage(req), nil
	}

	// 构建模板渲染上下文
	templateData := buildTemplateData(req)

	t, err := template.New("user_prompt").Funcs(template.FuncMap(tplx.TemplateFuncMap)).Parse(a.cfg.UserPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse user prompt template: %v", err)
	}

	var body bytes.Buffer
	if err = t.Execute(&body, templateData); err != nil {
		return "", fmt.Errorf("failed to execute user prompt template: %v", err)
	}

	return body.String(), nil
}

// buildDefaultUserMessage 构建默认用户消息（通用，不依赖 Event）
func (a *Agent) buildDefaultUserMessage(req *AgentRequest) string {
	var sb strings.Builder

	sb.WriteString("Please complete the task based on your instructions.\n\n")

	// 如果有明确的用户消息
	if req.UserMessage != "" {
		sb.WriteString("**User Request**:\n")
		sb.WriteString(req.UserMessage)
		sb.WriteString("\n\n")
	}

	// 展示输入参数信息
	if len(req.Params) > 0 {
		sb.WriteString("**Context**:\n")
		for k, v := range req.Params {
			if isSensitiveKey(k) {
				continue
			}
			sb.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Use the available tools to gather information and complete the task.")
	return sb.String()
}

// isSensitiveKey 判断是否是敏感信息的 key
func isSensitiveKey(k string) bool {
	lower := strings.ToLower(k)
	return strings.Contains(lower, "key") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "password") ||
		strings.Contains(lower, "token")
}

// buildReActSystemPrompt 构建 ReAct 系统提示词
func (a *Agent) buildReActSystemPrompt(rc *runCtx) string {
	var sb strings.Builder

	// 基础提示词
	sb.WriteString(prompts.ReactSystemPrompt)
	sb.WriteString("\n\n")

	// Skills 知识（如果有）
	if len(rc.skills) > 0 {
		skillContents := make([]string, len(rc.skills))
		for i, skill := range rc.skills {
			if len(rc.skills) > 1 {
				skillContents[i] = fmt.Sprintf("### %s\n\n%s", skill.Metadata.Name, skill.MainContent)
			} else {
				skillContents[i] = skill.MainContent
			}
		}
		sb.WriteString(llm.BuildSkillsSection(skillContents))
		sb.WriteString("**Important**: Follow the workflow and guidelines defined in the loaded Skills when applicable.\n\n")
	}

	// 工具说明
	if len(rc.tools) > 0 {
		sb.WriteString(llm.BuildToolsSection(convertToolsToInfo(rc.tools)))
	}

	// 环境信息
	sb.WriteString(llm.BuildEnvSection())

	return sb.String()
}

// buildPlanningPrompt 构建规划阶段的系统提示词
func (a *Agent) buildPlanningPrompt(rc *runCtx) string {
	var sb strings.Builder

	sb.WriteString(prompts.PlanSystemPrompt)
	sb.WriteString("\n\n")

	// Skills 知识
	if len(rc.skills) > 0 {
		skillContents := make([]string, len(rc.skills))
		for i, skill := range rc.skills {
			if len(rc.skills) > 1 {
				skillContents[i] = fmt.Sprintf("### %s\n\n%s", skill.Metadata.Name, skill.MainContent)
			} else {
				skillContents[i] = skill.MainContent
			}
		}
		sb.WriteString(llm.BuildSkillsSection(skillContents))
		sb.WriteString("**Important**: Use the workflow from the loaded Skills above to guide your planning.\n")
		sb.WriteString("The steps in your plan should follow the phases and methods defined in the Skills.\n\n")
	}

	// 工具列表（简洁版）
	if len(rc.tools) > 0 {
		sb.WriteString(llm.BuildToolsListBrief(convertToolsToInfo(rc.tools)))
	}

	return sb.String()
}

// buildStepExecutionPrompt 构建步骤执行的系统提示词
func (a *Agent) buildStepExecutionPrompt(step *PlanStep, previousFindings []string, rc *runCtx) string {
	var sb strings.Builder

	sb.WriteString(prompts.StepExecutionPrompt)
	sb.WriteString("\n\n")

	// Skills 知识
	if len(rc.skills) > 0 {
		skillContents := make([]string, len(rc.skills))
		for i, skill := range rc.skills {
			skillContents[i] = skill.MainContent
		}
		sb.WriteString(llm.BuildSkillsSection(skillContents))
	}

	// 当前步骤目标
	sb.WriteString(llm.BuildCurrentStepSection(step.Goal, step.Approach))

	// 之前的发现
	sb.WriteString(llm.BuildPreviousFindingsSection(previousFindings))

	// 工具列表
	if len(rc.tools) > 0 {
		sb.WriteString(llm.BuildToolsListBrief(convertToolsToInfo(rc.tools)))
	}

	sb.WriteString("\nUse 'Step Complete' as the Action when you have gathered enough information for this step.\n")

	return sb.String()
}

// buildSynthesisPrompt 构建综合分析提示词
func (a *Agent) buildSynthesisPrompt(rc *runCtx) string {
	var sb strings.Builder

	sb.WriteString(prompts.SynthesisPrompt)
	sb.WriteString("\n\n")

	if len(rc.skills) > 0 {
		skillContents := make([]string, len(rc.skills))
		for i, skill := range rc.skills {
			skillContents[i] = skill.MainContent
		}
		sb.WriteString(llm.BuildSkillsSection(skillContents))
	}

	return sb.String()
}

// buildStepUserMessage 构建步骤的用户消息（通用，不依赖 Event）
func (a *Agent) buildStepUserMessage(req *AgentRequest, step *PlanStep) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Step %d\n\n", step.StepNumber))
	sb.WriteString(fmt.Sprintf("**Goal**: %s\n", step.Goal))
	sb.WriteString(fmt.Sprintf("**Suggested Approach**: %s\n\n", step.Approach))

	// 从 Params 提供上下文
	sb.WriteString("## Task Context\n")
	if len(req.Params) > 0 {
		for k, v := range req.Params {
			if isSensitiveKey(k) {
				continue
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", k, v))
		}
	}

	sb.WriteString("\nPlease investigate according to this step's goal.")

	return sb.String()
}

// buildSynthesisUserMessage 构建综合分析的用户消息
func (a *Agent) buildSynthesisUserMessage(req *AgentRequest, plan *ExecutionPlan, allFindings []string) string {
	var sb strings.Builder

	// 从 Params 提取任务标题
	if taskName := req.Params["alert_name"]; taskName != "" {
		sb.WriteString(fmt.Sprintf("## Task: %s\n\n", taskName))
	} else if req.UserMessage != "" {
		title := req.UserMessage
		if len(title) > 100 {
			title = title[:100] + "..."
		}
		sb.WriteString(fmt.Sprintf("## Task: %s\n\n", title))
	}

	sb.WriteString("## Investigation Findings\n\n")
	for _, finding := range allFindings {
		sb.WriteString(fmt.Sprintf("- %s\n", finding))
	}

	sb.WriteString("\n## Step Summaries\n\n")
	for _, step := range plan.Steps {
		if step.Status == "completed" && step.Summary != "" {
			sb.WriteString(fmt.Sprintf("### Step %d: %s\n", step.StepNumber, step.Goal))
			sb.WriteString(fmt.Sprintf("%s\n\n", step.Summary))
		}
	}

	sb.WriteString("\nPlease synthesize these findings into a comprehensive analysis.")

	return sb.String()
}

// convertToolsToInfo 将 AgentTool 转换为 llm.ToolInfo
func convertToolsToInfo(src []AgentTool) []llm.ToolInfo {
	tools := make([]llm.ToolInfo, len(src))
	for i, tool := range src {
		params := make([]llm.ToolParamInfo, len(tool.Parameters))
		for j, param := range tool.Parameters {
			params[j] = llm.ToolParamInfo{
				Name:        param.Name,
				Type:        param.Type,
				Description: param.Description,
				Required:    param.Required,
			}
		}
		tools[i] = llm.ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  params,
		}
	}
	return tools
}

// buildTemplateData 构建统一的模板渲染上下文
// 新模板使用 .Params / .Vars；旧模板（adapter 路径）通过 TemplateExtra 注入 .Event / .Inputs
func buildTemplateData(req *AgentRequest) map[string]interface{} {
	data := map[string]interface{}{
		"Params": req.Params,
		"Vars":   req.Vars,
	}

	// 合并 adapter 注入的兼容字段（Event, Inputs 等）
	for k, v := range req.TemplateExtra {
		data[k] = v
	}

	return data
}

// parseReActResponse 解析 LLM 响应，提取 Thought、Action、Action Input。
//
// 同时识别 "Final Answer: <result>" 这种简写形式——很多模型（尤其是国产/
// 开源模型）会用 "Final Answer: ..." 替代标准的 "Action: Final Answer\n
// Action Input: ..." 两行写法。识别到简写时归一化为标准内部表示，让下游
// IsComplete 判断和 parseResponse 解析器都能按统一格式处理。
func (a *Agent) parseReActResponse(response string) ReActStep {
	step := ReActStep{}

	lines := strings.Split(response, "\n")
	var currentField string
	var currentValue strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Thought:") {
			if currentField != "" {
				setStepField(&step, currentField, strings.TrimSpace(currentValue.String()))
			}
			currentField = "thought"
			currentValue.Reset()
			currentValue.WriteString(strings.TrimPrefix(line, "Thought:"))
		} else if strings.HasPrefix(line, "Final Answer:") {
			// Shorthand: "Final Answer: <result>" → Action="Final Answer",
			// ActionInput=<result>. Equivalent to the canonical two-line form.
			if currentField != "" {
				setStepField(&step, currentField, strings.TrimSpace(currentValue.String()))
			}
			step.Action = ActionFinalAnswer
			currentField = "action_input"
			currentValue.Reset()
			currentValue.WriteString(strings.TrimPrefix(line, "Final Answer:"))
		} else if strings.HasPrefix(line, "Action:") {
			if currentField != "" {
				setStepField(&step, currentField, strings.TrimSpace(currentValue.String()))
			}
			currentField = "action"
			currentValue.Reset()
			currentValue.WriteString(strings.TrimPrefix(line, "Action:"))
		} else if strings.HasPrefix(line, "Action Input:") {
			if currentField != "" {
				setStepField(&step, currentField, strings.TrimSpace(currentValue.String()))
			}
			currentField = "action_input"
			currentValue.Reset()
			currentValue.WriteString(strings.TrimPrefix(line, "Action Input:"))
		} else if strings.HasPrefix(line, "Observation:") {
			// 防御性兜底：如果模型没尊重 stop 序列继续自造 Observation，
			// 不要把它续写到 action_input，立刻终结当前字段并丢弃后续内容。
			if currentField != "" {
				setStepField(&step, currentField, strings.TrimSpace(currentValue.String()))
				currentField = ""
				currentValue.Reset()
			}
			break
		} else if currentField != "" {
			currentValue.WriteString("\n")
			currentValue.WriteString(line)
		}
	}

	if currentField != "" {
		setStepField(&step, currentField, strings.TrimSpace(currentValue.String()))
	}

	return step
}

func setStepField(step *ReActStep, field, value string) {
	switch field {
	case "thought":
		step.Thought = value
	case "action":
		step.Action = value
	case "action_input":
		step.ActionInput = value
	}
}

// parsePlanResponse 解析 LLM 返回的计划
func (a *Agent) parsePlanResponse(response string) *ExecutionPlan {
	plan := &ExecutionPlan{}

	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")

	if jsonStart >= 0 && jsonEnd > jsonStart {
		jsonStr := response[jsonStart : jsonEnd+1]

		var parsed struct {
			TaskSummary string   `json:"task_summary"`
			Goal        string   `json:"goal"`
			FocusAreas  []string `json:"focus_areas"`
			Steps       []struct {
				StepNumber int    `json:"step_number"`
				Goal       string `json:"goal"`
				Approach   string `json:"approach"`
			} `json:"steps"`
		}

		if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil {
			plan.TaskSummary = parsed.TaskSummary
			plan.Goal = parsed.Goal
			plan.FocusAreas = parsed.FocusAreas

			for _, s := range parsed.Steps {
				plan.Steps = append(plan.Steps, PlanStep{
					StepNumber: s.StepNumber,
					Goal:       s.Goal,
					Approach:   s.Approach,
					Status:     "pending",
				})
			}
		}
	}

	// 兜底：默认计划
	if len(plan.Steps) == 0 {
		plan.Goal = "Investigate the task"
		plan.Steps = []PlanStep{
			{StepNumber: 1, Goal: "Gather initial information", Approach: "Query relevant data sources", Status: "pending"},
			{StepNumber: 2, Goal: "Identify patterns", Approach: "Look for anomalies or relevant patterns", Status: "pending"},
			{StepNumber: 3, Goal: "Synthesize findings", Approach: "Correlate findings and draw conclusions", Status: "pending"},
		}
	}

	return plan
}

// extractStepFindings 从步骤结果中提取关键发现
func (a *Agent) extractStepFindings(stepResult *AgentResponse) string {
	if stepResult.Content != "" {
		findings := stepResult.Content
		if len(findings) > 500 {
			findings = findings[:500] + "..."
		}
		return findings
	}
	return ""
}
