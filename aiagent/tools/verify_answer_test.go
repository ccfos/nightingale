package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
)

// 因为 verify_answer 用 sync.Once 缓存 rules, 测试之间需要重置避免污染。
// 暴露一个 reset 入口给测试用 (生产环境不会调)。
func resetVerifyAnswerForTest() {
	rulesMu.Lock()
	defer rulesMu.Unlock()
	rulesOnce = sync.Once{}
	loadedRule = nil
}

// 把测试需要的 landmines.yaml 写到临时 skillsPath 下, 模拟生产部署的目录结构。
func setupTestSkillsPath(t *testing.T, yaml string) string {
	t.Helper()
	root := t.TempDir()
	skillDir := filepath.Join(root, "doc-qa")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "landmines.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func callVerify(t *testing.T, skillsPath, answer string) map[string]interface{} {
	t.Helper()
	resetVerifyAnswerForTest()
	deps := &aiagent.ToolDeps{SkillsPath: skillsPath}
	resp, err := verifyAnswer(context.Background(), deps, map[string]interface{}{"answer": answer}, nil)
	if err != nil {
		t.Fatalf("verifyAnswer err: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return got
}

func TestVerifyAnswer_CleanAnswer(t *testing.T) {
	yaml := `
- name: dummy
  pattern: 'NONEXISTENT_TOKEN_XYZ'
  severity: high
  annotate: ''
`
	root := setupTestSkillsPath(t, yaml)
	got := callVerify(t, root, "完全正确的答案, 不会命中。")
	if got["clean"] != true {
		t.Errorf("expected clean=true, got %v", got)
	}
	if got["must_revise"] != false {
		t.Errorf("expected must_revise=false, got %v", got)
	}
}

func TestVerifyAnswer_HighSeverityHit(t *testing.T) {
	yaml := `
- name: telegraf
  pattern: '\[\[inputs\.[a-z]+\]\]'
  severity: high
  annotate: 'should be [[instances]]'
  retry_hint: 'search categraf config'
`
	root := setupTestSkillsPath(t, yaml)
	got := callVerify(t, root, "在 categraf 配置中使用 [[inputs.mysql]] 即可")
	if got["clean"] != false {
		t.Errorf("expected clean=false, got %v", got)
	}
	if got["must_revise"] != true {
		t.Errorf("expected must_revise=true, got %v", got)
	}
	hits := got["hits"].([]interface{})
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	h := hits[0].(map[string]interface{})
	if h["matched"] != "[[inputs.mysql]]" {
		t.Errorf("matched wrong: %v", h["matched"])
	}
	if h["severity"] != "high" {
		t.Errorf("severity wrong: %v", h["severity"])
	}
	if !strings.Contains(got["next_action"].(string), "禁止 Final Answer") {
		t.Errorf("next_action should warn about Final Answer, got %v", got["next_action"])
	}
}

func TestVerifyAnswer_MediumOnlyAllowsFinalAnswer(t *testing.T) {
	yaml := `
- name: minor
  pattern: 'ping_result_milliseconds'
  severity: medium
  annotate: 'maybe wrong name'
`
	root := setupTestSkillsPath(t, yaml)
	got := callVerify(t, root, "查询 ping_result_milliseconds 即可")
	if got["clean"] != false {
		t.Errorf("expected clean=false (has hit), got %v", got)
	}
	if got["must_revise"] != false {
		t.Errorf("medium-only hit should not require revise, got must_revise=%v", got["must_revise"])
	}
	if !strings.Contains(got["next_action"].(string), "可以 Final Answer") {
		t.Errorf("next_action should allow Final Answer, got %v", got["next_action"])
	}
}

func TestVerifyAnswer_EmptyAnswerErr(t *testing.T) {
	resetVerifyAnswerForTest()
	_, err := verifyAnswer(context.Background(), &aiagent.ToolDeps{}, map[string]interface{}{"answer": ""}, nil)
	if err == nil {
		t.Error("empty answer should return error")
	}
}

func TestVerifyAnswer_MissingYamlFile_GracefulDegrade(t *testing.T) {
	// 没有 landmines.yaml — 不应当 panic 或 error, 应当当作 clean
	resetVerifyAnswerForTest()
	deps := &aiagent.ToolDeps{SkillsPath: t.TempDir()}
	resp, err := verifyAnswer(context.Background(), deps, map[string]interface{}{"answer": "任何内容"}, nil)
	if err != nil {
		t.Fatalf("missing yaml should not error: %v", err)
	}
	var got map[string]interface{}
	_ = json.Unmarshal([]byte(resp), &got)
	if got["clean"] != true {
		t.Errorf("missing yaml should result in clean=true (no rules = nothing to flag), got %v", got)
	}
}

// 真实 landmines.yaml 跑通: 检验仓库下的规则文件确实能覆盖 80 题回归里的翻车 case。
// 这一条比单元测试更接近 production — 用真实规则集打真实翻车样本。
func TestVerifyAnswer_RealLandmineYaml(t *testing.T) {
	wd, _ := os.Getwd()
	// 从 aiagent/tools/ 找仓库根 — 与 integrations_loader_test 同样的策略
	root := wd
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(root, "aiagent", "skill", "embedded", "builtin", "doc-qa", "landmines.yaml")
		if _, err := os.Stat(candidate); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Skip("repo root not found")
		}
		root = parent
	}
	skillsPath := filepath.Join(root, "aiagent", "skill", "embedded", "builtin")

	cases := []struct {
		name      string
		answer    string
		wantMust  bool
		wantHits  bool
	}{
		{"q001_telegraf_syntax", "用 [[inputs.mysql]] 配置 mysql 采集", true, true},
		{"q079_n9e_addr", "通过 N9E_ADDR 环境变量配置", true, true},
		{"q041_severity_critical", "Severity 字段: 1=Critical", true, true},
		{"q040_ping_metric", "查询 ping_result_milliseconds 指标", false, true},
		{"clean_answer", "categraf 用 [[instances]] 配置, Severity 1 是 Emergency", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := callVerify(t, skillsPath, c.answer)
			gotMust, _ := got["must_revise"].(bool)
			gotHits, _ := got["hits"].([]interface{})
			if gotMust != c.wantMust {
				t.Errorf("must_revise=%v, want %v (answer=%q)", gotMust, c.wantMust, c.answer)
			}
			if (len(gotHits) > 0) != c.wantHits {
				t.Errorf("hits=%d, wantHits=%v (answer=%q)", len(gotHits), c.wantHits, c.answer)
			}
		})
	}
}

func TestVerifyAnswer_InvalidRegexSkipped(t *testing.T) {
	yaml := `
- name: bad
  pattern: '[unclosed'
  severity: high
  annotate: 'invalid'
- name: good
  pattern: 'valid_string'
  severity: high
  annotate: 'hit'
`
	root := setupTestSkillsPath(t, yaml)
	got := callVerify(t, root, "this contains valid_string here")
	// bad 规则应跳过, good 规则应命中
	hits := got["hits"].([]interface{})
	if len(hits) != 1 {
		t.Errorf("expected 1 hit from valid rule, got %d", len(hits))
	}
}
