package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
)

// seedCodeCorpus 在临时 projectRoot 下搭 skill/ + code/{n9e,categraf}/ 结构，
// 返回指向 skill/ 的 deps（工具以 SkillsPath 父目录定位 code/）。
func seedCodeCorpus(t *testing.T) *aiagent.ToolDeps {
	t.Helper()
	root := t.TempDir()
	skillsPath := filepath.Join(root, "skill")
	if err := os.MkdirAll(skillsPath, 0o755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"code/manifest.json":                  `[{"repo":"n9e","ref":"v9.0.0"},{"repo":"categraf","ref":"v0.4.19"}]`,
		"code/.corpus_hash":                   "deadbeef\n",
		"code/n9e/TREE.md":                    "# n9e corpus\n",
		"code/n9e/models/alert_rule.go":       "package models\n\n// Severity levels\nconst Emergency = 1\nconst Warning = 2\nconst Notice = 3\n",
		"code/categraf/inputs/ping/ping.go":   "package ping\n\nconst metricName = \"ping_average_response_ms\"\n",
		"code/categraf/inputs/ping/README.md": "# ping plugin\n",
		"code/categraf/.hidden.txt":           "should not list\n",
	}
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &aiagent.ToolDeps{SkillsPath: skillsPath}
}

func TestListCode(t *testing.T) {
	deps := seedCodeCorpus(t)
	bg := context.Background()
	params := map[string]string{}

	out, err := listCode(bg, deps, map[string]interface{}{"repo": "categraf", "path": "inputs/ping"}, params)
	if err != nil {
		t.Fatalf("list_code should succeed: %v", err)
	}
	if !strings.Contains(out, "ping.go") || !strings.Contains(out, "README.md") {
		t.Errorf("listing missing files: %q", out)
	}

	// 点文件不外露
	out, err = listCode(bg, deps, map[string]interface{}{"repo": "categraf"}, params)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, ".hidden.txt") {
		t.Errorf("dotfiles must be hidden: %q", out)
	}

	// 白名单外 repo / 路径穿越拒绝
	if _, err := listCode(bg, deps, map[string]interface{}{"repo": "evil"}, params); err == nil {
		t.Error("unknown repo must be rejected")
	}
	if _, err := listCode(bg, deps, map[string]interface{}{"repo": "n9e", "path": "../categraf"}, params); err == nil {
		t.Error("path traversal must be rejected")
	}
}

func TestSearchCode(t *testing.T) {
	deps := seedCodeCorpus(t)
	bg := context.Background()
	params := map[string]string{}

	// 内容命中：file:line + 语料版本行
	out, err := searchCode(bg, deps, map[string]interface{}{"repo": "categraf", "pattern": "ping_average_response_ms"}, params)
	if err != nil {
		t.Fatalf("search_code should succeed: %v", err)
	}
	if !strings.Contains(out, "inputs/ping/ping.go:3:") {
		t.Errorf("expected line match with file:line, got: %q", out)
	}
	if !strings.Contains(out, "code corpus: n9e@v9.0.0, categraf@v0.4.19") {
		t.Errorf("expected corpus version line, got: %q", out)
	}

	// pattern 命中文件路径：单列 matching files 节
	out, err = searchCode(bg, deps, map[string]interface{}{"repo": "categraf", "pattern": "ping"}, params)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "files whose path contains") || !strings.Contains(out, "inputs/ping/ping.go") {
		t.Errorf("expected path-hit section, got: %q", out)
	}

	// 大小写不敏感 + path 缩小范围：命中路径必须以仓库根为基准（含 models/
	// 前缀），保证可直接投给 read_code，两工具参照系一致
	out, err = searchCode(bg, deps, map[string]interface{}{"repo": "n9e", "pattern": "EMERGENCY", "path": "models"}, params)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "models/alert_rule.go:4:") {
		t.Errorf("path-narrowed hits must be repo-root-relative: %q", out)
	}
	// search→read 接力：命中路径原样喂给 read_code 必须可读
	if _, err := readCode(bg, deps, map[string]interface{}{"repo": "n9e", "path": "models/alert_rule.go", "start_line": float64(4), "end_line": float64(4)}, params); err != nil {
		t.Errorf("search hit path must be directly readable by read_code: %v", err)
	}

	// context_lines 带上下文与 > 标记
	out, err = searchCode(bg, deps, map[string]interface{}{"repo": "n9e", "pattern": "Emergency", "context_lines": float64(1)}, params)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, ":4:> ") || !strings.Contains(out, ":3:  ") {
		t.Errorf("expected context lines with marker, got: %q", out)
	}

	// 无命中
	out, err = searchCode(bg, deps, map[string]interface{}{"repo": "n9e", "pattern": "no_such_thing_xyz"}, params)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no matches") {
		t.Errorf("expected no-matches message, got: %q", out)
	}
}

// 路径命中超过展示上限时必须显式标注截断：fe 语料上 "icon" 这类 pattern 的
// 路径命中轻易超 20，静默截断会让模型把前 20 条当成全部文件。
func TestSearchCodePathHitsTruncationNote(t *testing.T) {
	deps := seedCodeCorpus(t)
	bg := context.Background()
	params := map[string]string{}

	// 25 个路径含 iconpack 的文件，内容刻意不含该词——只走路径命中通道
	root := filepath.Dir(deps.SkillsPath)
	for i := 0; i < 25; i++ {
		p := filepath.Join(root, "code", "fe", "src", "iconpack", fmt.Sprintf("f%02d.ts", i))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("export {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out, err := searchCode(bg, deps, map[string]interface{}{"repo": "fe", "pattern": "iconpack"}, params)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, fmt.Sprintf("(%d of 25 path matches shown", searchCodeMaxPathHits)) ||
		!strings.Contains(out, "narrow with the path argument") {
		t.Errorf("over-limit path hits must carry a truncation note: %q", out)
	}

	// 未超限时不出现截断提示
	out, err = searchCode(bg, deps, map[string]interface{}{"repo": "categraf", "pattern": "ping"}, params)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "path matches shown") {
		t.Errorf("within-limit path hits must not be marked truncated: %q", out)
	}
}

// 超长单行必须被截断：fe 语料里有内联 SVG 常量单行 12k+ 字符，命中它会把
// 数 KB 噪声塞进 LLM 上下文，context_lines 还会成倍放大。两条输出路径
// （无上下文 / 带上下文）都要截，且按 rune 截以免切碎中文注释。
func TestSearchCodeTruncatesLongLines(t *testing.T) {
	deps := seedCodeCorpus(t)
	bg := context.Background()
	params := map[string]string{}

	root := filepath.Dir(deps.SkillsPath)
	longLine := "const svgIcon = \"" + strings.Repeat("中", 4000) + "\" // needle_xyz"
	content := "package icon\n" + longLine + "\nconst after = 1\n"
	p := filepath.Join(root, "code", "fe", "src", "icon.ts")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, ctxLines := range []float64{0, 2} {
		out, err := searchCode(bg, deps, map[string]interface{}{
			"repo": "fe", "pattern": "needle_xyz", "context_lines": ctxLines,
		}, params)
		if err != nil {
			t.Fatalf("context_lines=%v: %v", ctxLines, err)
		}
		if !strings.Contains(out, "...(line truncated)") {
			t.Errorf("context_lines=%v: long line must be marked truncated, got %d bytes", ctxLines, len(out))
		}
		// 截断后整段输出应远小于原始行（12KB+）；留足版本行/路径节的余量
		if len(out) > 4000 {
			t.Errorf("context_lines=%v: output not truncated, got %d bytes", ctxLines, len(out))
		}
		// rune 边界：不得切出损坏的 UTF-8
		if strings.ContainsRune(out, '�') {
			t.Errorf("context_lines=%v: truncation broke UTF-8", ctxLines)
		}
	}

	// 正常长度的行不受影响
	out, err := searchCode(bg, deps, map[string]interface{}{"repo": "fe", "pattern": "const after"}, params)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "const after = 1") || strings.Contains(out, "(line truncated)") {
		t.Errorf("short lines must pass through untouched: %q", out)
	}
}

// 命中条数上限管不住体积：context_lines 把每条命中放大到 (2n+1) 行，100 条
// 命中在真实语料上可达 110KB。累计字节预算必须同时生效，且结尾要给出"还有更
// 多、请收窄"的提示——否则模型把半截输出当成全部结果。
func TestSearchCodeCapsTotalOutput(t *testing.T) {
	deps := seedCodeCorpus(t)
	bg := context.Background()
	params := map[string]string{}

	// 每行 ~400 字符、共 1500 行且行行命中的文件（总量压在 searchCodeMaxFileBytes
	// 以内，否则整文件会被跳过内容搜索）
	root := filepath.Dir(deps.SkillsPath)
	line := "	logger.Infof(\"needle_bulk %s\", " + strings.Repeat("a", 380) + ")\n"
	p := filepath.Join(root, "code", "n9e", "bulk.go")
	if err := os.WriteFile(p, []byte("package bulk\n"+strings.Repeat(line, 1500)), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, ctxLines := range []float64{0, 5} {
		out, err := searchCode(bg, deps, map[string]interface{}{
			"repo": "n9e", "pattern": "needle_bulk", "context_lines": ctxLines,
		}, params)
		if err != nil {
			t.Fatalf("context_lines=%v: %v", ctxLines, err)
		}
		// 预算之上只允许溢出最后一处命中（含 context 行）+ 头部版本行/提示尾注
		if len(out) > searchCodeMaxOutputBytes+16*1024 {
			t.Errorf("context_lines=%v: output %d bytes blew the %d budget", ctxLines, len(out), searchCodeMaxOutputBytes)
		}
		if !strings.Contains(out, "narrow with the path argument") {
			t.Errorf("context_lines=%v: truncated output must tell the model to narrow the search: %q", ctxLines, out[max(0, len(out)-200):])
		}
	}

	// 未触发预算时不应出现截断提示
	out, err := searchCode(bg, deps, map[string]interface{}{"repo": "categraf", "pattern": "ping_average_response_ms"}, params)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "stopped at") {
		t.Errorf("small result must not be marked truncated: %q", out)
	}
}

func TestReadCode(t *testing.T) {
	deps := seedCodeCorpus(t)
	bg := context.Background()
	params := map[string]string{}

	// 全文读取带行号
	out, err := readCode(bg, deps, map[string]interface{}{"repo": "n9e", "path": "models/alert_rule.go"}, params)
	if err != nil {
		t.Fatalf("read_code should succeed: %v", err)
	}
	if !strings.Contains(out, "1\tpackage models") || !strings.Contains(out, "4\tconst Emergency = 1") {
		t.Errorf("expected numbered lines, got: %q", out)
	}

	// 行区间
	out, err = readCode(bg, deps, map[string]interface{}{"repo": "n9e", "path": "models/alert_rule.go", "start_line": float64(4), "end_line": float64(5)}, params)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "package models") || !strings.Contains(out, "4\tconst Emergency = 1") || !strings.Contains(out, "5\tconst Warning = 2") {
		t.Errorf("range read wrong: %q", out)
	}

	// 起始行超过文件总行数
	if _, err := readCode(bg, deps, map[string]interface{}{"repo": "n9e", "path": "models/alert_rule.go", "start_line": float64(100)}, params); err == nil {
		t.Error("out-of-range start_line must error")
	}

	// 路径穿越拒绝
	if _, err := readCode(bg, deps, map[string]interface{}{"repo": "n9e", "path": "../manifest.json"}, params); err == nil {
		t.Error("path traversal must be rejected")
	}
}

// 语料缺失（默认构建未解压）时三工具统一报降级提示，引导 LLM 回文档检索。
func TestCodeToolsCorpusUnavailable(t *testing.T) {
	root := t.TempDir()
	skillsPath := filepath.Join(root, "skill")
	if err := os.MkdirAll(skillsPath, 0o755); err != nil {
		t.Fatal(err)
	}
	deps := &aiagent.ToolDeps{SkillsPath: skillsPath}
	bg := context.Background()
	params := map[string]string{}

	for name, fn := range map[string]func() (string, error){
		"list_code": func() (string, error) { return listCode(bg, deps, map[string]interface{}{"repo": "n9e"}, params) },
		"search_code": func() (string, error) {
			return searchCode(bg, deps, map[string]interface{}{"repo": "n9e", "pattern": "x"}, params)
		},
		"read_code": func() (string, error) {
			return readCode(bg, deps, map[string]interface{}{"repo": "n9e", "path": "TREE.md"}, params)
		},
	} {
		_, err := fn()
		if err == nil || !strings.Contains(err.Error(), "code corpus not available") {
			t.Errorf("%s should report corpus unavailable, got: %v", name, err)
		}
	}

	// code/ 目录存在但无 .corpus_hash 完成标记（用户自建/残缺）：同样不可用，
	// 否则残缺语料的"没搜到"会被 LLM 误读成"该标识符不存在"。
	if err := os.MkdirAll(filepath.Join(root, "code", "n9e"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := searchCode(bg, deps, map[string]interface{}{"repo": "n9e", "pattern": "x"}, params)
	if err == nil || !strings.Contains(err.Error(), "code corpus not available") {
		t.Errorf("unmarked code dir must be unavailable, got: %v", err)
	}
}
