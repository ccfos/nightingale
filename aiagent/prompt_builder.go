package aiagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
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

// appendGuidedFollowup 在系统提示词末尾（利用 recency）追加收尾建议规则，仅交互式路径注入——
// workflow/事件路径输出会被下游结构化消费，追加"下一步"建议是噪声甚至会破坏该值。
func (a *Agent) appendGuidedFollowup(sb *strings.Builder) {
	if !a.cfg.GuidedFollowup {
		return
	}
	sb.WriteString("\n")
	sb.WriteString(prompts.GuidedFollowupRule)
}

// buildDirectSystemPrompt 构建 Direct 模式系统提示词。
// 跟 ReAct 版的关键差异：
//   - 不包含 ReactSystemPrompt（~70 行格式说明），节省 ~1.5KB 输入 token
//   - 不包含 tools 列表（Direct 模式不调工具）
//   - 保留 Skills 内容（真正的知识，决定回答质量）
//   - 保留 Env 信息（语言、时间等上下文）
func (a *Agent) buildDirectSystemPrompt(rc *runCtx) string {
	var sb strings.Builder

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

	sb.WriteString(llm.BuildEnvSection())

	a.appendGuidedFollowup(&sb)

	return sb.String()
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

	a.appendGuidedFollowup(&sb)

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

	// plan 模式的最终答案由 synthesis 产出，收尾建议也要带上
	a.appendGuidedFollowup(&sb)

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

	// 格式兜底层：deepseek-v4-pro 输出格式漂移严重，会用非 ReAct 形态发工具调用。
	// 逐行扫描前先归一化成规范的 Action/Action Input，否则 step.Action 为空 →
	// runReActLoop 当成 Final Answer 直接返回、工具不执行（executed_tools 始终 false）。
	//
	//  1) XML 工具调用：<tool_call name="X">{json}</tool_call>（含批量/畸形闭合标签）
	response = normalizeXMLToolCall(response)
	//  2) 行内标记：Thought: ...。Action: X 黏在一行
	response = normalizeReActMarkers(response)

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

var (
	// 匹配 name 作为 XML 属性的工具调用：<tool_call name="X"> ... {json} ... </tool_call>。
	// 开/闭标签都容忍 tool_call / tool_calls 混用（模型常写畸形）；(?s) 让 . 跨行；
	// (.*?) 非贪婪，确保多个工具调用时只吃到第一个的闭合标签。
	// 注意：要求 name 是 XML 属性；name 写在 JSON 内部的 <tool_call>{"name":...}</tool_call>
	// （Nous/Hermes 形态）不匹配。
	xmlToolCallRe = regexp.MustCompile(`(?s)<tool_calls?\s+name=["']([^"']+)["']\s*>(.*?)</tool_calls?>`)
	// 行尾残留的 <tool_calls> 外层包裹标签，提取 Thought 前缀时一并剥掉。
	openToolCallsTagRe = regexp.MustCompile(`(?s)<tool_calls?[^>]*>\s*$`)
)

// normalizeXMLToolCall 把 name 作为 XML 属性的工具调用 <tool_call name="X">{json}</tool_call>
// 改写成规范的 ReAct 文本形态，让 parseReActResponse 能识别。某些模型（尤其 deepseek-v4-pro）
// 无视 ReAct 格式，直接吐这种 XML 而不是 Action/Action Input：
//
//	<tool_calls>
//	<tool_call name="read_file">
//	{"base": "...", "path": "..."}
//	</tool_call>
//	...
//	</tool_calls>
//
// 逐行扫描扫不到 Action，step.Action 为空，runReActLoop 把整段 XML 当 Final Answer
// 返回 → 工具永不执行。
//
// 只转换【第一个】工具调用（ReAct 一步一个 Action；其余的下一轮模型会再发）。
// 当响应已含规范的行首 Action/Final Answer，或没有【此形态】的 XML 工具调用时，原样返回。
//
// 仅覆盖 name 作为 XML 属性的形态（deepseek 自定义形态）。以下两种均不匹配本函数：
//   - Nous/Hermes：name 写在 JSON 内 —— <tool_call>{"name":...,"arguments":...}</tool_call>
//   - Anthropic 原生：<function_calls><invoke name="...">…</invoke></function_calls>
func normalizeXMLToolCall(response string) string {
	// 已有规范 Action 就不要去动一个本来正常的响应。
	if hasReActActionMarker(response) {
		return response
	}
	loc := xmlToolCallRe.FindStringSubmatchIndex(response)
	if loc == nil {
		return response
	}
	name := strings.TrimSpace(response[loc[2]:loc[3]])
	body := response[loc[4]:loc[5]]
	input := extractJSONObject(body)
	if input == "" {
		input = strings.TrimSpace(body)
	}

	// 工具调用块之前的散文是模型的推理，作为 Thought 保留。剥掉行尾可能残留的
	// 外层 <tool_calls> 包裹标签。
	prefix := strings.TrimSpace(response[:loc[0]])
	prefix = strings.TrimSpace(openToolCallsTagRe.ReplaceAllString(prefix, ""))

	var b strings.Builder
	if prefix != "" {
		if !strings.HasPrefix(prefix, "Thought:") {
			b.WriteString("Thought: ")
		}
		b.WriteString(prefix)
		b.WriteString("\n")
	}
	b.WriteString("Action: ")
	b.WriteString(name)
	b.WriteString("\nAction Input: ")
	b.WriteString(input)
	return b.String()
}

// hasReActActionMarker 报告响应里是否有行首的 Action: / Final Answer: 标记。
func hasReActActionMarker(response string) bool {
	for _, line := range strings.Split(response, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "Action:") || strings.HasPrefix(t, "Final Answer:") {
			return true
		}
	}
	return false
}

var (
	// 工具调用 JSON 指纹：同时出现 "name": 与 "arguments":/"parameters":。
	// 这是模型用原生/并行 function-calling 形态泄漏到正文的特征
	// （OpenAI 风格 [{"name":...,"arguments":{...}}, ...] 或单对象），
	// ReAct 文本解析器无法把它变成 Action。
	toolCallNameKeyRe = regexp.MustCompile(`"name"\s*:`)
	toolCallArgsKeyRe = regexp.MustCompile(`"(arguments|parameters)"\s*:`)
	// 未被 normalizeXMLToolCall 归一化的工具调用标签：
	// Nous/Hermes <tool_call>{...}</tool_call>、Anthropic <function_calls><invoke …>、
	// 以及裸 <tool_calls> 外层包裹。
	toolCallTagRe = regexp.MustCompile(`(?i)<(tool_call|tool_calls|function_call|function_calls|invoke)\b`)
	// OpenAI/通用 envelope：{"tool_calls":[…]} / {"function_call":{…}}。
	toolCallEnvelopeKeyRe = regexp.MustCompile(`"(tool_calls|function_call)"\s*:`)
)

// looksLikeToolCall 判断一段【已解析出空 Action】的响应，是否其实是模型用非 ReAct
// 形态发出的工具调用（裸 JSON 工具调用数组/对象、OpenAI/Anthropic 原生 function-calling
// 语法、或未被归一化的 tool_call XML 标签）。
//
// runReActLoop 用它把【真正的最终答案】（应直接返回）与【格式漂移】（必须纠正后重试，
// 而不是当成最终答案静默接受）区分开。后者若被当成 Final Answer，工具一个都不执行，
// 对话在第 0 轮就"假完成"终止。
//
// 仅在 step.Action=="" 时调用。要求 JSON 形态【同时】命中 "name": 与
// "arguments":/"parameters":，避免把恰好含其中一个词的正常 markdown 答案误判为工具调用。
func looksLikeToolCall(response string) bool {
	if toolCallTagRe.MatchString(response) || toolCallEnvelopeKeyRe.MatchString(response) {
		return true
	}
	return toolCallNameKeyRe.MatchString(response) && toolCallArgsKeyRe.MatchString(response)
}

// reactFormatCorrection 是检测到工具调用格式漂移后回灌给模型的 Observation 文本，
// 指导它改回规范 ReAct 形态、且一次只发一个工具。
const reactFormatCorrection = `Observation: ⚠️ Format error: your previous response was NOT in the required ReAct format. ` +
	`No "Action:" line was found, so no tool was executed and nothing has happened yet.

You appear to have emitted a tool call as raw JSON, a parallel/batch tool-call array, ` +
	"a ```json fenced block, a <tool_call> tag, or native function-calling syntax. " +
	`None of these are supported by this host.

You MUST reply in EXACTLY this three-line text format, and call only ONE tool per response:

Thought: <your reasoning>
Action: <one tool name>
Action Input: <JSON arguments for that one tool>

Do NOT output JSON arrays, multiple tool calls in one response, code fences, XML tags, or native ` +
	`function-calling syntax. If you need several tools, call the FIRST one now — you will get its ` +
	`Observation and can call the next one afterwards. Re-emit your intended tool call now in the correct format.`

// extractJSONObject 取 s 中第一个 '{' 到最后一个 '}' 的子串；找不到返回空串。
func extractJSONObject(s string) string {
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return ""
}

// normalizeReActMarkers 把模型黏在行尾的 ReAct 结构标记（Action:/Action Input:/
// Final Answer:）提升到新行行首，使 parseReActResponse 的行首前缀扫描能识别它们。
//
// 改写严格限制在 header 段（到第一个 Action Input: / Final Answer: 的值开始之前）。
// 值区是不透明的——最终答案的 markdown、工具参数 JSON 里可能合法地包含 "Action:"
// 或 "Observation:" 字样，改写值区会损坏答案或在 Observation 处被截断。
func normalizeReActMarkers(response string) string {
	// 定位值区起点：第一个出现的 Action Input: 或 Final Answer: 标记之后。
	boundaryIdx, boundaryLen := len(response), 0
	for _, m := range []string{"Action Input:", "Final Answer:"} {
		if i := strings.Index(response, m); i >= 0 && i < boundaryIdx {
			boundaryIdx, boundaryLen = i, len(m)
		}
	}
	head, tail := response[:boundaryIdx+boundaryLen], response[boundaryIdx+boundaryLen:]

	// 只在 header 段提升标记。"Action:" 与 "Action Input:" 不会互相误匹配
	// （冒号位置不同："Action:" vs "Action "），处理顺序无关紧要。
	for _, m := range []string{"Final Answer:", "Action Input:", "Action:"} {
		head = promoteMarkerToLineStart(head, m)
	}
	return head + tail
}

// promoteMarkerToLineStart 在每个不在行首的 marker 前插入换行。行首前导空格由调用
// 方逐行 TrimSpace 兜底，这里只需保证 marker 前有换行。
func promoteMarkerToLineStart(s, marker string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, marker)
		if i < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		if i > 0 && s[i-1] != '\n' { // 已在行首则不重复插入
			b.WriteByte('\n')
		}
		b.WriteString(marker)
		s = s[i+len(marker):]
	}
	return b.String()
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
