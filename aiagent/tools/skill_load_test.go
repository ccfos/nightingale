package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
)

func TestLoadSkill(t *testing.T) {
	root := t.TempDir()
	skills := filepath.Join(root, "skills")
	dir := filepath.Join(skills, "test-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: test-skill\ndescription: 测试技能\nmax_iterations: 8\n---\n\n# 工作流\n\n第一步：读取配置。"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := &aiagent.ToolDeps{SkillsPath: skills}

	out, err := loadSkill(context.Background(), deps, map[string]interface{}{"name": "test-skill"}, nil)
	if err != nil {
		t.Fatalf("loadSkill: %v", err)
	}
	if !strings.Contains(out, "# Skill: test-skill") || !strings.Contains(out, "# 工作流") {
		t.Fatalf("missing header/body: %q", out)
	}
	// Frontmatter must be stripped — it's metadata for the selector, not workflow.
	if strings.Contains(out, "max_iterations") || strings.Contains(out, "description: 测试技能") {
		t.Fatalf("frontmatter leaked into result: %q", out)
	}

	if _, err := loadSkill(context.Background(), deps, map[string]interface{}{"name": "nope"}, nil); err == nil {
		t.Fatal("missing skill must error")
	}
	if _, err := loadSkill(context.Background(), deps, map[string]interface{}{"name": "../secrets"}, nil); err == nil {
		t.Fatal("path traversal must be rejected")
	}
	if _, err := loadSkill(context.Background(), deps, map[string]interface{}{"name": ""}, nil); err == nil {
		t.Fatal("empty name must error")
	}
	if _, err := loadSkill(context.Background(), nil, map[string]interface{}{"name": "test-skill"}, nil); err == nil {
		t.Fatal("nil deps must error, not panic")
	}
}
