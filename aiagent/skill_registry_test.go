package aiagent

import (
	"os"
	"testing"
)

func TestLoadMetadata_HandlesBlockScalarDescription(t *testing.T) {
	// 用临时文件喂给 registry — 历史 bug 会在第 3 行 yaml 报 missing ":"
	dir := t.TempDir()
	skillDir := dir + "/mock-skill"
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
name: mock-skill
description: |
  **批量导入 YAML 文件**到夜莺（一次性建一组规则）。
  ⚠️ 不要用这个 skill 做单条创建——用 n9e-create-alert-rule。
  触发：导入 / import / 批量 / URL。
builtin_tools:
  - http_fetch
---
# body`
	if err := os.WriteFile(skillDir+"/SKILL.md", []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewSkillRegistry(dir)
	sk := r.GetByName("mock-skill")
	if sk == nil {
		t.Fatal("skill not loaded — block scalar parse regressed")
	}
	if len(sk.BuiltinTools) != 1 || sk.BuiltinTools[0] != "http_fetch" {
		t.Errorf("builtin_tools wrong: %#v", sk.BuiltinTools)
	}
}
