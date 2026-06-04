package aiagent

import (
	"encoding/json"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/toolkits/pkg/logger"
)

// 本文件实现"工具的渐进披露"：
// 与技能的渐进披露对称——基线工具面保持精简，模型经 load_skill 取用某技能时，
// 该技能 frontmatter 声明的 builtin_tools 随之注入本轮循环的工具表，下一迭代
// 即可调用。跨轮场景下，Run 预备期从 history 的 load_skill 结果轮重建注入
// （transcript 是唯一事实源 I1，工具表可由它重建）。

// loadSkillResultPrefix 是 tools/skill_load.go 产出结果的固定头，两处必须保持
// 一致——历史回放靠它识别"哪些技能已加载"。
const loadSkillResultPrefix = "# Skill: "

// injectSkillTools 把名为 skillName 的技能声明的 builtin_tools 追加进 tools
// （按 Name 去重），返回新表与新增的工具名。registry 缺失或技能不存在时原样返回。
func (a *Agent) injectSkillTools(tools []AgentTool, skillName string) ([]AgentTool, []string) {
	if a.skillRegistry == nil || skillName == "" {
		return tools, nil
	}
	meta := a.skillRegistry.GetByName(skillName)
	if meta == nil || len(meta.BuiltinTools) == 0 {
		return tools, nil
	}
	var added []string
	for _, name := range meta.BuiltinTools {
		def, ok := GetBuiltinToolDef(name)
		if !ok {
			continue
		}
		before := len(tools)
		tools = appendToolIfAbsent(tools, def)
		if len(tools) > before {
			added = append(added, name)
			logger.Debugf("[Agent] injected tool %s (from skill %s)", name, skillName)
		}
	}
	return tools, added
}

// skillNameFromLoadArgs 从 load_skill 的调用参数 JSON 里取技能名。
func skillNameFromLoadArgs(arguments string) string {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(arguments), &p); err != nil {
		return ""
	}
	return strings.TrimSpace(p.Name)
}

// skillNameFromLoadResult 从 load_skill 的结果文本（"# Skill: <name>\n..."）里
// 取技能名；非该形态返回空。
func skillNameFromLoadResult(result string) string {
	if !strings.HasPrefix(result, loadSkillResultPrefix) {
		return ""
	}
	rest := result[len(loadSkillResultPrefix):]
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[:i]
	}
	return strings.TrimSpace(rest)
}

// appendToolsFromLoadedSkills 扫描 history 中已成功加载的技能（load_skill 结果
// 轮），把它们声明的 builtin_tools 注入 tools——跨轮回放：上一轮加载的技能在
// 本轮 transcript 里仍可见，其工具也必须仍可调。覆盖两种协议形态：
//   - native：role=tool 且 ToolName=load_skill 的结果轮；
//   - ReAct 文本：user 轮 "Observation: # Skill: <name>"。
func (a *Agent) appendToolsFromLoadedSkills(tools []AgentTool, history []ChatMessage) []AgentTool {
	if a.skillRegistry == nil {
		return tools
	}
	for _, m := range history {
		name := ""
		switch {
		case m.Role == llm.RoleTool && m.ToolName == "load_skill":
			name = skillNameFromLoadResult(m.Content)
		case m.Role == "user":
			obs := strings.TrimSpace(m.Content)
			if strings.HasPrefix(obs, "Observation:") {
				name = skillNameFromLoadResult(strings.TrimSpace(strings.TrimPrefix(obs, "Observation:")))
			}
		}
		if name == "" {
			continue
		}
		tools, _ = a.injectSkillTools(tools, name)
	}
	return tools
}
