// integrations-i18n 内置模板 i18n 词条工具：
//
//	extract  扫描 integrations/ 各组件展示字段中的中文，与 i18n/en_US.json 词条表
//	         diff，输出缺译清单（供翻译流水线消费）
//	check    按 loader 同一遍历渲染 en_US 变体后全量扫描中文字符，作为 CI 门禁；
//	         同时校验「README.md 含中文 ⇒ 必须有 README.en_US.md」、
//	         「告警文件名（=cate，语言无关筛选键）不含中文」
//
// 用法：
//
//	go run ./cmd/integrations-i18n extract [-dir integrations] [-out <目录>]
//	go run ./cmd/integrations-i18n check   [-dir integrations] [-scope p0|all] [-v]
//
// 分类口径按「字段路径」判定（与运行时翻译白名单同源）：
// p0 = 告警 name/note/annotations（含 key）/文件名 + 仪表盘 name/note/tags + README；
// p1 = 仪表盘 configs 白名单字段；other = 白名单之外仍含中文的字段——运行时永远
// 不会翻译它，补词条也消不掉，只能源头清洗或写进豁免。
// 豁免：integrations/i18n_exemptions.json，
// {"paths": ["字段路径后缀", ...], "strings": ["整条原文", ...]}
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/ccfos/nightingale/v6/center/integration"

	"github.com/toolkits/pkg/file"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: integrations-i18n <extract|check> [flags]")
		os.Exit(2)
	}
	action := os.Args[1]

	fs := flag.NewFlagSet(action, flag.ExitOnError)
	dir := fs.String("dir", "integrations", "integrations directory")
	out := fs.String("out", "", "extract: write per-component missing-key files to this directory (default stdout summary only)")
	scope := fs.String("scope", "all", "check: which categories fail the gate: p0 | all")
	verbose := fs.Bool("v", false, "check: print every finding instead of per-component counts")
	fs.Parse(os.Args[2:])

	components, err := file.DirsUnder(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read integrations dir fail: %v\n", err)
		os.Exit(2)
	}
	sort.Strings(components)

	exempted := loadExemptions(*dir)

	switch action {
	case "extract":
		runExtract(*dir, components, *out, exempted)
	case "check":
		os.Exit(runCheck(*dir, components, *scope, *verbose, exempted))
	default:
		fmt.Fprintf(os.Stderr, "unknown action %q\n", action)
		os.Exit(2)
	}
}

// exemptions 允许在 en_US 渲染结果里保留中文的清单。两种粒度：
//   - paths：字段路径后缀，适合「这个字段天生是功能性的」（SQL/promql/列引用），
//     不会因为同一个词也出现在展示字段而误放行
//   - strings：整条原文，适合个别一次性内容；粒度粗，新增须谨慎
type exemptions struct {
	strings map[string]struct{}
	paths   []string
}

func (e exemptions) allow(path []string, text string) bool {
	if _, ok := e.strings[text]; ok {
		return true
	}
	joined := strings.Join(path, ".")
	for _, suffix := range e.paths {
		if joined == suffix || strings.HasSuffix(joined, "."+suffix) {
			return true
		}
	}
	return false
}

func loadExemptions(dir string) exemptions {
	res := exemptions{strings: make(map[string]struct{})}
	fp := filepath.Join(dir, "i18n_exemptions.json")
	bs, err := os.ReadFile(fp)
	if err != nil {
		return res
	}
	var v struct {
		Strings []string `json:"strings"`
		Paths   []string `json:"paths"`
	}
	if err := json.Unmarshal(bs, &v); err != nil {
		fmt.Fprintf(os.Stderr, "parse %s fail: %v\n", fp, err)
		os.Exit(2)
	}
	for _, s := range v.Strings {
		res.strings[s] = struct{}{}
	}
	res.paths = v.Paths
	return res
}

func containsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func decodeUseNumber(bs []byte, v interface{}) error {
	dec := json.NewDecoder(bytes.NewReader(bs))
	dec.UseNumber()
	return dec.Decode(v)
}

// componentStrings 单个组件按类别收集到的「含中文的展示字段原文」集合
type componentStrings struct {
	p0 map[string]struct{} // 告警 name/note/annotations + 仪表盘 name/note
	p1 map[string]struct{} // 仪表盘 configs 白名单字段
}

func newComponentStrings() *componentStrings {
	return &componentStrings{p0: map[string]struct{}{}, p1: map[string]struct{}{}}
}

// collectComponent 按 loader 同一遍历口径收集组件内全部可翻译的中文原文
func collectComponent(componentDir string) (*componentStrings, error) {
	cs := newComponentStrings()

	collectInto := func(set map[string]struct{}) func(string) string {
		return func(s string) string {
			if containsHan(s) {
				set[s] = struct{}{}
			}
			return s
		}
	}

	alertFiles, _ := file.FilesUnder(componentDir + "/alerts")
	for _, f := range alertFiles {
		bs, err := os.ReadFile(filepath.Join(componentDir, "alerts", f))
		if err != nil {
			return nil, err
		}
		var rules []map[string]interface{}
		if err := decodeUseNumber(bs, &rules); err != nil {
			return nil, fmt.Errorf("parse %s/alerts/%s: %v", componentDir, f, err)
		}
		// cate（=文件名）是语言无关的筛选键，不走词条翻译，中文文件名由
		// check 门禁直接拦截（改文件名而非补词条），extract 不收集
		for _, rule := range rules {
			integration.VisitAlertDisplayFields(rule, collectInto(cs.p0))
		}
	}

	dashFiles, _ := file.FilesUnder(componentDir + "/dashboards")
	for _, f := range dashFiles {
		bs, err := os.ReadFile(filepath.Join(componentDir, "dashboards", f))
		if err != nil {
			return nil, err
		}
		var dash map[string]interface{}
		if err := decodeUseNumber(bs, &dash); err != nil {
			return nil, fmt.Errorf("parse %s/dashboards/%s: %v", componentDir, f, err)
		}
		if s, ok := dash["name"].(string); ok {
			collectInto(cs.p0)(s)
		}
		if s, ok := dash["note"].(string); ok {
			collectInto(cs.p0)(s)
		}
		if s, ok := dash["tags"].(string); ok {
			collectInto(cs.p0)(s)
		}
		// 根级展示字段摘除后再走白名单遍历，剩下的命中即 configs（p1）
		delete(dash, "name")
		delete(dash, "note")
		delete(dash, "tags")
		integration.VisitDashboardDisplayFields(dash, collectInto(cs.p1))
	}

	return cs, nil
}

func missing(set map[string]struct{}, dict integration.ComponentDict, exempted exemptions) []string {
	var res []string
	for s := range set {
		// 路径级豁免的字段不会被 collectComponent 收进来（它只遍历白名单），
		// 所以提取阶段只需看整条原文豁免
		if _, ok := exempted.strings[s]; ok {
			continue
		}
		if t, ok := dict[s]; !ok || t == "" {
			res = append(res, s)
		}
	}
	sort.Strings(res)
	return res
}

// readmeStatus 返回（README 是否含中文, 是否已有 en_US 副本, en 副本内含中文的行）
func readmeStatus(componentDir string) (bool, bool, []string) {
	files, err := file.FilesUnder(componentDir + "/markdown")
	if err != nil || len(files) == 0 {
		return false, false, nil
	}
	source, variants := integration.PickReadmeFiles(files)
	if source == "" {
		return false, false, nil
	}
	content, err := os.ReadFile(filepath.Join(componentDir, "markdown", source))
	if err != nil {
		return false, false, nil
	}

	enFile, hasEn := variants["en_US"]
	var hanLines []string
	if hasEn {
		enContent, err := os.ReadFile(filepath.Join(componentDir, "markdown", enFile))
		if err == nil {
			for _, line := range strings.Split(string(enContent), "\n") {
				if containsHan(line) {
					hanLines = append(hanLines, line)
				}
			}
		}
	}
	return containsHan(string(content)), hasEn, hanLines
}

func runExtract(dir string, components []string, out string, exempted exemptions) {
	totalP0, totalP1, readmeTodo := 0, 0, 0

	for _, component := range components {
		componentDir := filepath.Join(dir, component)
		cs, err := collectComponent(componentDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(2)
		}
		dict := integration.LoadComponentDicts(componentDir)[integration.LangEnUS]

		p0 := missing(cs.p0, dict, exempted)
		p1 := missing(cs.p1, dict, exempted)
		zhReadme, hasEnReadme, _ := readmeStatus(componentDir)
		needReadme := zhReadme && !hasEnReadme

		if len(p0) == 0 && len(p1) == 0 && !needReadme {
			continue
		}
		totalP0 += len(p0)
		totalP1 += len(p1)
		if needReadme {
			readmeTodo++
		}

		fmt.Printf("%-20s p0 missing: %-4d p1 missing: %-5d readme.en_US.md needed: %v\n",
			component, len(p0), len(p1), needReadme)

		if out != "" {
			writeMissing(out, component, "p0", p0)
			writeMissing(out, component, "p1", p1)
		}
	}

	fmt.Printf("\nTOTAL p0 missing: %d, p1 missing: %d, readme to translate: %d\n", totalP0, totalP1, readmeTodo)
}

func writeMissing(out, component, phase string, keys []string) {
	if len(keys) == 0 {
		return
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %s fail: %v\n", out, err)
		os.Exit(2)
	}
	m := make(map[string]string, len(keys))
	for _, k := range keys {
		m[k] = ""
	}
	bs, _ := json.MarshalIndent(m, "", "    ")
	fp := filepath.Join(out, fmt.Sprintf("%s.%s.missing.json", component, phase))
	if err := os.WriteFile(fp, bs, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s fail: %v\n", fp, err)
		os.Exit(2)
	}
}

// scanHan 全量扫描（含非白名单字段），把命中中文的字段路径（key 链，数组下标
// 不计入）与原文报给 report；这是比词条覆盖率更强的兜底：白名单漏掉的字段也会
// 被抓出来。map 的 key 自身含中文时按所在路径一并上报（annotations 的 key 即
// 展示字段，运行时会翻译）
func scanHan(node interface{}, path []string, report func(path []string, s string)) {
	switch v := node.(type) {
	case map[string]interface{}:
		for k, child := range v {
			// 新建切片而非 append 复用底层数组：report 会持有这个路径
			sub := append(append([]string{}, path...), k)
			if containsHan(k) {
				report(sub, k)
			}
			scanHan(child, sub, report)
		}
	case []interface{}:
		for _, child := range v {
			scanHan(child, path, report)
		}
	case string:
		if containsHan(v) {
			report(path, v)
		}
	}
}

type finding struct {
	component string
	category  string // p0 / p1 / other / readme
	path      string
	text      string
}

// classifyPath 按「字段路径」而非「字符串内容」判定类别，与运行时翻译口径同源：
//   - p0：告警 name/note/annotations（含 key）、仪表盘根级 name/note/tags
//   - p1：仪表盘 configs 内命中展示字段白名单的路径
//   - other：运行时永远不会翻译的路径（功能字段），补词条也消不掉，
//     只能靠源头清洗或写进 i18n_exemptions.json
//
// 早期版本按字符串内容查 p0/p1 集合，同一个串同时出现在展示字段和功能字段时会
// 被误判成可翻译，掩盖真正的 other
func classifyPath(kind string, path []string) string {
	if len(path) == 0 {
		return "other"
	}
	if kind == "alert" {
		switch path[0] {
		case "name", "note":
			if len(path) == 1 {
				return "p0"
			}
		case "annotations":
			return "p0"
		}
		return "other"
	}
	// dashboard
	switch path[0] {
	case "name", "note", "tags":
		if len(path) == 1 {
			return "p0"
		}
		return "other"
	case "configs":
		if integration.IsDisplayFieldPath(path) {
			return "p1"
		}
	}
	return "other"
}

func runCheck(dir string, components []string, scope string, verbose bool, exempted exemptions) int {
	var findings []finding
	// path 为 nil 时只按整条原文豁免（README 行、告警文件名等非字段路径来源）
	add := func(component, category string, path []string, label, text string) {
		if exempted.allow(path, text) {
			return
		}
		findings = append(findings, finding{component, category, label, text})
	}

	for _, component := range components {
		componentDir := filepath.Join(dir, component)
		dict := integration.LoadComponentDicts(componentDir)[integration.LangEnUS]

		// 渲染 en_US 后全量扫描
		checkFile := func(kind, name string, m map[string]interface{}) {
			switch kind {
			case "alert":
				integration.TranslateAlertMap(m, dict)
			case "dashboard":
				integration.TranslateDashboardMap(m, dict)
				// configs 的历史字符串形态：展开成对象再扫，否则整块 JSON 会以
				// 一个 configs 字符串的形式命中，路径信息全丢
				if s, ok := m["configs"].(string); ok {
					var parsed interface{}
					if err := decodeUseNumber([]byte(s), &parsed); err == nil {
						m["configs"] = parsed
					}
				}
			}
			scanHan(m, nil, func(path []string, s string) {
				add(component, classifyPath(kind, path), path, kind+"("+name+")."+strings.Join(path, "."), s)
			})
		}

		alertFiles, _ := file.FilesUnder(componentDir + "/alerts")
		for _, f := range alertFiles {
			bs, _ := os.ReadFile(filepath.Join(componentDir, "alerts", f))
			var rules []map[string]interface{}
			if err := decodeUseNumber(bs, &rules); err != nil {
				fmt.Fprintf(os.Stderr, "parse %s/alerts/%s: %v\n", componentDir, f, err)
				return 2
			}
			// cate（=文件名）是 DB 查询与内存桶共用的 exact-match 筛选键，
			// 必须语言无关：中文文件名要改名，补词条消不掉这条 finding
			if cate := strings.TrimSuffix(f, ".json"); containsHan(cate) {
				add(component, "p0", nil, "alert-cate("+f+")", "alert file name must not contain chinese (cate is a language-agnostic key, rename the file)")
			}
			for _, rule := range rules {
				name, _ := rule["name"].(string)
				checkFile("alert", name, rule)
			}
		}

		dashFiles, _ := file.FilesUnder(componentDir + "/dashboards")
		for _, f := range dashFiles {
			bs, _ := os.ReadFile(filepath.Join(componentDir, "dashboards", f))
			var dash map[string]interface{}
			if err := decodeUseNumber(bs, &dash); err != nil {
				fmt.Fprintf(os.Stderr, "parse %s/dashboards/%s: %v\n", componentDir, f, err)
				return 2
			}
			checkFile("dashboard", f, dash)
		}

		zhReadme, hasEnReadme, hanLines := readmeStatus(componentDir)
		if zhReadme && !hasEnReadme {
			add(component, "readme", nil, "markdown/README.md", "README.en_US.md missing")
		}
		// en 副本自身残留中文也算未完成翻译
		for _, line := range hanLines {
			add(component, "readme", nil, "markdown/README.en_US.md", line)
		}
	}

	counts := map[string]int{}
	byComponent := map[string]map[string]int{}
	for _, f := range findings {
		counts[f.category]++
		if byComponent[f.component] == nil {
			byComponent[f.component] = map[string]int{}
		}
		byComponent[f.component][f.category]++
	}

	if verbose {
		for _, f := range findings {
			fmt.Printf("[%s] %-20s %s: %s\n", f.category, f.component, f.path, f.text)
		}
	} else {
		var comps []string
		for c := range byComponent {
			comps = append(comps, c)
		}
		sort.Strings(comps)
		for _, c := range comps {
			m := byComponent[c]
			fmt.Printf("%-20s p0: %-4d p1: %-5d other: %-3d readme: %d\n", c, m["p0"], m["p1"], m["other"], m["readme"])
		}
	}
	fmt.Printf("\nTOTAL p0: %d, p1: %d, other: %d, readme: %d (scope=%s)\n",
		counts["p0"], counts["p1"], counts["other"], counts["readme"], scope)

	gate := counts["p0"] + counts["readme"]
	if scope == "all" {
		gate += counts["p1"] + counts["other"]
	}
	if gate > 0 {
		fmt.Printf("FAIL: %d finding(s) in scope %q\n", gate, scope)
		return 1
	}
	fmt.Println("OK: no chinese text in rendered en_US builtin templates")
	return 0
}
