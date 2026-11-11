package models_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// 线上真实数据：告警规则 p_01 存了 source_labels=["3"]（数字开头，在 prometheus
// LegacyValidation 下是非法 label name），导致 center 把它解坏成 [""] 发给 edge，
// edge 反序列化整批规则失败并退出进程。
const dirtyRelabelRuleConfig = `{"queries":[{"query":"up","ref":"A"}],"event_relabel_config":[{"action":"replace","replacement":"2","separator":";","source_labels":["3"],"target_label":"1"}]}`

func TestAlertRuleVerify_EventRelabelConfig(t *testing.T) {
	tests := []struct {
		name       string
		ruleConfig string
		wantErr    bool
	}{
		{"no rule config", "", false},
		{"no relabel config", `{"queries":[{"prom_ql":"up","severity":2}]}`, false},
		{"null relabel config", `{"event_relabel_config":null}`, false},
		{"empty relabel config", `{"event_relabel_config":[]}`, false},
		{"valid relabel config", `{"event_relabel_config":[{"source_labels":["ident","service"],"target_label":"host","action":"replace"}]}`, false},
		{"empty source label", `{"event_relabel_config":[{"source_labels":[""],"action":"replace"}]}`, true},
		{"source label starting with digit", dirtyRelabelRuleConfig, true},
		{"source label with dash", `{"event_relabel_config":[{"source_labels":["a-b"],"action":"replace"}]}`, true},
		{"empty target label is allowed", `{"event_relabel_config":[{"source_labels":["ident"],"target_label":"","action":"replace"}]}`, false},
		// target_label 在读取端是普通 string，不参与反序列化校验，运行期
		// lowercase/uppercase/hashmod 等分支也会原样写出（含点号的标签正是 REPLACE_DOT
		// 机制要支持的场景）。校验口径必须与读取端对齐，不能把这些存量配置拒之门外。
		{"dotted target label is allowed", `{"event_relabel_config":[{"source_labels":["ident"],"target_label":"k8s.pod","action":"lowercase"}]}`, false},
		{"non-ascii target label is allowed", `{"event_relabel_config":[{"source_labels":["ident"],"target_label":"主机名","action":"lowercase"}]}`, false},
		{"weird target label is allowed", `{"event_relabel_config":[{"source_labels":["ident"],"target_label":"1x!","action":"replace"}]}`, false},
		// 字段类型不合法同样会让读取端整段丢弃 relabel 配置，必须一起拦住，
		// 否则表现为"保存成功但 relabel 永远不生效"
		{"modulus of wrong type", `{"event_relabel_config":[{"source_labels":["ident"],"modulus":"3","action":"hashmod"}]}`, true},
		{"source_labels of wrong type", `{"event_relabel_config":[{"source_labels":"ident","action":"replace"}]}`, true},
		{"relabel config not an array", `{"event_relabel_config":{"source_labels":["ident"]}}`, true},
		// rule_config 的整体结构由各 cate 自己定义，不是本校验的职责
		{"rule config not an object", `"whatever"`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := &models.AlertRule{Name: "testrule", RuleConfig: tt.ruleConfig}
			if tt.ruleConfig == "" {
				// Verify 里有独立的 rule_config 非空校验，这里只关心 relabel 部分
				ar.RuleConfig = "{}"
			}

			err := ar.Verify()
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

// DB2FE 遇到解不动的 event_relabel_config 时必须整段丢弃，而不是把 json 包
// 半途中止后留下的残缺结构体传下去 —— 那份数据会被 center 原样序列化给 edge。
func TestAlertRuleDB2FE_DropsUndecodableEventRelabelConfig(t *testing.T) {
	ar := &models.AlertRule{Id: 1099, Name: "p_01", RuleConfig: dirtyRelabelRuleConfig}
	if err := ar.DB2FE(); err != nil {
		t.Fatalf("DB2FE err: %v", err)
	}

	if len(ar.EventRelabelConfig) != 0 {
		b, _ := json.Marshal(ar.EventRelabelConfig)
		t.Fatalf("expected event_relabel_config to be dropped, got: %s", b)
	}

	// center 序列化后发给 edge 的内容必须能被 edge 原样解回来
	b, err := json.Marshal(ar)
	if err != nil {
		t.Fatalf("marshal err: %v", err)
	}
	var decoded models.AlertRule
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("edge side failed to decode what center serialized: %v", err)
	}
}

func TestAlertRuleDB2FE_KeepsValidEventRelabelConfig(t *testing.T) {
	ar := &models.AlertRule{
		Id:         1,
		Name:       "ok",
		RuleConfig: `{"event_relabel_config":[{"source_labels":["ident"],"target_label":"host","action":"replace"}]}`,
	}
	if err := ar.DB2FE(); err != nil {
		t.Fatalf("DB2FE err: %v", err)
	}

	if len(ar.EventRelabelConfig) != 1 {
		t.Fatalf("expected 1 relabel config, got %d", len(ar.EventRelabelConfig))
	}
	if got := ar.EventRelabelConfig[0].TargetLabel; got != "host" {
		t.Fatalf("target_label = %q, want %q", got, "host")
	}
	if got := len(ar.EventRelabelConfig[0].SourceLabels); got != 1 {
		t.Fatalf("len(source_labels) = %d, want 1", got)
	}
	if got := string(ar.EventRelabelConfig[0].SourceLabels[0]); got != "ident" {
		t.Fatalf("source_labels[0] = %q, want %q", got, "ident")
	}
}

// 边缘机房同步告警规则时，单条规则的脏数据不能让整批规则都同步不下来 ——
// 这正是线上 edge 起不来的直接原因：center 返回的 271 条规则里有 1 条
// event_relabel_config 解不动，整个响应被判为解码失败，edge 随即 exit(1)。
func TestAlertRuleGetsAll_EdgeSkipsUndecodableRule(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"dat":[
			{"id":1,"name":"good-before"},
			{"id":1099,"name":"p_01","event_relabel_config":[{"source_labels":[""],"action":"replace"}]},
			{"id":2,"name":"good-after"}
		],"err":""}`)
	}))
	defer srv.Close()

	edgeCtx := ctx.NewContext(context.Background(), nil, false, conf.CenterApi{Addrs: []string{srv.URL}})

	lst, err := models.AlertRuleGetsAll(edgeCtx)
	if err != nil {
		t.Fatalf("one dirty rule must not fail the whole sync, got err: %v", err)
	}

	if len(lst) != 2 {
		t.Fatalf("expected 2 usable rules, got %d", len(lst))
	}
	for _, want := range []int64{1, 2} {
		found := false
		for _, ar := range lst {
			if ar.Id == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("rule %d missing from result", want)
		}
	}
}

// 一条都解不出来时必须报错：调用方（memsto.AlertRuleCacheType.syncAlertRules）
// 只有拿到 error 才会保住旧缓存并继续重试；返回空列表会让规则缓存被清空、
// 同步水位被刷成最新，边缘机房告警全停且不再重试。
func TestAlertRuleGetsAll_EdgeFailsWhenAllRulesUndecodable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"dat":[
			{"id":1,"name":"bad-1","event_relabel_config":[{"source_labels":[""],"action":"replace"}]},
			{"id":2,"name":"bad-2","event_relabel_config":[{"source_labels":["3"],"action":"replace"}]}
		],"err":""}`)
	}))
	defer srv.Close()

	edgeCtx := ctx.NewContext(context.Background(), nil, false, conf.CenterApi{Addrs: []string{srv.URL}})

	lst, err := models.AlertRuleGetsAll(edgeCtx)
	if err == nil {
		t.Fatalf("expected error when every rule fails to decode, got %d rules", len(lst))
	}
}

// 空 rule_config 是老库里的存量数据，不是异常：DB2FE 不应把它当成解码失败去处理。
func TestAlertRuleDB2FE_EmptyRuleConfig(t *testing.T) {
	ar := &models.AlertRule{Id: 1, Name: "legacy", RuleConfig: ""}
	if err := ar.DB2FE(); err != nil {
		t.Fatalf("DB2FE err: %v", err)
	}

	if len(ar.EventRelabelConfig) != 0 {
		t.Fatalf("expected empty event_relabel_config, got %v", ar.EventRelabelConfig)
	}
}
