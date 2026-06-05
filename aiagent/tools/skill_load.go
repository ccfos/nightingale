package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	skillpkg "github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
)

func init() {
	register(defs.LoadSkill, loadSkill)
}

// loadSkill 读取 <SkillsPath>/<name>/SKILL.md，剥掉 frontmatter，把技能工作流文档
// 作为工具结果注入对话。配合两点机制工作：
//   - 系统提示词常驻「可用技能目录」（名 + 描述，见 appendSkillCatalog），模型据此
//     决定加载哪个技能；
//   - 加载结果是一条普通工具结果轮，经结构化 transcript 自动跨轮持久——
//     后续轮模型仍能看到技能内容，无需重复加载。
func loadSkill(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	name := strings.TrimSpace(getArgString(args, "name"))
	if name == "" {
		return "", fmt.Errorf("name is required (a skill name from the available-skills catalog)")
	}
	// 技能名不允许路径段（resolveBasePath 另有逃逸检查，这里是语义校验：
	// load_skill 只接受技能名，不是通用文件读取口——那是 read_file 的职责）。
	if strings.ContainsAny(name, "/\\") || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("invalid skill name: %q (must be a bare skill name from the catalog)", name)
	}

	filePath, err := resolveBasePath(deps, name, "SKILL.md")
	if err != nil {
		return "", fmt.Errorf("skill %q not found: %v", name, err)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read skill %q: %v", name, err)
	}

	body := string(data)
	if _, instructions, ok := skillpkg.ParseMarkdown(body); ok {
		body = instructions
	}
	if len(body) > aiagent.FileReadMaxBytes {
		body = body[:aiagent.FileReadMaxBytes] + "\n\n... (truncated, skill too large)"
	}

	return fmt.Sprintf("# Skill: %s\n\n%s\n\n---\n(以上技能工作流已加载进对话，后续轮次无需重复加载；请严格按其步骤执行)", name, body), nil
}
