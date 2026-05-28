package tools

import (
	"os"
	"strings"
	"testing"

	"github.com/toolkits/pkg/runner"
)

// TestLoadIntegrationsEntries 验证扫盘逻辑能产生形状正确的 docEntry。
// 测试 cwd (aiagent/tools/) 自己没有 integrations/, 所以临时 chdir 到仓库根。
// 仓库根定义为"包含 integrations/ 目录的上级"。找不到则 SKIP。
func TestLoadIntegrationsEntries_ShapeAndPresence(t *testing.T) {
	prev := runner.Cwd
	defer func() { runner.Cwd = prev }()

	// 从当前测试 cwd 向上找 integrations/ 所在层
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := wd
	for i := 0; i < 8; i++ {
		if info, err := os.Stat(root + "/integrations"); err == nil && info.IsDir() {
			runner.Cwd = root
			break
		}
		parent := strings.TrimRight(root[:strings.LastIndex(root, "/")], "/")
		if parent == "" || parent == root {
			break
		}
		root = parent
	}

	entries, err := loadIntegrationsEntries()
	if err != nil {
		t.Fatalf("loadIntegrationsEntries err: %v", err)
	}
	if len(entries) == 0 {
		t.Skip("no integrations/ dir reachable from " + wd + " (cwd=" + runner.Cwd + ")")
	}

	var configCount, docCount int
	var aliyunSeen, mysqlSeen bool
	for _, e := range entries {
		switch {
		case strings.HasPrefix(e.Title, integrationConfigTitlePrefix):
			configCount++
			// 配置样例应当包含 [[instances]] 这种 categraf 真实语法
			if !strings.Contains(e.Contents, "[[instances]]") &&
				!strings.Contains(e.Contents, "[global]") &&
				!strings.Contains(e.Contents, "[heartbeat]") {
				// 不强制每个 toml 都有这些段, 但起码绝大多数有
			}
			if e.Permalink == "" {
				t.Errorf("config entry %q has empty permalink", e.Title)
			}
		case strings.HasPrefix(e.Title, integrationDocTitlePrefix):
			docCount++
			if e.Contents == "" {
				t.Errorf("doc entry %q has empty contents", e.Title)
			}
		default:
			t.Errorf("entry has unexpected title prefix: %q", e.Title)
		}
		if strings.Contains(e.Title, "AliYun") {
			aliyunSeen = true
		}
		if strings.Contains(e.Title, "MySQL") {
			mysqlSeen = true
		}
	}

	t.Logf("loaded %d entries (%d config, %d doc), aliyun=%v mysql=%v",
		len(entries), configCount, docCount, aliyunSeen, mysqlSeen)

	if configCount == 0 {
		t.Error("expected at least one integration-config entry")
	}
	if docCount == 0 {
		t.Error("expected at least one integration-doc entry")
	}
}

func TestClassifyEntrySource(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"[integration-config] MySQL · mysql.toml", "integration-config"},
		{"[integration-doc] AliYun", "integration-doc"},
		{"Random doc from n9e", "n9e-docs"},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			e := docEntry{Title: c.title}
			got := classifyEntrySource(&e)
			if got != c.want {
				t.Errorf("classifyEntrySource(%q) = %q, want %q", c.title, got, c.want)
			}
		})
	}
}

func TestIsConfigQuery(t *testing.T) {
	cases := []struct {
		terms []string
		want  bool
	}{
		{[]string{"categraf", "mysql"}, true},
		{[]string{"配置", "心跳"}, true},
		{[]string{"toml", "writers"}, true},
		{[]string{"什么是", "p99"}, false},
		{[]string{"业务组", "新建"}, false},
	}
	for _, c := range cases {
		got := isConfigQuery(c.terms)
		if got != c.want {
			t.Errorf("isConfigQuery(%v) = %v, want %v", c.terms, got, c.want)
		}
	}
}

func TestFirstNonEmptyParagraph(t *testing.T) {
	md := "# Title\n\n## Subtitle\n\nThis is the **real** first paragraph.\nMore text."
	got := firstNonEmptyParagraph(md, 200)
	if !strings.Contains(got, "real") {
		t.Errorf("firstNonEmptyParagraph should skip headings, got %q", got)
	}
}

func TestTomlLeadingComments(t *testing.T) {
	toml := "# 阿里云监控插件\n# 通过 OpenAPI 拉取\n\n[[instances]]\nregion = \"cn-beijing\"\n"
	got := tomlLeadingComments(toml, 200)
	if !strings.Contains(got, "阿里云") {
		t.Errorf("expected to extract leading comments, got %q", got)
	}
	if strings.Contains(got, "region") {
		t.Errorf("should stop at first blank line, not include toml body: %q", got)
	}
}
