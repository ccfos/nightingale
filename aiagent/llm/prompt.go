package llm

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

// ToolInfo 工具信息（用于提示词构建）
type ToolInfo struct {
	Name        string
	Description string
	Parameters  []ToolParamInfo
}

// ToolParamInfo 工具参数信息
type ToolParamInfo struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

// PromptData 提示词模板数据
type PromptData struct {
	Platform string // 操作系统
	Date     string // 当前日期
}

// BuildToolsSection 构建工具描述段落
func BuildToolsSection(tools []ToolInfo) string {
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Available Tools\n\n")

	for _, tool := range tools {
		sb.WriteString(fmt.Sprintf("### %s\n", tool.Name))
		sb.WriteString(fmt.Sprintf("%s\n", tool.Description))

		if len(tool.Parameters) > 0 {
			sb.WriteString("Parameters:\n")
			for _, param := range tool.Parameters {
				required := ""
				if param.Required {
					required = " (required)"
				}
				sb.WriteString(fmt.Sprintf("- %s (%s)%s: %s\n", param.Name, param.Type, required, param.Description))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// BuildToolsListBrief 构建简洁的工具列表（用于 Plan 模式）
func BuildToolsListBrief(tools []ToolInfo) string {
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Available Tools\n\n")

	for _, tool := range tools {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", tool.Name, tool.Description))
	}

	return sb.String()
}

// BuildEnvSection 构建环境信息段落
func BuildEnvSection() string {
	var sb strings.Builder
	sb.WriteString("## Environment\n\n")
	sb.WriteString(fmt.Sprintf("- Platform: %s\n", runtime.GOOS))
	sb.WriteString(fmt.Sprintf("- Date: %s\n", time.Now().Format("2006-01-02")))
	return sb.String()
}

// BuildSkillsSection 构建技能指导段落
func BuildSkillsSection(skillContents []string) string {
	if len(skillContents) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 专项技能指导\n\n")

	if len(skillContents) == 1 {
		sb.WriteString("你已被加载以下专项技能，请参考技能中的流程：\n\n")
		sb.WriteString(skillContents[0])
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("你已被加载以下专项技能，请参考技能中的流程来制定执行计划：\n\n")
		for i, content := range skillContents {
			sb.WriteString(fmt.Sprintf("### 技能 %d\n\n", i+1))
			sb.WriteString(content)
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

// BuildPreviousFindingsSection 构建之前发现段落
func BuildPreviousFindingsSection(findings []string) string {
	if len(findings) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Previous Findings\n\n")
	for _, finding := range findings {
		sb.WriteString(fmt.Sprintf("- %s\n", finding))
	}
	sb.WriteString("\n")

	return sb.String()
}

// BuildCurrentStepSection 构建当前步骤段落
func BuildCurrentStepSection(goal, approach string) string {
	var sb strings.Builder
	sb.WriteString("## Current Step\n\n")
	sb.WriteString(fmt.Sprintf("**Goal**: %s\n", goal))
	sb.WriteString(fmt.Sprintf("**Approach**: %s\n\n", approach))
	return sb.String()
}
