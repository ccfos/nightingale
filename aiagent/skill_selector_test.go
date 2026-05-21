package aiagent

import (
	"os"
	"strings"
	"reflect"
	"testing"
)

func TestParseSelectionResponse(t *testing.T) {
	s := &LLMSkillSelector{}
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			"fenced json with CoT prefix",
			"选择理由：用户要批量导入，应当选导入类 skill。\n```json\n[\"n9e-import-prom-rule\"]\n```\n",
			[]string{"n9e-import-prom-rule"},
		},
		{
			"bare array",
			`["a","b"]`,
			[]string{"a", "b"},
		},
		{
			"CoT contains [node-exporter.yml], JSON at end",
			"理由：用户给的 [node-exporter.yml] 是 YAML 文件\n[\"n9e-import-prom-rule\"]",
			[]string{"n9e-import-prom-rule"},
		},
		{
			"empty array",
			"理由：没匹配\n[]",
			nil,
		},
		{
			"plain text no array",
			"我无法选择",
			nil,
		},
		{
			"fenced without json marker",
			"```\n[\"x\"]\n```",
			[]string{"x"},
		},
	}
	for _, c := range cases {
		got := s.parseSelectionResponse(c.in)
		// nil vs []string{} 视作等价
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("[%s] got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBuildSelectionPromptRendersExamples(t *testing.T) {
	s := &LLMSkillSelector{}
	skills := []*SkillMetadata{
		{
			Name:        "n9e-create-alert-rule",
			Description: "**创建单条告警规则**。⚠️ 不要用这个 skill 做批量导入。",
			Examples:    []string{"帮我建一条 CPU 告警"},
		},
		{
			Name:        "n9e-import-prom-rule",
			Description: "**批量导入 YAML 文件**到夜莺。⚠️ 不要用这个 skill 做单条创建。",
			Examples:    []string{"帮我导入 https://.../node-exporter.yml"},
		},
	}
	out := s.buildSelectionPrompt(skills, 2)
	for _, want := range []string{
		"动词", "不要被", "典型问法",
		"`n9e-create-alert-rule`",
		"`n9e-import-prom-rule`",
		"帮我建一条 CPU 告警",
		"帮我导入 https://.../node-exporter.yml",
		"选 n9e-create-alert-rule",
		"选 n9e-import-prom-rule",
		"选择理由：",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestLoadMetadata_HandlesBlockScalarDescription(t *testing.T) {
	// 用临时文件喂给 registry — 历史 bug 会在第 3 行 yaml 报 missing ":"
	dir := t.TempDir()
	skillDir := dir + "/mock-skill"
	if err := os.MkdirAll(skillDir, 0o755); err != nil { t.Fatal(err) }
	body := `---
name: mock-skill
description: |
  **批量导入 YAML 文件**到夜莺（一次性建一组规则）。
  ⚠️ 不要用这个 skill 做单条创建——用 n9e-create-alert-rule。
  触发：导入 / import / 批量 / URL。
examples:
  - "帮我导入 https://.../node-exporter.yml"
  - "把这个 yaml 里的告警建到 n9e"
builtin_tools:
  - http_fetch
---
# body`
	if err := os.WriteFile(skillDir+"/SKILL.md", []byte(body), 0o644); err != nil { t.Fatal(err) }

	r := NewSkillRegistry(dir)
	sk := r.GetByName("mock-skill")
	if sk == nil {
		t.Fatal("skill not loaded — block scalar parse regressed")
	}
	if len(sk.Examples) != 2 || sk.Examples[0] != "帮我导入 https://.../node-exporter.yml" {
		t.Errorf("examples wrong: %#v", sk.Examples)
	}
	if len(sk.BuiltinTools) != 1 || sk.BuiltinTools[0] != "http_fetch" {
		t.Errorf("builtin_tools wrong: %#v", sk.BuiltinTools)
	}
}

func TestIsExplicitSkillReference(t *testing.T) {
	name := "n9e-import-prom-rule"
	cases := []struct {
		text string
		want bool
	}{
		// 命中：被引号 / 反引号 / 斜杠 / @ 包裹
		{"试用 `" + name + "` 这个 skill", true},
		{"用 '" + name + "' 处理", true},
		{`call "` + name + `" please`, true},
		{"调 /" + name + " 来导入", true},
		{"see @" + name + " for usage", true},

		// 不命中：裸出现（可能是粘贴的日志 / 文档 / 顺口提到）
		{"导入 " + name + " 的某个文件，但其实不是真要用它", false},
		{"[ERROR] " + name + " timeout 1s", false},
		{name + " is a great skill, anyway help me create something", false},
		{"", false},
		{"unrelated text", false},
	}
	for _, c := range cases {
		got := isExplicitSkillReference(c.text, name)
		if got != c.want {
			t.Errorf("isExplicitSkillReference(%q, %q) = %v, want %v", c.text, name, got, c.want)
		}
	}
}
