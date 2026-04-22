package skill

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter 是 SKILL.md 顶部 YAML frontmatter 的解析结果。
//
// Frontmatter 格式：
//
//	---
//	name: my-skill
//	description: what this skill does
//	---
//	# Instructions body...
type Frontmatter struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license"`
	Compatibility string            `yaml:"compatibility"`
	Metadata      map[string]string `yaml:"metadata"`
	AllowedTools  string            `yaml:"allowed-tools"`
}

// ParseMarkdown 解析 SKILL.md：成功返回 (meta, body, true)。
// 当没有 frontmatter、格式不合法、或 meta.Name 为空时，ok=false，
// 调用方应当视为 "非法 SKILL.md" 并拒绝。body 依然会返回（去掉 frontmatter 的正文部分），
// 方便上游日志 / 兜底使用。
func ParseMarkdown(content string) (meta Frontmatter, instructions string, ok bool) {
	text := strings.TrimSpace(content)

	if !strings.HasPrefix(text, "---") {
		return meta, text, false
	}

	endIdx := strings.Index(text[3:], "\n---")
	if endIdx < 0 {
		return meta, text, false
	}

	frontmatter := text[3 : 3+endIdx]
	body := strings.TrimSpace(text[3+endIdx+4:]) // skip past closing ---

	if yaml.Unmarshal([]byte(frontmatter), &meta) != nil || meta.Name == "" {
		return meta, body, false
	}

	return meta, body, true
}
