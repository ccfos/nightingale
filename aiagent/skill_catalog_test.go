package aiagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestSkill(t *testing.T, skillsDir, name, desc string) {
	t.Helper()
	dir := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: " + name + "\ndescription: " + desc + "\n---\nbody"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSkillCatalogInSystemPrompts: the always-present, sorted catalog
// appears in the system prompt, excludes preloaded skills, and is
// absent when the skill subsystem is off.
func TestSkillCatalogInSystemPrompts(t *testing.T) {
	skillsDir := t.TempDir()
	writeTestSkill(t, skillsDir, "test-skill-b", "技能B")
	writeTestSkill(t, skillsDir, "test-skill-a", "技能A")

	a := &Agent{
		cfg:           &AgentConfig{Skills: &SkillConfig{}},
		skillRegistry: NewSkillRegistry(skillsDir),
	}

	native := a.buildNativeSystemPrompt(&runCtx{})
	if !strings.Contains(native, "Available Skills (on-demand)") ||
		!strings.Contains(native, "- test-skill-a: 技能A") ||
		!strings.Contains(native, "- test-skill-b: 技能B") {
		t.Fatalf("native prompt missing catalog:\n%s", native)
	}
	// Deterministic ordering (prompt-cache friendly): a before b.
	if strings.Index(native, "- test-skill-a:") > strings.Index(native, "- test-skill-b:") {
		t.Fatal("catalog must be sorted by name")
	}

	// Preloaded skills (RequiredSkills/agent 绑定) are excluded from the catalog.
	loaded := &runCtx{skills: []*SkillContent{{Metadata: &SkillMetadata{Name: "test-skill-a"}, MainContent: "x"}}}
	got := a.buildNativeSystemPrompt(loaded)
	if strings.Contains(got, "- test-skill-a:") {
		t.Fatalf("preloaded skill must not be re-listed:\n%s", got)
	}
	if !strings.Contains(got, "- test-skill-b:") {
		t.Fatalf("other skills must stay listed:\n%s", got)
	}

	// Skill subsystem off → no catalog at all.
	off := &Agent{cfg: &AgentConfig{}, skillRegistry: NewSkillRegistry(skillsDir)}
	if strings.Contains(off.buildNativeSystemPrompt(&runCtx{}), "Available Skills") {
		t.Fatal("catalog must be absent when cfg.Skills is nil")
	}
}

// TestAppendToolIfAbsent: load_skill injection must not duplicate an existing tool.
func TestAppendToolIfAbsent(t *testing.T) {
	tools := []AgentTool{{Name: "load_skill"}}
	if got := appendToolIfAbsent(tools, AgentTool{Name: "load_skill"}); len(got) != 1 {
		t.Fatalf("duplicate appended: %+v", got)
	}
	if got := appendToolIfAbsent(tools, AgentTool{Name: "other"}); len(got) != 2 {
		t.Fatalf("append failed: %+v", got)
	}
}
