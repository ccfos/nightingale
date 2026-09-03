// integrations-alert-action 内置告警规则 action 门禁：
//
// 遵循 Google SRE「每个告警都必须有对应的 action」——内置告警规则模板必须在
// annotations 里带非空的 action（可执行的处置动作），否则 on-call 收到告警也
//不知道该做什么。action 会随内置通知模板的 AnnotationsJSON 遍历进入告警正文，
// 因此它是「能被执行」的前提，而不只是页面上的说明文字。
//
// 存量 620 条规则不可能一次补完，所以采用棘轮（ratchet）基线：
// integrations/alert_action_baseline.json 记录每个组件当前仍缺 action 的规则数，
// 门禁只允许这个数字变小。这样既不阻塞存量，又能保证：
//   - 已梳理完的组件不会退化（缺口为 0 的组件再加无 action 的规则会红）
//   - 新增组件必须一开始就带 action（不在基线里 ⇒ 允许缺口为 0）
//
// 用法：
//
//	go run ./cmd/integrations-alert-action check    [-dir integrations]
//	go run ./cmd/integrations-alert-action update   [-dir integrations]   # 重写基线
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const baselineName = "alert_action_baseline.json"

type baseline struct {
	Comment string         `json:"_comment"`
	Pending map[string]int `json:"pending"`
}

const baselineComment = "每个组件当前仍缺 action annotation 的内置告警规则数（棘轮基线，只允许变小）。" +
	"补完 action 后运行 `go run ./cmd/integrations-alert-action update` 重写本文件。" +
	"不在 pending 里的组件要求缺口为 0——新增组件必须一开始就给每条规则写 action。"

// alertRule 只解析门禁需要的字段
type alertRule struct {
	Name        string            `json:"name"`
	Annotations map[string]string `json:"annotations"`
}

type componentStat struct {
	missing int
	total   int
	// 缺 action 的规则，用于报错时给出具体位置
	offenders []string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: integrations-alert-action <check|update> [flags]")
		os.Exit(2)
	}

	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	dir := fs.String("dir", "integrations", "integrations directory")
	fs.Parse(os.Args[2:])

	stats, err := scan(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan fail: %v\n", err)
		os.Exit(1)
	}

	switch cmd {
	case "check":
		os.Exit(check(*dir, stats))
	case "update":
		if err := update(*dir, stats); err != nil {
			fmt.Fprintf(os.Stderr, "update fail: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		os.Exit(2)
	}
}

func scan(dir string) (map[string]*componentStat, error) {
	stats := make(map[string]*componentStat)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		comp := e.Name()
		alertDir := filepath.Join(dir, comp, "alerts")
		files, err := os.ReadDir(alertDir)
		if err != nil {
			// 没有 alerts 目录是常态，不参与统计
			continue
		}

		st := &componentStat{}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			p := filepath.Join(alertDir, f.Name())
			bs, err := os.ReadFile(p)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", p, err)
			}
			var rules []alertRule
			if err := json.Unmarshal(bs, &rules); err != nil {
				return nil, fmt.Errorf("parse %s: %w", p, err)
			}
			for _, r := range rules {
				st.total++
				if strings.TrimSpace(r.Annotations["action"]) == "" {
					st.missing++
					st.offenders = append(st.offenders,
						fmt.Sprintf("%s/%s :: %s", comp, f.Name(), r.Name))
				}
			}
		}
		if st.total > 0 {
			stats[comp] = st
		}
	}
	return stats, nil
}

func readBaseline(dir string) (*baseline, error) {
	p := filepath.Join(dir, baselineName)
	bs, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return &baseline{Comment: baselineComment, Pending: map[string]int{}}, nil
	}
	if err != nil {
		return nil, err
	}
	b := &baseline{}
	if err := json.Unmarshal(bs, b); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	if b.Pending == nil {
		b.Pending = map[string]int{}
	}
	return b, nil
}

func check(dir string, stats map[string]*componentStat) int {
	b, err := readBaseline(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read baseline fail: %v\n", err)
		return 1
	}

	comps := make([]string, 0, len(stats))
	for c := range stats {
		comps = append(comps, c)
	}
	sort.Strings(comps)

	var regressed, improved []string
	totalRules, totalMissing := 0, 0

	for _, c := range comps {
		st := stats[c]
		totalRules += st.total
		totalMissing += st.missing
		allowed := b.Pending[c] // 不在基线里 ⇒ 0，新组件必须全部带 action

		switch {
		case st.missing > allowed:
			regressed = append(regressed, c)
			fmt.Printf("FAIL %-16s 缺 action %d 条，基线只允许 %d 条\n", c, st.missing, allowed)
			// 超出基线的部分逐条列出来，方便定位
			over := st.missing - allowed
			for i, o := range st.offenders {
				if i >= over {
					break
				}
				fmt.Printf("       ↳ %s\n", o)
			}
		case st.missing < allowed:
			improved = append(improved, c)
			fmt.Printf("STALE %-16s 缺 action 已降到 %d 条（基线 %d），请运行 update 收紧基线\n",
				c, st.missing, allowed)
		}
	}

	// 基线里有、但组件已经没有告警规则了
	for c := range b.Pending {
		if _, ok := stats[c]; !ok {
			fmt.Printf("STALE %-16s 基线里有记录但该组件已无告警规则，请运行 update\n", c)
			improved = append(improved, c)
		}
	}

	fmt.Printf("\n规则总数 %d，带 action %d，缺 action %d\n",
		totalRules, totalRules-totalMissing, totalMissing)

	if len(regressed) > 0 {
		fmt.Printf("\nFAIL: %d 个组件新增了没有 action 的告警规则。\n", len(regressed))
		fmt.Println("每条内置告警规则都必须在 annotations 里写明可执行的处置动作（action）；")
		fmt.Println("写不出 action 的规则不应进模板（宁缺毋滥）。")
		return 1
	}
	if len(improved) > 0 {
		fmt.Printf("\nFAIL: 基线已过期，运行 `go run ./cmd/integrations-alert-action update` 收紧后提交。\n")
		return 1
	}

	fmt.Println("OK: 没有新增缺 action 的内置告警规则")
	return 0
}

func update(dir string, stats map[string]*componentStat) error {
	b := &baseline{Comment: baselineComment, Pending: map[string]int{}}
	for c, st := range stats {
		if st.missing > 0 {
			b.Pending[c] = st.missing
		}
	}

	bs, err := json.MarshalIndent(b, "", "    ")
	if err != nil {
		return err
	}
	bs = append(bs, '\n')

	p := filepath.Join(dir, baselineName)
	if err := os.WriteFile(p, bs, 0644); err != nil {
		return err
	}

	total, missing := 0, 0
	for _, st := range stats {
		total += st.total
		missing += st.missing
	}
	fmt.Printf("已写入 %s：%d 个组件仍有缺口，规则总数 %d，缺 action %d\n",
		p, len(b.Pending), total, missing)
	return nil
}
