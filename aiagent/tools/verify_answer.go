package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/toolkits/pkg/logger"
	"gopkg.in/yaml.v3"
)

// 设计取舍 (重要):
//
// 这是 M3 landmine guard 的"按 skill+tool 收敛"重写版。原方案在 router 落地后扫描,
// 命中强制 retry+annotate, 100% 确定性兜底。这一版改成 LLM 在工具循环里**主动调**,
// 由 SKILL.md 规定流程: 草稿 → verify_answer → 命中就用 search_n9e_docs 重搜 → 重写。
//
// 取舍:
//   - ✅ 域知识 (具体哪些字符串是错的) 完全封闭在 n9e-doc-qa skill 目录的 landmines.yaml
//   - ✅ router / chat / aiagent 通用层零 n9e 字面量, 高内聚低耦合
//   - ❌ 失去确定性 — LLM 没调本工具就直接 Final Answer 的话, 编造的字段名会直接到用户面前
//
// 用户已确认接受 ❌ 这一项的损失, 换 ✅ 的清晰边界。

func init() {
	register(defs.VerifyAnswer, verifyAnswer)
}

// landmineRule 是 landmines.yaml 里每条规则的内存表示。
// yaml -> 解析时编译 regex, 编译失败的规则会被 skip 并打 warning。
type landmineRule struct {
	Name        string `yaml:"name"`
	Pattern     string `yaml:"pattern"`
	Severity    string `yaml:"severity"` // "high" / "medium" / "low"
	Annotate    string `yaml:"annotate"`
	RetryHint   string `yaml:"retry_hint,omitempty"`
	compiledRgx *regexp.Regexp
}

const (
	severityHigh   = "high"
	severityMedium = "medium"
	severityLow    = "low"
)

// 进程级缓存: 第一次调本 tool 时按 ToolDeps.SkillsPath 加载, 之后不变。
// SkillsPath 改了需要重启进程 — 跟 ExtractBuiltin 的语义一致 (启动期解压, 运行期不动)。
var (
	rulesOnce  sync.Once
	rulesMu    sync.RWMutex
	loadedRule []landmineRule
)

// loadRules 从 <skillsPath>/n9e-doc-qa/landmines.yaml 加载规则。
// 找不到文件 / 文件空 / yaml 解析失败 → 返回空切片 + 打 warning, 不阻塞工具调用
// (返回 "clean: true" 给 LLM, 至少不报错; 等运维补 yaml)。
func loadRules(skillsPath string) []landmineRule {
	rulesOnce.Do(func() {
		if skillsPath == "" {
			logger.Warningf("verify_answer: skillsPath empty, no rules loaded")
			return
		}
		// 跟 SKILL.md 同目录的 landmines.yaml
		path := filepath.Join(skillsPath, "n9e-doc-qa", "landmines.yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			logger.Warningf("verify_answer: read %s failed: %v", path, err)
			return
		}
		var raw []landmineRule
		if err := yaml.Unmarshal(data, &raw); err != nil {
			logger.Warningf("verify_answer: parse %s failed: %v", path, err)
			return
		}
		compiled := make([]landmineRule, 0, len(raw))
		for _, r := range raw {
			rgx, err := regexp.Compile(r.Pattern)
			if err != nil {
				logger.Warningf("verify_answer: rule %q invalid regex %q: %v", r.Name, r.Pattern, err)
				continue
			}
			r.compiledRgx = rgx
			compiled = append(compiled, r)
		}
		rulesMu.Lock()
		loadedRule = compiled
		rulesMu.Unlock()
		logger.Infof("verify_answer: loaded %d landmine rules from %s", len(compiled), path)
	})
	rulesMu.RLock()
	defer rulesMu.RUnlock()
	return loadedRule
}

type verifyHit struct {
	Name      string `json:"name"`
	Severity  string `json:"severity"`
	Matched   string `json:"matched"`
	Annotate  string `json:"annotate,omitempty"`
	RetryHint string `json:"retry_hint,omitempty"`
}

func verifyAnswer(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	answer := strings.TrimSpace(getArgString(args, "answer"))
	if answer == "" {
		return "", fmt.Errorf("answer is required")
	}

	var skillsPath string
	if deps != nil {
		skillsPath = deps.SkillsPath
	}
	rules := loadRules(skillsPath)

	var hits []verifyHit
	hasHigh := false
	for _, r := range rules {
		m := r.compiledRgx.FindString(answer)
		if m == "" {
			continue
		}
		hits = append(hits, verifyHit{
			Name:      r.Name,
			Severity:  r.Severity,
			Matched:   m,
			Annotate:  r.Annotate,
			RetryHint: r.RetryHint,
		})
		if r.Severity == severityHigh {
			hasHigh = true
		}
	}

	payload := map[string]interface{}{
		"clean":       len(hits) == 0,
		"must_revise": hasHigh,
		"hits":        hits,
	}
	if len(hits) == 0 {
		payload["next_action"] = "你可以输出 Final Answer 了"
	} else if hasHigh {
		payload["next_action"] = "至少一条 HIGH 命中, 禁止 Final Answer。请按 hits[*].retry_hint 用 search_n9e_docs 重搜, 重写答案, 再次调 verify_answer 验证, 直到 clean=true 或所有命中都是 medium/low 才能 Final Answer。"
	} else {
		payload["next_action"] = "命中均为 medium/low, 可以 Final Answer; 但建议尽量按 hits[*].annotate 调整一下。"
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal result: %v", err)
	}
	return string(out), nil
}
