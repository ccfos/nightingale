package aiagent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
)

// writeInjectTestSkill 写一个声明 builtin_tools 的临时技能。
func writeInjectTestSkill(t *testing.T, dir, name string, builtinTools []string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: " + name + "\ndescription: 测试\nbuiltin_tools:\n"
	for _, tn := range builtinTools {
		md += "  - " + tn + "\n"
	}
	md += "---\nbody"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
}

// registerFakeBuiltinTools 在 aiagent 包内测试里注册假内置工具（真实注册由
// aiagent/tools 子包 init 完成，包内测试导入它会形成循环依赖）。
func registerFakeBuiltinTools(names ...string) {
	for _, n := range names {
		RegisterBuiltinTool(n, &BuiltinTool{Definition: AgentTool{Name: n, Type: ToolTypeBuiltin}})
	}
}

func TestInjectSkillTools(t *testing.T) {
	registerFakeBuiltinTools("inj_tool_a", "inj_tool_b")
	dir := t.TempDir()
	// inj_tool_c 未注册，用于验证未注册名被跳过
	writeInjectTestSkill(t, dir, "inject-skill", []string{"inj_tool_a", "inj_tool_b", "inj_tool_c"})
	a := &Agent{cfg: &AgentConfig{Skills: &SkillConfig{}}, skillRegistry: NewSkillRegistry(dir)}

	tools := []AgentTool{{Name: "inj_tool_b"}} // 已存在的不重复注入
	tools, added := a.injectSkillTools(tools, "inject-skill")
	if len(added) != 1 || added[0] != "inj_tool_a" {
		t.Fatalf("expected only inj_tool_a injected, got %v", added)
	}
	if len(tools) != 2 {
		t.Fatalf("tool table wrong: %+v", tools)
	}

	// 不存在的技能：原样返回
	tools2, added2 := a.injectSkillTools(tools, "no-such-skill")
	if len(added2) != 0 || len(tools2) != len(tools) {
		t.Fatalf("unknown skill must be a no-op, added=%v", added2)
	}
}

func TestSkillNameFromLoadHelpers(t *testing.T) {
	if got := skillNameFromLoadArgs(`{"name":" n9e-doc-qa "}`); got != "n9e-doc-qa" {
		t.Fatalf("args parse: %q", got)
	}
	if got := skillNameFromLoadArgs(`not-json`); got != "" {
		t.Fatalf("bad json must yield empty, got %q", got)
	}
	if got := skillNameFromLoadResult("# Skill: n9e-doc-qa\n\n正文..."); got != "n9e-doc-qa" {
		t.Fatalf("result parse: %q", got)
	}
	if got := skillNameFromLoadResult("普通工具结果"); got != "" {
		t.Fatalf("non-skill result must yield empty, got %q", got)
	}
}

// TestAppendToolsFromLoadedSkills：跨轮回放——history 中的 load_skill 结果轮
// 能让已加载技能的工具重新进表。
func TestAppendToolsFromLoadedSkills(t *testing.T) {
	registerFakeBuiltinTools("inj_replay_a")
	dir := t.TempDir()
	writeInjectTestSkill(t, dir, "skill-native", []string{"inj_replay_a"})
	a := &Agent{cfg: &AgentConfig{Skills: &SkillConfig{}}, skillRegistry: NewSkillRegistry(dir)}

	history := []ChatMessage{
		{Role: "user", Content: "建个大盘"},
		{Role: llm.RoleTool, ToolCallID: "c1", ToolName: "load_skill", Content: "# Skill: skill-native\n\n工作流..."},
		// 普通观测不应触发
		{Role: llm.RoleTool, ToolCallID: "c2", ToolName: "query_prometheus", Content: "{}"},
	}
	tools := a.appendToolsFromLoadedSkills(nil, history)
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name] = true
	}
	if !names["inj_replay_a"] {
		t.Fatalf("replayed tools missing: %+v", tools)
	}
	if len(tools) != 1 {
		t.Fatalf("unexpected extra tools: %+v", tools)
	}
}
