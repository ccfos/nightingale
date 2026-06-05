package llm

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

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
