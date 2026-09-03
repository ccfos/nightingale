package aiagent

import (
	"bytes"
	"fmt"
	"sort"
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

// appendSkillCatalog 在系统提示词常驻「可用技能目录」（名 + 一行描述），供模型经
// load_skill 工具按需加载（渐进披露：目录是技能进入上下文的唯一自取入口，
// 无 LLM 预选）。已预加载（rc.skills）的不再列出；无 registry / 目录为空时
// 不输出。按名字典序排序，保持提示词稳定（利于 prompt cache）。
//
// 选择纪律：先扫描再决定、选最具体的、一开始最多读一个、无明显适用就不读——
// 这四条显著降低模型"凭常识硬答"和"乱加载撑爆上下文"两类失败。
func (a *Agent) appendSkillCatalog(sb *strings.Builder, rc *runCtx) {
	if a.cfg.Skills == nil || a.skillRegistry == nil {
		return
	}
	loaded := map[string]bool{}
	for _, s := range rc.skills {
		if s != nil && s.Metadata != nil {
			loaded[s.Metadata.Name] = true
		}
	}
	var lines []string
	for _, m := range a.skillRegistry.ListAll() {
		// 私有 skill 对非授权用户不可见（含 fail-closed 的 deny-all）：从目录里剔除。
		if m == nil || loaded[m.Name] || a.isSkillHidden(m.Name) {
			continue
		}
		desc := m.Description
		if i := strings.IndexByte(desc, '\n'); i >= 0 {
			desc = desc[:i]
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", m.Name, desc))
	}
	if len(lines) == 0 {
		return
	}
	sort.Strings(lines)

	sb.WriteString("## Available Skills (on-demand)\n\n")
	sb.WriteString(`动手前先扫描以下技能目录：
- 恰好一个技能明显适用：先调用 load_skill(name) 读取其工作流，再严格按步骤执行；
- 多个技能可能适用：选描述最具体匹配的那个，再读取/执行；
- 没有技能明显适用：不要加载任何技能，直接处理。
约束：一开始最多只加载一个技能；其工作流指引已出现在上下文中时无需重复加载。

`)
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

// buildNativeSystemPrompt 构建工具循环模式的系统提示词：注入 Skills 与环境信息。
// 不在提示词里铺工具说明——工具经原生 tools 参数下发（见 buildNativeToolDefs）。
func (a *Agent) buildNativeSystemPrompt(rc *runCtx) string {
	var sb strings.Builder

	sb.WriteString(prompts.NativeSystemPrompt)
	sb.WriteString("\n\n")

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

	// 可用技能目录（按需 load_skill）
	a.appendSkillCatalog(&sb, rc)

	sb.WriteString(llm.BuildEnvSection())

	a.appendGuidedFollowup(&sb)

	return sb.String()
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
