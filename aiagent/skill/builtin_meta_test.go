package skill

import (
	"io/fs"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/skill/embedded"
)

// TestAllBuiltinFrontmatterValid 守卫每个内置 skill 的 frontmatter 都能解析。
//
// 解析失败是静默的：loadBuiltin 只打一行 WARNING 就跳过，该 skill 从「可用技能
// 目录」里彻底消失，模型永远不会 load_skill 它——线上表现只是"这技能怎么不生效"，
// 没人会去翻启动日志。v9.0.0 就带着 4 颗这样的哑弹发布（categraf-deploy-guide /
// host-health-diagnose / host-onboard-diagnose / notify-rule-copilot），根因是
// description 写成无引号 plain scalar，正文里的 ": " 让 YAML 直接报错。
//
// 结论：长 description 一律用块标量 `>-`，冒号和引号都不用转义。
func TestAllBuiltinFrontmatterValid(t *testing.T) {
	entries, err := fs.ReadDir(embedded.FS, BuiltinEmbedRoot)
	if err != nil {
		t.Fatalf("read builtin embed root: %v", err)
	}

	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		count++
		name := e.Name()

		data, err := fs.ReadFile(embedded.FS, BuiltinEmbedRoot+"/"+name+"/"+skillMDName)
		if err != nil {
			t.Errorf("%s: read SKILL.md: %v", name, err)
			continue
		}
		meta, _, ok := ParseMarkdown(string(data))
		if !ok {
			t.Errorf("%s/SKILL.md: frontmatter 解析失败或 name 为空（最常见原因：description 未加引号却含 \": \"，改用 `description: >-` 块标量）", name)
			continue
		}
		// name 必须与目录名一致：SkillRegistry 以 metadata.Name 建索引，而技能目录
		// 与 load_skill 用的是同一个名字，不一致会出现"目录里列着、加载不到"的错位。
		if meta.Name != name {
			t.Errorf("%s/SKILL.md: name=%q 与目录名不一致", name, meta.Name)
		}
	}

	if count == 0 {
		t.Fatal("embed 里一个内置 skill 都没有，构建约定被破坏了")
	}
	// 缓存路径（A2A AgentCard / SkillRegistry 都走它）必须一个不落地收下全部。
	if got := len(ListBuiltinFrontmatters()); got != count {
		t.Errorf("ListBuiltinFrontmatters 返回 %d 个，目录里有 %d 个", got, count)
	}
}
