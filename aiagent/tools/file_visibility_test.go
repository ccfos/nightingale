package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
)

func seedSkillsDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	skillsPath := filepath.Join(root, "skills")
	for _, name := range []string{"pub", "priv", "foo", "foo-private"} {
		dir := filepath.Join(skillsPath, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("secret content for "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return skillsPath
}

// 通用文件工具（list_files/read_file/grep_files）必须与 load_skill 走同一道可见性门：
// 隐藏技能不可读/枚举，skills 根目录不可直接访问，deny-all 下连公共技能也拒绝。
func TestFileToolsSkillVisibility(t *testing.T) {
	skillsPath := seedSkillsDir(t)
	deps := &aiagent.ToolDeps{
		SkillsPath:       skillsPath,
		HiddenSkillNames: map[string]struct{}{"priv": {}, "foo-private": {}},
	}
	bg := context.Background()
	params := map[string]string{}

	// 可见技能：三工具均可读
	if _, err := listFiles(bg, deps, map[string]interface{}{"base": "pub"}, params); err != nil {
		t.Errorf("list_files(pub) should succeed: %v", err)
	}
	if out, err := readFile(bg, deps, map[string]interface{}{"base": "pub", "path": "SKILL.md"}, params); err != nil || out == "" {
		t.Errorf("read_file(pub) should succeed: %v", err)
	}
	// 前缀相关的可见 skill 读自身文件不受影响
	if _, err := readFile(bg, deps, map[string]interface{}{"base": "foo", "path": "SKILL.md"}, params); err != nil {
		t.Errorf("read_file(foo/SKILL.md) should succeed: %v", err)
	}

	// 隐藏技能 + skills 根目录：三工具均拒绝
	denied := []struct {
		name string
		fn   func() (string, error)
	}{
		{"list_files priv", func() (string, error) { return listFiles(bg, deps, map[string]interface{}{"base": "priv"}, params) }},
		{"read_file priv", func() (string, error) {
			return readFile(bg, deps, map[string]interface{}{"base": "priv", "path": "SKILL.md"}, params)
		}},
		{"grep_files priv", func() (string, error) {
			return grepFiles(bg, deps, map[string]interface{}{"base": "priv", "pattern": "secret"}, params)
		}},
		{"list_files root(.)", func() (string, error) { return listFiles(bg, deps, map[string]interface{}{"base": "."}, params) }},
		{"read_file root escape", func() (string, error) {
			return readFile(bg, deps, map[string]interface{}{"base": "..", "path": "x"}, params)
		}},
		// integrations 前缀 + .. 穿越回落到 skills：必须按清理后落点判定，不能因前缀跳过门。
		{"read_file integrations/.. traversal to priv", func() (string, error) {
			return readFile(bg, deps, map[string]interface{}{"base": "integrations/../skills/priv", "path": "SKILL.md"}, params)
		}},
		{"list_files integrations/.. traversal to skills root", func() (string, error) {
			return listFiles(bg, deps, map[string]interface{}{"base": "integrations/../skills"}, params)
		}},
		// 可见 skill 名是私有 skill 名前缀 + path 含 ../：subPath 边界不能靠字符串前缀，
		// 否则 base="foo" + path="../foo-private/..." 会逃到私有 skills/foo-private。
		{"read_file prefix traversal foo->foo-private", func() (string, error) {
			return readFile(bg, deps, map[string]interface{}{"base": "foo", "path": "../foo-private/SKILL.md"}, params)
		}},
		{"list_files prefix traversal foo->foo-private", func() (string, error) {
			return listFiles(bg, deps, map[string]interface{}{"base": "foo", "path": "../foo-private"}, params)
		}},
		{"grep_files prefix traversal foo->foo-private", func() (string, error) {
			return grepFiles(bg, deps, map[string]interface{}{"base": "foo", "pattern": "secret", "path": "../foo-private"}, params)
		}},
	}
	for _, tc := range denied {
		if _, err := tc.fn(); err == nil {
			t.Errorf("%s should be denied but succeeded", tc.name)
		}
	}

	// deny-all（fail-closed）：连公共技能也拒绝
	denyDeps := &aiagent.ToolDeps{SkillsPath: skillsPath, DenyAllSkills: true}
	if _, err := listFiles(bg, denyDeps, map[string]interface{}{"base": "pub"}, params); err == nil {
		t.Errorf("list_files(pub) under deny-all should be denied")
	}
	if _, err := readFile(bg, denyDeps, map[string]interface{}{"base": "pub", "path": "SKILL.md"}, params); err == nil {
		t.Errorf("read_file(pub) under deny-all should be denied")
	}
	if _, err := readFile(bg, denyDeps, map[string]interface{}{"base": "integrations/../skills/pub", "path": "SKILL.md"}, params); err == nil {
		t.Errorf("read_file(traversal to pub) under deny-all should be denied")
	}
}
