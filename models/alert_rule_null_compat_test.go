package models

import (
	"encoding/json"
	"strings"
	"testing"
)

// alertRuleJSONFields 把 AlertRule 编码成对外 JSON 再解回 map，用来断言字段形状。
func alertRuleJSONFields(t *testing.T, ar *AlertRule) map[string]json.RawMessage {
	t.Helper()
	b, err := json.Marshal(ar)
	if err != nil {
		t.Fatalf("marshal alert rule: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal alert rule json: %v", err)
	}
	return m
}

// 一条"什么都没配"的老规则：serializer:json 列为空、annotations 为空串、rule_config 为空。
// 这些字段以前全是 null，现在数组要是 []，map 要是 {}。
func TestAlertRuleNullCompat_DB2FEEmptyFields(t *testing.T) {
	ar := &AlertRule{Id: 1, Name: "old", Cate: PROMETHEUS}
	if err := ar.DB2FE(); err != nil {
		t.Fatalf("DB2FE: %v", err)
	}

	m := alertRuleJSONFields(t, ar)
	for _, key := range []string{
		"datasource_queries",
		"event_relabel_config",
		"notify_groups_obj",
		"notify_rule_ids",
	} {
		if got := string(m[key]); got != "[]" {
			t.Errorf("%s = %s, want []", key, got)
		}
	}
	if got := string(m["annotations"]); got != "{}" {
		t.Errorf("annotations = %s, want {}", got)
	}
	// pipeline_configs 与 severities 故意保持 null，原因见 DB2FE 注释
	for _, key := range []string{"pipeline_configs", "severities"} {
		if got := string(m[key]); got != "null" {
			t.Errorf("%s = %s, want null", key, got)
		}
	}
}

// DB2FE 不能把已有的值冲掉。
func TestAlertRuleNullCompat_DB2FEKeepsExistingValues(t *testing.T) {
	ar := &AlertRule{
		Id:              2,
		Name:            "configured",
		Cate:            PROMETHEUS,
		Annotations:     `{"k":"v"}`,
		NotifyRuleIds:   []int64{7},
		PipelineConfigs: []PipelineConfig{{PipelineId: 3, Enable: true}},
		DatasourceQueries: []DatasourceQuery{
			{MatchType: 0, Op: "in", Values: []interface{}{float64(1)}},
		},
	}
	if err := ar.DB2FE(); err != nil {
		t.Fatalf("DB2FE: %v", err)
	}

	m := alertRuleJSONFields(t, ar)
	if got := string(m["annotations"]); got != `{"k":"v"}` {
		t.Errorf("annotations = %s", got)
	}
	if got := string(m["notify_rule_ids"]); got != "[7]" {
		t.Errorf("notify_rule_ids = %s", got)
	}
	if got := string(m["pipeline_configs"]); !strings.Contains(got, `"pipeline_id":3`) {
		t.Errorf("pipeline_configs = %s", got)
	}
	if got := string(m["datasource_queries"]); !strings.Contains(got, `"op":"in"`) {
		t.Errorf("datasource_queries = %s", got)
	}
}

func TestAlertRuleNullCompat_NormalizeRuleConfigNulls(t *testing.T) {
	in := `{
		"queries":[{"prom_ql":"up==0","var_enabled":false,
			"var_config":{"param_val":null,"child_var_configs":{"param_val":null,"child_var_configs":null}}}],
		"triggers":null,
		"algo_params":null,
		"inhibit":false
	}`
	var v interface{}
	if err := json.Unmarshal([]byte(in), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	out, err := json.Marshal(normalizeRuleConfigNulls(v))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(out)

	// 白名单里的数组 key：null 归成 []，且递归到子层
	if strings.Contains(s, `"param_val":null`) {
		t.Errorf("param_val still null: %s", s)
	}
	if n := strings.Count(s, `"param_val":[]`); n != 2 {
		t.Errorf("param_val normalized %d times, want 2 (top + child): %s", n, s)
	}
	if !strings.Contains(s, `"triggers":[]`) {
		t.Errorf("triggers not normalized: %s", s)
	}
	// 对象类型的叶子保留 null，表示没有下一层
	if !strings.Contains(s, `"child_var_configs":null`) {
		t.Errorf("child_var_configs leaf should stay null: %s", s)
	}
	// 不在白名单里的 null 不动
	if !strings.Contains(s, `"algo_params":null`) {
		t.Errorf("algo_params should stay null: %s", s)
	}
	// 已有值原样保留
	for _, want := range []string{`"prom_ql":"up==0"`, `"var_enabled":false`, `"inhibit":false`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
}

func TestAlertRuleNullCompat_NormalizeRuleConfigNullsPassThrough(t *testing.T) {
	// 类型化结构体和 nil 原样返回，不 panic
	if _, ok := normalizeRuleConfigNulls(PromRuleConfig{}).(PromRuleConfig); !ok {
		t.Errorf("typed struct should pass through untouched")
	}
	if got := normalizeRuleConfigNulls(nil); got != nil {
		t.Errorf("nil should stay nil, got %v", got)
	}

	// 顶层是数组、白名单 key 藏在数组元素里也能处理
	var v interface{}
	if err := json.Unmarshal([]byte(`[{"joins":null,"on":null,"exp":"$A"}]`), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, _ := json.Marshal(normalizeRuleConfigNulls(v))
	if got := string(out); got != `[{"exp":"$A","joins":[],"on":[]}]` {
		t.Errorf("got %s", got)
	}
}

// v8.x 类型化 marshal 落库的存量形状：读出来 param_val 要是 []，severities 照常解析。
func TestAlertRuleNullCompat_DB2FELegacyRuleConfig(t *testing.T) {
	ar := &AlertRule{
		Id:   3,
		Name: "legacy",
		Cate: PROMETHEUS,
		RuleConfig: `{"queries":[{"prom_ql":"up==0","severity":2,"var_enabled":false,` +
			`"var_config":{"param_val":null,"child_var_configs":null},` +
			`"recover_config":{"judge_type":0,"recover_exp":""},"unit":""}],` +
			`"inhibit":false,"prom_ql":"","severity":0,"algo_params":null}`,
	}
	if err := ar.DB2FE(); err != nil {
		t.Fatalf("DB2FE: %v", err)
	}

	m := alertRuleJSONFields(t, ar)
	rc := string(m["rule_config"])
	if strings.Contains(rc, `"param_val":null`) || !strings.Contains(rc, `"param_val":[]`) {
		t.Errorf("rule_config.param_val not normalized: %s", rc)
	}
	if !strings.Contains(rc, `"child_var_configs":null`) {
		t.Errorf("child_var_configs leaf should stay null: %s", rc)
	}
	if got := string(m["severities"]); got != "[2]" {
		t.Errorf("severities = %s, want [2]", got)
	}
}

// 前端编辑页把老数据里的 null 原样回传：写侧要洗掉，落库不再带 null。
func TestAlertRuleNullCompat_FE2DBNormalizesRuleConfig(t *testing.T) {
	var rc interface{}
	err := json.Unmarshal([]byte(`{"queries":[{"prom_ql":"up==0","severity":2,`+
		`"var_config":{"param_val":null,"child_var_configs":null}}],"triggers":null}`), &rc)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	ar := &AlertRule{Name: "fe", Cate: PROMETHEUS, RuleConfigJson: rc}
	if err := ar.FE2DB(); err != nil {
		t.Fatalf("FE2DB: %v", err)
	}
	if strings.Contains(ar.RuleConfig, `"param_val":null`) || strings.Contains(ar.RuleConfig, `"triggers":null`) {
		t.Errorf("rule_config stored with null arrays: %s", ar.RuleConfig)
	}
	if !strings.Contains(ar.RuleConfig, `"param_val":[]`) || !strings.Contains(ar.RuleConfig, `"triggers":[]`) {
		t.Errorf("rule_config arrays not normalized: %s", ar.RuleConfig)
	}
}

// 老式 prom_ql 入参走类型化结构体：归一函数跳过，结构体自己的 MarshalJSON 兜底仍然生效。
func TestAlertRuleNullCompat_FE2DBLegacyPromQlPath(t *testing.T) {
	ar := &AlertRule{Name: "legacy-api", Cate: PROMETHEUS, PromQl: "up==0", Severity: 2}
	if err := ar.FE2DB(); err != nil {
		t.Fatalf("FE2DB: %v", err)
	}
	if ar.PromQl != "" {
		t.Errorf("prom_ql should be cleared after FE2DB, got %q", ar.PromQl)
	}
	if !strings.Contains(ar.RuleConfig, `"prom_ql":"up==0"`) || !strings.Contains(ar.RuleConfig, `"param_val":[]`) {
		t.Errorf("unexpected rule_config: %s", ar.RuleConfig)
	}
}
