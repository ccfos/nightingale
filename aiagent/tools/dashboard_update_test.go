package tools

import (
	"encoding/json"
	"testing"
)

func ptrStr(s string) *string { return &s }
func ptrBool(b bool) *bool    { return &b }

// sampleConfigs returns a small but representative board payload: one query
// variable, two data panels (one nested inside a collapsed row), with units
// and targets — enough to exercise summary, lint, and patch paths.
func sampleConfigs(t *testing.T) map[string]interface{} {
	t.Helper()
	const raw = `{
		"version":"3.4.0",
		"var":[
			{"name":"prom","type":"datasource","definition":"prometheus"},
			{"name":"ident","type":"query","label":"主机","multi":true,"definition":"label_values(cpu_usage_idle, ident)","datasource":{"cate":"prometheus","value":"${prom}"}}
		],
		"panels":[
			{"id":"panel-1","type":"timeseries","name":"CPU使用率","datasourceValue":"${prom}",
			 "options":{"standardOptions":{"util":"percent"}},
			 "targets":[{"refId":"A","expr":"cpu_usage_active{ident=~\"$ident\"}","legendFormat":"{{ident}}"}]},
			{"id":"row-1","type":"row","name":"内存","panels":[
				{"id":"panel-2","type":"timeseries","name":"内存使用率","datasourceValue":1,
				 "targets":[{"refId":"A","expr":"mem_used_percent{ident=~\"$host\"}","legendFormat":"{{ident}}"}]}
			]}
		]
	}`
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal sample: %v", err)
	}
	return cfg
}

func TestSummarizeConfigs(t *testing.T) {
	cfg := sampleConfigs(t)
	vars, panels := summarizeConfigs(cfg)

	if len(vars) != 2 {
		t.Fatalf("vars = %d, want 2", len(vars))
	}
	if vars[1].Name != "ident" || vars[1].Definition != "label_values(cpu_usage_idle, ident)" {
		t.Fatalf("ident var summary wrong: %#v", vars[1])
	}
	if vars[1].DatasourceValue != "${prom}" {
		t.Fatalf("ident datasource_value = %v, want ${prom}", vars[1].DatasourceValue)
	}

	// row + 2 data panels are flattened in → 3 entries.
	if len(panels) != 3 {
		t.Fatalf("panels = %d, want 3 (incl nested + row)", len(panels))
	}
	var cpu *panelSummary
	for i := range panels {
		if panels[i].Id == "panel-1" {
			cpu = &panels[i]
		}
	}
	if cpu == nil {
		t.Fatal("panel-1 not in summary")
	}
	if cpu.Unit != "percent" {
		t.Fatalf("panel-1 unit = %q, want percent", cpu.Unit)
	}
	if len(cpu.Queries) != 1 || cpu.Queries[0].PromQL != `cpu_usage_active{ident=~"$ident"}` {
		t.Fatalf("panel-1 query summary wrong: %#v", cpu.Queries)
	}
	// nested panel-2 must be present
	found := false
	for _, p := range panels {
		if p.Id == "panel-2" {
			found = true
		}
	}
	if !found {
		t.Fatal("nested panel-2 missing from summary")
	}
}

func TestLintVariables_DetectsUndefinedRef(t *testing.T) {
	cfg := sampleConfigs(t)
	issues := lintVariables(cfg)
	// panel-2 references $host which is not defined.
	if len(issues) == 0 {
		t.Fatal("expected lint to flag undefined $host reference")
	}
	hit := false
	for _, s := range issues {
		if contains(s, "host") {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("lint issues did not mention $host: %v", issues)
	}
}

func TestLintVariables_CleanBoard(t *testing.T) {
	cfg := sampleConfigs(t)
	// Define host so the previously dangling ref resolves.
	vars := cfg["var"].([]interface{})
	vars = append(vars, map[string]interface{}{"name": "host", "type": "query"})
	cfg["var"] = vars
	if issues := lintVariables(cfg); len(issues) != 0 {
		t.Fatalf("expected no issues, got %v", issues)
	}
}

func TestApplyVariablePatches_Update(t *testing.T) {
	cfg := sampleConfigs(t)
	changes, err := applyVariablePatches("", cfg, []variablePatch{
		{Name: "ident", DefaultValue: ptrStr("web01"), Multi: ptrBool(false)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("changes = %v", changes)
	}
	vars := cfg["var"].([]interface{})
	ident := vars[1].(map[string]interface{})
	if ident["defaultValue"] != "web01" {
		t.Fatalf("defaultValue = %v, want web01", ident["defaultValue"])
	}
	if ident["multi"] != false {
		t.Fatalf("multi = %v, want false", ident["multi"])
	}
}

func TestApplyVariablePatches_AddAndDelete(t *testing.T) {
	cfg := sampleConfigs(t)

	// add
	if _, err := applyVariablePatches("", cfg, []variablePatch{
		{Name: "interface", Definition: ptrStr("label_values(net_bytes_recv, interface)"), Label: ptrStr("网卡")},
	}); err != nil {
		t.Fatal(err)
	}
	if len(cfg["var"].([]interface{})) != 3 {
		t.Fatalf("after add, vars = %d, want 3", len(cfg["var"].([]interface{})))
	}

	// delete
	if _, err := applyVariablePatches("", cfg, []variablePatch{{Name: "interface", Delete: true}}); err != nil {
		t.Fatal(err)
	}
	if len(cfg["var"].([]interface{})) != 2 {
		t.Fatalf("after delete, vars = %d, want 2", len(cfg["var"].([]interface{})))
	}

	// delete missing → error
	if _, err := applyVariablePatches("", cfg, []variablePatch{{Name: "nope", Delete: true}}); err == nil {
		t.Fatal("expected error deleting missing variable")
	}
}

func TestApplyVariablePatches_RequiresName(t *testing.T) {
	cfg := sampleConfigs(t)
	if _, err := applyVariablePatches("", cfg, []variablePatch{{Definition: ptrStr("x")}}); err == nil {
		t.Fatal("expected error for empty name")
	}
}

// TestApplyVariablePatches_AddRejectsNonQueryType：buildVariable 只会构造 query
// 型骨架，新增时指定其他 type 只会改写 type 字段、产出字段布局错乱的畸形变量
// （如 type=datasource 却自带 datasource 子对象），必须拒绝。
func TestApplyVariablePatches_AddRejectsNonQueryType(t *testing.T) {
	cfg := sampleConfigs(t)
	for _, typ := range []string{"custom", "textbox", "datasource"} {
		if _, err := applyVariablePatches("", cfg, []variablePatch{
			{Name: "env", Type: ptrStr(typ)},
		}); err == nil {
			t.Fatalf("expected error adding variable with type %q", typ)
		}
	}
	// 显式 type=query 等价于缺省，照常新增
	if _, err := applyVariablePatches("", cfg, []variablePatch{
		{Name: "env", Definition: ptrStr("label_values(up, env)"), Type: ptrStr("query")},
	}); err != nil {
		t.Fatal(err)
	}
	vars := cfg["var"].([]interface{})
	added := vars[len(vars)-1].(map[string]interface{})
	if added["type"] != "query" {
		t.Fatalf("added variable type = %v, want query", added["type"])
	}
}

// TestApplyVariablePatches_AddRequiresDefinition：name 没匹配上现有变量且不带
// definition 的 patch，多半是名字手误 + 真正想改的字段被宽容解码丢弃；必须报错，
// 不能静默新增一个 definition 为空的废变量再宣称"新增变量"成功（与更新分支的
// 全 nil 守卫同源）。
func TestApplyVariablePatches_AddRequiresDefinition(t *testing.T) {
	cfg := sampleConfigs(t)
	before := len(cfg["var"].([]interface{}))

	// 全 nil patch（如 {"name":"identt","color":"red"}，color 被解码器丢弃）
	if _, err := applyVariablePatches("", cfg, []variablePatch{{Name: "identt"}}); err == nil {
		t.Fatal("expected error adding variable without definition")
	}
	// 带了其他字段但没 definition，同样拒绝
	if _, err := applyVariablePatches("", cfg, []variablePatch{{Name: "identt", Label: ptrStr("机器")}}); err == nil {
		t.Fatal("expected error adding variable with label but no definition")
	}
	if got := len(cfg["var"].([]interface{})); got != before {
		t.Fatalf("vars changed on rejected patch: %d -> %d", before, got)
	}
}

// TestQuerySpec_TolerantDecode is the regression for the strict-decode abort:
// the LLM quotes scalars ("step":"15", "instant":"true") or writes ints as
// floats ("step":15.0); none must fail the decode (which would abort the whole
// panels parse with "invalid JSON").
func TestQuerySpec_TolerantDecode(t *testing.T) {
	const raw = `[
		{"ref":"A","promql":"x","step":15.0,"instant":"true","hide":"false"},
		{"ref":"B","promql":"y","step":"30","delete":"true"}
	]`
	var qs []QuerySpec
	if err := json.Unmarshal([]byte(raw), &qs); err != nil {
		t.Fatalf("tolerant decode must accept float/string scalars: %v", err)
	}
	if len(qs) != 2 {
		t.Fatalf("len = %d, want 2", len(qs))
	}
	if qs[0].Step == nil || *qs[0].Step != 15 {
		t.Fatalf("step from 15.0 = %v, want 15", qs[0].Step)
	}
	if qs[0].Instant == nil || *qs[0].Instant != true {
		t.Fatalf("instant from \"true\" = %v, want true", qs[0].Instant)
	}
	if qs[0].Hide == nil || *qs[0].Hide != false {
		t.Fatalf("hide from \"false\" = %v, want false", qs[0].Hide)
	}
	if qs[1].Step == nil || *qs[1].Step != 30 {
		t.Fatalf("step from \"30\" = %v, want 30", qs[1].Step)
	}
	if !qs[1].Delete {
		t.Fatalf("delete from \"true\" = %v, want true", qs[1].Delete)
	}
}

// TestQuerySpec_TolerantDecode_RejectsGarbage confirms genuinely invalid scalars
// still error (we tolerate string/float forms, not arbitrary junk).
func TestQuerySpec_TolerantDecode_RejectsGarbage(t *testing.T) {
	if err := json.Unmarshal([]byte(`[{"step":"fifteen"}]`), &[]QuerySpec{}); err == nil {
		t.Fatal("a non-numeric step string must still error")
	}
}

// TestVariablePatch_TolerantDecode covers the same tolerance on the `variables`
// arg: a string-form multi/delete must not abort the parse.
func TestVariablePatch_TolerantDecode(t *testing.T) {
	const raw = `[{"name":"ident","multi":"true","delete":"false"}]`
	var vp []variablePatch
	if err := json.Unmarshal([]byte(raw), &vp); err != nil {
		t.Fatalf("tolerant decode: %v", err)
	}
	if vp[0].Multi == nil || *vp[0].Multi != true {
		t.Fatalf("multi from \"true\" = %v, want true", vp[0].Multi)
	}
	if vp[0].Delete {
		t.Fatalf("delete from \"false\" = %v, want false", vp[0].Delete)
	}
}

// TestApplyVariablePatches_AddFollowsBoardDatasourceVar is the regression for
// the hard-coded ${prom} bug: a new query variable must reference the board's
// ACTUAL datasource variable. Most boards name it "datasource" (not "prom"), so
// a hard-coded ${prom} would dangle and the variable would silently resolve no
// values.
func TestApplyVariablePatches_AddFollowsBoardDatasourceVar(t *testing.T) {
	const raw = `{"var":[{"name":"datasource","type":"datasource","definition":"prometheus"}]}`
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, err := applyVariablePatches("", cfg, []variablePatch{
		{Name: "ident", Definition: ptrStr("label_values(up, ident)")},
	}); err != nil {
		t.Fatal(err)
	}
	vars := cfg["var"].([]interface{})
	nv := vars[len(vars)-1].(map[string]interface{}) // new var appended last
	ds, ok := nv["datasource"].(map[string]interface{})
	if !ok {
		t.Fatalf("new var missing datasource block: %#v", nv)
	}
	if ds["value"] != "${datasource}" {
		t.Fatalf("new var datasource = %v, want ${datasource} (board's actual var, not hard-coded ${prom})", ds["value"])
	}
}

// TestBoardDatasourceVarRef_FallsBackToProm covers the no-datasource-var board:
// with nothing to reference, the builder default ${prom} is the safe fallback.
func TestBoardDatasourceVarRef_FallsBackToProm(t *testing.T) {
	cfg := map[string]interface{}{"var": []interface{}{
		map[string]interface{}{"name": "ident", "type": "query"},
	}}
	if got := boardDatasourceVarRef(cfg); got != "${prom}" {
		t.Fatalf("boardDatasourceVarRef with no datasource var = %q, want ${prom}", got)
	}
}

// TestSummarizeConfigs_SurfacesStepHideInstant is the regression for the dropped
// fields: the "before" snapshot must expose instant (incl. explicit false), step,
// and hide so the diff the user approves is accurate for the fields being edited.
// Curves that set none of them must omit all three (nil), not show defaults.
func TestSummarizeConfigs_SurfacesStepHideInstant(t *testing.T) {
	const raw = `{"panels":[{"id":"p1","type":"timeseries","name":"x",
		"targets":[
			{"refId":"A","expr":"a","instant":false,"step":30,"hide":true},
			{"refId":"B","expr":"b"}
		]}]}`
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	_, panels := summarizeConfigs(cfg)
	if len(panels) != 1 || len(panels[0].Queries) != 2 {
		t.Fatalf("unexpected summary shape: %#v", panels)
	}

	a := panels[0].Queries[0]
	if a.Instant == nil || *a.Instant != false {
		t.Fatalf("curve A instant = %v, want explicit false (must be visible in the before-diff)", a.Instant)
	}
	if a.Step == nil || *a.Step != 30 {
		t.Fatalf("curve A step = %v, want 30", a.Step)
	}
	if a.Hide == nil || *a.Hide != true {
		t.Fatalf("curve A hide = %v, want true", a.Hide)
	}

	b := panels[0].Queries[1]
	if b.Instant != nil || b.Step != nil || b.Hide != nil {
		t.Fatalf("curve B set none of instant/step/hide → all must be nil (omitted), got %#v", b)
	}
}

func TestApplyPanelPatches_ReplaceQueriesAndUnit(t *testing.T) {
	cfg := sampleConfigs(t)
	newPromQL := `avg(cpu_usage_active{cpu="cpu-total",ident=~"$ident"})`
	// Edit panel-1's single existing curve by ref (refId "A"); a no-ref spec
	// would instead add a second curve under the new no-positional semantics.
	queries := []QuerySpec{{Ref: "A", PromQL: newPromQL, Legend: "{{ident}}"}}
	changes, err := applyPanelPatches("", cfg, []panelPatch{
		{ID: "panel-1", Unit: ptrStr("none"), NewName: ptrStr("CPU使用率(总)"), Queries: &queries},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("changes = %v", changes)
	}
	p := findPanel(cfg["panels"].([]interface{}), "panel-1", "")
	if p["name"] != "CPU使用率(总)" {
		t.Fatalf("name = %v", p["name"])
	}
	if panelUnit(p) != "none" {
		t.Fatalf("unit = %q, want none", panelUnit(p))
	}
	targets := p["targets"].([]interface{})
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1", len(targets))
	}
	if targets[0].(map[string]interface{})["expr"] != newPromQL {
		t.Fatalf("expr not replaced: %v", targets[0])
	}
}

func TestApplyPanelPatches_NestedAndAddCurve(t *testing.T) {
	cfg := sampleConfigs(t)
	// panel-2 is nested inside a collapsed row and already has one curve (refId
	// "A"). Adding a no-ref curve must append it, leaving the original intact.
	queries := []QuerySpec{
		{PromQL: "swap_used_percent{ident=~\"$ident\"}", Legend: "swap"},
	}
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "panel-2", Queries: &queries}}); err != nil {
		t.Fatal(err)
	}
	p := findPanel(cfg["panels"].([]interface{}), "panel-2", "")
	if p == nil {
		t.Fatal("nested panel-2 not found")
	}
	targets := p["targets"].([]interface{})
	if got := len(targets); got != 2 {
		t.Fatalf("targets = %d, want 2 (original + added)", got)
	}
	orig := targets[0].(map[string]interface{})
	if orig["refId"] != "A" || orig["expr"] != "mem_used_percent{ident=~\"$host\"}" {
		t.Fatalf("original curve must be preserved when adding, got: %#v", orig)
	}
}

func TestApplyPanelPatches_DeleteNested(t *testing.T) {
	cfg := sampleConfigs(t)
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "panel-2", Delete: true}}); err != nil {
		t.Fatal(err)
	}
	if findPanel(cfg["panels"].([]interface{}), "panel-2", "") != nil {
		t.Fatal("panel-2 should be deleted")
	}
	// the row container stays
	if findPanel(cfg["panels"].([]interface{}), "row-1", "") == nil {
		t.Fatal("row-1 should still exist")
	}
}

// TestDeletePanel_OnlyDeletesOneAcrossLevels locks the guarantee that a single
// name-based delete removes at most one panel, even when the same name appears
// both at the top level and inside a nested row. Without the !deleted guard on
// the recursion, deleting "dup" would silently drop both.
func TestDeletePanel_OnlyDeletesOneAcrossLevels(t *testing.T) {
	panels := []interface{}{
		map[string]interface{}{"id": "p1", "name": "dup"},
		map[string]interface{}{
			"id":   "row-1",
			"name": "row",
			"panels": []interface{}{
				map[string]interface{}{"id": "p2", "name": "dup"},
			},
		},
	}
	out, deleted := deletePanel(panels, "", "dup")
	if !deleted {
		t.Fatal("expected a panel to be deleted")
	}
	// Top-level "dup" removed; the nested same-named panel must survive.
	if findPanel(out, "p1", "") != nil {
		t.Fatal("top-level p1 should be deleted")
	}
	if findPanel(out, "p2", "") == nil {
		t.Fatal("nested p2 (same name) must NOT be deleted in the same call")
	}
}

// dupNamePanels builds a board where the same panel name ("CPU") appears twice
// — once at the top level and once inside a collapsed row — with distinct ids.
func dupNamePanels(t *testing.T) map[string]interface{} {
	t.Helper()
	const raw = `{
		"panels":[
			{"id":"panel-1","type":"timeseries","name":"CPU","datasourceCate":"prometheus",
			 "targets":[{"refId":"A","expr":"a"}]},
			{"id":"row-1","type":"row","name":"row","panels":[
				{"id":"panel-2","type":"timeseries","name":"CPU","datasourceCate":"prometheus",
				 "targets":[{"refId":"A","expr":"b"}]}
			]}
		]
	}`
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return cfg
}

// TestApplyPanelPatches_AmbiguousNameRejected locks the fix for the silent
// wrong-panel mutation: a name-based update that matches >1 panel must error
// (asking for the id) instead of editing the first match.
func TestApplyPanelPatches_AmbiguousNameRejected(t *testing.T) {
	cfg := dupNamePanels(t)
	_, err := applyPanelPatches("", cfg, []panelPatch{{Name: "CPU", Unit: ptrStr("percent")}})
	if err == nil {
		t.Fatal("expected ambiguity error updating a duplicate panel name")
	}
	if !contains(err.Error(), "ambiguous") {
		t.Fatalf("error should mention ambiguity, got: %v", err)
	}
	// Neither panel should have been touched (unit not set).
	if panelUnit(findPanel(cfg["panels"].([]interface{}), "panel-1", "")) != "" {
		t.Fatal("panel-1 must not be mutated on an ambiguous patch")
	}
	if panelUnit(findPanel(cfg["panels"].([]interface{}), "panel-2", "")) != "" {
		t.Fatal("panel-2 must not be mutated on an ambiguous patch")
	}
}

// TestApplyPanelPatches_AmbiguousNameDeleteRejected is the delete-side of the
// same guard: a duplicate-name delete must error rather than dropping the first
// match.
func TestApplyPanelPatches_AmbiguousNameDeleteRejected(t *testing.T) {
	cfg := dupNamePanels(t)
	_, err := applyPanelPatches("", cfg, []panelPatch{{Name: "CPU", Delete: true}})
	if err == nil {
		t.Fatal("expected ambiguity error deleting a duplicate panel name")
	}
	if !contains(err.Error(), "ambiguous") {
		t.Fatalf("error should mention ambiguity, got: %v", err)
	}
	// Both panels survive.
	if findPanel(cfg["panels"].([]interface{}), "panel-1", "") == nil {
		t.Fatal("panel-1 must survive an ambiguous delete")
	}
	if findPanel(cfg["panels"].([]interface{}), "panel-2", "") == nil {
		t.Fatal("panel-2 must survive an ambiguous delete")
	}
}

// TestApplyPanelPatches_DuplicateNameByIdStillWorks shows the escape hatch: with
// duplicate names, addressing the exact panel by id remains unambiguous.
func TestApplyPanelPatches_DuplicateNameByIdStillWorks(t *testing.T) {
	cfg := dupNamePanels(t)
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "panel-2", Unit: ptrStr("percent")}}); err != nil {
		t.Fatalf("id-based update on a duplicate name should succeed: %v", err)
	}
	if panelUnit(findPanel(cfg["panels"].([]interface{}), "panel-2", "")) != "percent" {
		t.Fatal("panel-2 unit should be set via id-based patch")
	}
	// The same-named sibling stays untouched.
	if panelUnit(findPanel(cfg["panels"].([]interface{}), "panel-1", "")) != "" {
		t.Fatal("panel-1 must not be touched when targeting panel-2 by id")
	}
}

// TestApplyPanelPatches_UnknownNameRejected verifies the 0-match branch reports
// not-found (rather than ambiguous).
func TestApplyPanelPatches_UnknownNameRejected(t *testing.T) {
	cfg := dupNamePanels(t)
	_, err := applyPanelPatches("", cfg, []panelPatch{{Name: "nope", Unit: ptrStr("x")}})
	if err == nil {
		t.Fatal("expected not-found error for unknown panel name")
	}
	if contains(err.Error(), "ambiguous") {
		t.Fatalf("unknown name should be not-found, not ambiguous: %v", err)
	}
}

func TestCountPanels(t *testing.T) {
	cfg := dupNamePanels(t)
	panels := cfg["panels"].([]interface{})
	if n := countPanels(panels, "", "CPU"); n != 2 {
		t.Fatalf("countPanels by name = %d, want 2 (incl nested)", n)
	}
	if n := countPanels(panels, "panel-2", ""); n != 1 {
		t.Fatalf("countPanels by id = %d, want 1", n)
	}
	if n := countPanels(panels, "", "missing"); n != 0 {
		t.Fatalf("countPanels missing = %d, want 0", n)
	}
}

func TestApplyPanelPatches_MatchByName(t *testing.T) {
	cfg := sampleConfigs(t)
	if _, err := applyPanelPatches("", cfg, []panelPatch{{Name: "CPU使用率", Unit: ptrStr("percent")}}); err != nil {
		t.Fatal(err)
	}
}

func TestApplyPanelPatches_NotFound(t *testing.T) {
	cfg := sampleConfigs(t)
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "panel-99", Unit: ptrStr("x")}}); err == nil {
		t.Fatal("expected not-found error")
	}
	if _, err := applyPanelPatches("", cfg, []panelPatch{{Unit: ptrStr("x")}}); err == nil {
		t.Fatal("expected error when no locator given")
	}
}

func TestSetPanelUnit_CreatesNesting(t *testing.T) {
	pm := map[string]interface{}{}
	setPanelUnit(pm, "bytesIEC")
	if panelUnit(pm) != "bytesIEC" {
		t.Fatalf("unit = %q", panelUnit(pm))
	}
}

// TestSetPanelUnit_WritesUnitKey locks the write to standardOptions.unit — the
// key the current FE renderer/editor (panel schema >= 3.3.0) actually reads. The
// legacy "util" key is only migrated to "unit" for panels older than 3.3.0, so
// writing "util" on a current panel would silently do nothing. We also strip any
// stale "util" so a pre-3.3.0 panel's load-time migration can't overwrite it.
func TestSetPanelUnit_WritesUnitKey(t *testing.T) {
	pm := map[string]interface{}{
		"options": map[string]interface{}{
			"standardOptions": map[string]interface{}{"util": "percent"},
		},
	}
	setPanelUnit(pm, "bytesIEC")
	so := pm["options"].(map[string]interface{})["standardOptions"].(map[string]interface{})
	if so["unit"] != "bytesIEC" {
		t.Fatalf("standardOptions.unit = %v, want bytesIEC (FE reads unit, not util)", so["unit"])
	}
	if _, ok := so["util"]; ok {
		t.Fatalf("legacy standardOptions.util should be stripped, got %v", so["util"])
	}
}

// TestPanelUnit_RealExportedPanel guards the read side against the unit key
// drifting. This payload is copied verbatim from a real exported dashboard
// (integrations/SpringBoot/.../JVM(Actuator)...json), which uses the legacy
// "util" key; panelUnit must still read it via the util fallback so the "before"
// value in the diff is correct for older boards.
func TestPanelUnit_RealExportedPanel(t *testing.T) {
	const raw = `{
		"id":"c325f6ba-bca2-42f1-a518-1d3077b54a54",
		"type":"stat",
		"name":"Start time",
		"options":{
			"standardOptions":{"util":"datetimeMilliseconds"},
			"thresholds":{"steps":[{"color":"#634CD9","type":"base","value":null}]}
		}
	}`
	var pm map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &pm); err != nil {
		t.Fatalf("unmarshal real panel: %v", err)
	}
	if got := panelUnit(pm); got != "datetimeMilliseconds" {
		t.Fatalf("panelUnit on real exported panel = %q, want datetimeMilliseconds (legacy util fallback)", got)
	}
}

// TestPanelUnit_PrefersUnitOverUtil verifies that when a current panel carries
// the canonical "unit" key, panelUnit returns it rather than a stale "util".
func TestPanelUnit_PrefersUnitOverUtil(t *testing.T) {
	pm := map[string]interface{}{
		"options": map[string]interface{}{
			"standardOptions": map[string]interface{}{
				"unit": "bytesIEC",
				"util": "percent",
			},
		},
	}
	if got := panelUnit(pm); got != "bytesIEC" {
		t.Fatalf("panelUnit = %q, want bytesIEC (unit must win over legacy util)", got)
	}
}

func TestPrometheusLikeDashboard(t *testing.T) {
	// sampleConfigs is a Prometheus board (datasource var definition=prometheus,
	// panels datasourceCate unset → defaults to Prometheus).
	if cate, ok := prometheusLikeDashboard(sampleConfigs(t)); !ok {
		t.Fatalf("sample board should be Prometheus-like, got cate=%q", cate)
	}

	// A board whose datasource variable points at MySQL must be rejected.
	mysqlVar := sampleConfigs(t)
	vars := mysqlVar["var"].([]interface{})
	vars[0].(map[string]interface{})["definition"] = "mysql"
	if cate, ok := prometheusLikeDashboard(mysqlVar); ok {
		t.Fatalf("MySQL-var board should not be Prometheus-like (cate=%q)", cate)
	}

	// A board with a ClickHouse panel (nested in a row) must be rejected too.
	ckPanel := sampleConfigs(t)
	panels := ckPanel["panels"].([]interface{})
	row := panels[1].(map[string]interface{})
	row["panels"].([]interface{})[0].(map[string]interface{})["datasourceCate"] = "ck"
	if cate, ok := prometheusLikeDashboard(ckPanel); ok || cate != "ck" {
		t.Fatalf("ClickHouse-panel board should be rejected with cate=ck, got cate=%q ok=%v", cate, ok)
	}
}

// TestTargetLegend covers both the canonical "legend" key and the historical
// "legendFormat" fallback that imported/Grafana boards may carry.
func TestTargetLegend(t *testing.T) {
	cases := []struct {
		name string
		tm   map[string]interface{}
		want string
	}{
		{"canonical legend", map[string]interface{}{"legend": "{{ident}}"}, "{{ident}}"},
		{"historical legendFormat", map[string]interface{}{"legendFormat": "{{host}}"}, "{{host}}"},
		{"legend wins over legendFormat", map[string]interface{}{"legend": "a", "legendFormat": "b"}, "a"},
		{"neither", map[string]interface{}{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := targetLegend(c.tm); got != c.want {
				t.Fatalf("targetLegend = %q, want %q", got, c.want)
			}
		})
	}
}

// TestSummarizeConfigs_LegacyLegendFormat ensures the read-side summary surfaces
// a legend stored under the historical "legendFormat" key. The sample panels use
// legendFormat, so the summary must still report it.
func TestSummarizeConfigs_LegacyLegendFormat(t *testing.T) {
	cfg := sampleConfigs(t)
	_, panels := summarizeConfigs(cfg)
	var cpu *panelSummary
	for i := range panels {
		if panels[i].Id == "panel-1" {
			cpu = &panels[i]
		}
	}
	if cpu == nil || len(cpu.Queries) != 1 {
		t.Fatalf("panel-1 query summary missing: %#v", panels)
	}
	if cpu.Queries[0].Legend != "{{ident}}" {
		t.Fatalf("legend = %q, want {{ident}} (legendFormat fallback)", cpu.Queries[0].Legend)
	}
}

// TestApplyPanelPatches_RejectsQueriesOnNonPromPanel guards the query-edit
// guard: replacing queries on a SQL/non-Prometheus panel must error rather than
// silently overwriting its target schema with expr/refId/legend.
func TestApplyPanelPatches_RejectsQueriesOnNonPromPanel(t *testing.T) {
	cfg := sampleConfigs(t)
	// Make panel-1 a MySQL panel.
	p := findPanel(cfg["panels"].([]interface{}), "panel-1", "")
	p["datasourceCate"] = "mysql"

	queries := []QuerySpec{{PromQL: "select 1"}}
	_, err := applyPanelPatches("", cfg, []panelPatch{{ID: "panel-1", Queries: &queries}})
	if err == nil {
		t.Fatal("expected error replacing queries on a MySQL panel")
	}

	// A non-query edit (e.g. unit) on the same panel must still succeed.
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "panel-1", Unit: ptrStr("none")}}); err != nil {
		t.Fatalf("non-query edit on non-prom panel should succeed: %v", err)
	}
}

// TestApplyPanelPatches_TypeChangeGuards covers the two destructive paths
// checkPanelTypeChange must refuse: converting a non-chart source (a text
// panel's markdown lives in custom.content and changePanelType would wipe it
// with no way back), and converting a non-Prometheus panel (the prom-flavored
// custom/options defaults would strand its render config — same stance as the
// queries-edit guard).
func TestApplyPanelPatches_TypeChangeGuards(t *testing.T) {
	// Text source panel: refused, content untouched.
	cfg := sampleConfigs(t)
	p := findPanel(cfg["panels"].([]interface{}), "panel-1", "")
	p["type"] = "text"
	p["custom"] = map[string]interface{}{"content": "# runbook"}
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "panel-1", Type: ptrStr("timeseries")}}); err == nil {
		t.Fatal("expected error converting a text panel to a chart type")
	}
	if got := p["custom"].(map[string]interface{})["content"]; got != "# runbook" {
		t.Fatalf("text content must survive the refused conversion, got %v", got)
	}

	// Non-Prometheus panel: refused, mirroring the queries-edit guard.
	cfg = sampleConfigs(t)
	p = findPanel(cfg["panels"].([]interface{}), "panel-1", "")
	p["datasourceCate"] = "mysql"
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "panel-1", Type: ptrStr("stat")}}); err == nil {
		t.Fatal("expected error changing type of a MySQL panel")
	}

	// A Prometheus chart panel still converts normally.
	cfg = sampleConfigs(t)
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "panel-1", Type: ptrStr("stat")}}); err != nil {
		t.Fatalf("type change on a prom panel should succeed: %v", err)
	}
	p = findPanel(cfg["panels"].([]interface{}), "panel-1", "")
	if p["type"] != "stat" {
		t.Fatalf("panel type = %v, want stat", p["type"])
	}
}

// TestApplyPanelPatches_TypeChangeClearsInstant is the regression for the
// broken-chart bug: the FE picks instant-vs-range purely by target.instant
// (Renderer/datasource/prometheus.ts), so a stat/table panel queried with
// instant:true and converted to timeseries would render as a single dot
// instead of a curve unless the flag is cleared. Conversions in the other
// direction keep targets untouched — range targets render fine on every type.
func TestApplyPanelPatches_TypeChangeClearsInstant(t *testing.T) {
	const raw = `{
		"panels":[
			{"id":"panel-1","type":"stat","name":"CPU","datasourceCate":"prometheus",
			 "targets":[
				{"refId":"A","expr":"cpu_usage_active","instant":true},
				{"refId":"B","expr":"mem_used_percent","instant":true}
			 ]}
		]
	}`
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "panel-1", Type: ptrStr("timeseries")}}); err != nil {
		t.Fatalf("stat→timeseries should succeed: %v", err)
	}
	p := findPanel(cfg["panels"].([]interface{}), "panel-1", "")
	for _, tgt := range p["targets"].([]interface{}) {
		tm := tgt.(map[string]interface{})
		if _, has := tm["instant"]; has {
			t.Fatalf("instant must be cleared on conversion to timeseries, target %v still carries it", tm["refId"])
		}
	}

	// timeseries→stat keeps targets untouched (no instant forced on).
	cfg2 := sampleConfigs(t)
	if _, err := applyPanelPatches("", cfg2, []panelPatch{{ID: "panel-1", Type: ptrStr("stat")}}); err != nil {
		t.Fatalf("timeseries→stat should succeed: %v", err)
	}
	p2 := findPanel(cfg2["panels"].([]interface{}), "panel-1", "")
	for _, tgt := range p2["targets"].([]interface{}) {
		if _, has := tgt.(map[string]interface{})["instant"]; has {
			t.Fatal("conversion away from timeseries must not invent an instant flag")
		}
	}
}

// TestApplyPanelPatches_TypeChangeWithQueriesClearsInstant is the regression for
// the merge-order bug: when a single patch converts a stat panel to timeseries
// AND carries queries (which the model routinely echoes back with the panel's
// existing instant:true), changePanelType cleared instant but the following
// mergeTargets re-introduced it — so the new timeseries rendered as a single dot.
// The clear must run AFTER the query merge.
func TestApplyPanelPatches_TypeChangeWithQueriesClearsInstant(t *testing.T) {
	const raw = `{
		"panels":[
			{"id":"panel-1","type":"stat","name":"CPU","datasourceCate":"prometheus",
			 "targets":[{"refId":"A","expr":"cpu_usage_active","instant":true}]}
		]
	}`
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	// The model reads the stat panel and patches type→timeseries while echoing
	// the curve back with its current instant:true.
	queries := []QuerySpec{{Ref: "A", PromQL: "cpu_usage_active", Instant: ptrBool(true)}}
	if _, err := applyPanelPatches("", cfg, []panelPatch{
		{ID: "panel-1", Type: ptrStr("timeseries"), Queries: &queries},
	}); err != nil {
		t.Fatalf("stat→timeseries with queries should succeed: %v", err)
	}
	p := findPanel(cfg["panels"].([]interface{}), "panel-1", "")
	if p["type"] != "timeseries" {
		t.Fatalf("type = %v, want timeseries", p["type"])
	}
	for _, tgt := range p["targets"].([]interface{}) {
		tm := tgt.(map[string]interface{})
		if _, has := tm["instant"]; has {
			t.Fatalf("instant must be cleared even when queries ride in the same patch, target %v still carries it", tm["refId"])
		}
	}
}

// TestApplyPanelPatches_TypelessPanelTimeseriesIsNoOp is the regression for the
// effective-type bug: a type-less panel (the FE renders it as timeseries) set to
// type:timeseries is a no-op and must NOT run a full changePanelType, which would
// wipe the panel's authored custom render config.
func TestApplyPanelPatches_TypelessPanelTimeseriesIsNoOp(t *testing.T) {
	const raw = `{
		"panels":[
			{"id":"panel-1","name":"CPU","datasourceCate":"prometheus",
			 "custom":{"drawStyle":"bars","lineWidth":5},
			 "targets":[{"refId":"A","expr":"cpu_usage_active"}]}
		]
	}`
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	// type:timeseries on an (effectively-timeseries) type-less panel is a no-op.
	_, err := applyPanelPatches("", cfg, []panelPatch{{ID: "panel-1", Type: ptrStr("timeseries")}})
	if err == nil {
		t.Fatal("expected a no-op error restating a type-less panel's effective type")
	}
	p := findPanel(cfg["panels"].([]interface{}), "panel-1", "")
	custom := p["custom"].(map[string]interface{})
	if custom["drawStyle"] != "bars" || custom["lineWidth"] != float64(5) {
		t.Fatalf("authored custom config must survive the no-op, got %#v", custom)
	}
}

func ptrInt(i int) *int { return &i }

// TestApplyPanelPatches_PreservesTargetFields is the regression for the
// clobbering bug: editing a panel's queries must keep the fields the QuerySpec
// doesn't mention — refId, step, hide, __mode__, and any other persisted keys —
// so expression queries, fixed steps, hidden curves, and refId-keyed
// overrides/transformations survive the edit.
func TestApplyPanelPatches_PreservesTargetFields(t *testing.T) {
	const raw = `{
		"panels":[
			{"id":"panel-1","type":"timeseries","name":"CPU","datasourceCate":"prometheus",
			 "overrides":[{"matcher":{"id":"byFrameRefID","options":"B"},"properties":[]}],
			 "targets":[
				{"refId":"A","expr":"old_expr_a","step":30,"hide":true,"__mode__":"__expr__","time":{"from":"now-1h"}},
				{"refId":"B","expr":"old_expr_b","legend":"keepme"}
			 ]}
		]
	}`
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Edit only target B's expr (match by ref); leave A untouched by sending it
	// through unchanged with its ref. Add a brand-new curve with no ref.
	queries := []QuerySpec{
		{Ref: "A", PromQL: "old_expr_a"},       // expr unchanged, other fields must survive
		{Ref: "B", PromQL: "new_expr_b"},       // only expr changes
		{PromQL: "brand_new", Legend: "third"}, // new target → fresh refId
	}
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "panel-1", Queries: &queries}}); err != nil {
		t.Fatal(err)
	}

	p := findPanel(cfg["panels"].([]interface{}), "panel-1", "")
	targets := p["targets"].([]interface{})
	if len(targets) != 3 {
		t.Fatalf("targets = %d, want 3", len(targets))
	}

	a := targets[0].(map[string]interface{})
	if a["refId"] != "A" {
		t.Fatalf("target A refId = %v, want A", a["refId"])
	}
	// step is a JSON number → float64 after round-trip.
	if a["step"] != float64(30) {
		t.Fatalf("target A step = %v, want 30 (must be preserved)", a["step"])
	}
	if a["hide"] != true {
		t.Fatalf("target A hide = %v, want true (must be preserved)", a["hide"])
	}
	if a["__mode__"] != "__expr__" {
		t.Fatalf("target A __mode__ = %v, want __expr__ (must be preserved)", a["__mode__"])
	}
	if _, ok := a["time"].(map[string]interface{}); !ok {
		t.Fatalf("target A time range dropped: %#v", a["time"])
	}

	b := targets[1].(map[string]interface{})
	if b["refId"] != "B" {
		t.Fatalf("target B refId = %v, want B (refId-keyed override would break otherwise)", b["refId"])
	}
	if b["expr"] != "new_expr_b" {
		t.Fatalf("target B expr = %v, want new_expr_b", b["expr"])
	}
	if b["legend"] != "keepme" {
		t.Fatalf("target B legend = %v, want keepme (unchanged field preserved)", b["legend"])
	}

	c := targets[2].(map[string]interface{})
	if c["refId"] != "C" {
		t.Fatalf("new target refId = %v, want C (next free letter)", c["refId"])
	}
	if c["expr"] != "brand_new" || c["legend"] != "third" {
		t.Fatalf("new target wrong: %#v", c)
	}
}

// TestApplyPanelPatches_SetsStepAndHide verifies the new QuerySpec pointer
// fields actually write through, and that an omitted (nil) pointer leaves the
// existing value alone.
func TestApplyPanelPatches_SetsStepAndHide(t *testing.T) {
	const raw = `{"panels":[{"id":"p","type":"timeseries","name":"x","datasourceCate":"prometheus",
		"targets":[{"refId":"A","expr":"e","step":15,"hide":false}]}]}`
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	queries := []QuerySpec{{Ref: "A", PromQL: "e", Hide: ptrBool(true)}} // flip hide, leave step
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "p", Queries: &queries}}); err != nil {
		t.Fatal(err)
	}
	a := findPanel(cfg["panels"].([]interface{}), "p", "")["targets"].([]interface{})[0].(map[string]interface{})
	if a["hide"] != true {
		t.Fatalf("hide = %v, want true (explicit set)", a["hide"])
	}
	if a["step"] != float64(15) {
		t.Fatalf("step = %v, want 15 (nil pointer keeps existing)", a["step"])
	}
}

// TestApplyPanelPatches_InstantTrueToFalse is the regression for the bug where
// QuerySpec.Instant was a value-typed bool: a curve already carrying instant=true
// could never be switched back to range query, because applyQuerySpec only wrote
// instant when true. With Instant as *bool, an explicit false must write through;
// an omitted (nil) Instant must leave the existing value alone.
func TestApplyPanelPatches_InstantTrueToFalse(t *testing.T) {
	const raw = `{"panels":[{"id":"p","type":"stat","name":"x","datasourceCate":"prometheus",
		"targets":[
			{"refId":"A","expr":"a","instant":true},
			{"refId":"B","expr":"b","instant":true}
		]}]}`
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Flip A back to range query (instant=false); leave B's instant untouched (nil).
	queries := []QuerySpec{
		{Ref: "A", Instant: ptrBool(false)},
		{Ref: "B", PromQL: "b"},
	}
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "p", Queries: &queries}}); err != nil {
		t.Fatal(err)
	}

	targets := findPanel(cfg["panels"].([]interface{}), "p", "")["targets"].([]interface{})
	a := targets[0].(map[string]interface{})
	if a["instant"] != false {
		t.Fatalf("target A instant = %v, want false (explicit false must write through)", a["instant"])
	}
	b := targets[1].(map[string]interface{})
	if b["instant"] != true {
		t.Fatalf("target B instant = %v, want true (nil Instant must leave existing value)", b["instant"])
	}
}

// TestApplyPanelPatches_IncrementalMergeKeepsUnmentionedCurves locks down the
// incremental-merge semantics: editing ONE curve of a two-curve panel by ref
// must leave the other curve in place (no silent data loss), and an explicit
// delete=true is the only thing that removes a curve.
func TestApplyPanelPatches_IncrementalMergeKeepsUnmentionedCurves(t *testing.T) {
	const raw = `{
		"panels":[
			{"id":"panel-1","type":"timeseries","name":"CPU","datasourceCate":"prometheus",
			 "targets":[
				{"refId":"A","expr":"old_a","legend":"a"},
				{"refId":"B","expr":"old_b","legend":"b"}
			 ]}
		]
	}`
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Edit only curve A; do NOT mention B. B must survive untouched.
	queries := []QuerySpec{{Ref: "A", PromQL: "new_a"}}
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "panel-1", Queries: &queries}}); err != nil {
		t.Fatal(err)
	}
	targets := findPanel(cfg["panels"].([]interface{}), "panel-1", "")["targets"].([]interface{})
	if len(targets) != 2 {
		t.Fatalf("targets = %d, want 2 (curve B must NOT be dropped when editing only A)", len(targets))
	}
	a := targets[0].(map[string]interface{})
	if a["refId"] != "A" || a["expr"] != "new_a" {
		t.Fatalf("curve A wrong after edit: %#v", a)
	}
	b := targets[1].(map[string]interface{})
	if b["refId"] != "B" || b["expr"] != "old_b" || b["legend"] != "b" {
		t.Fatalf("curve B must be preserved verbatim, got: %#v", b)
	}

	// Now explicitly delete B; only A should remain.
	del := []QuerySpec{{Ref: "B", Delete: true}}
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "panel-1", Queries: &del}}); err != nil {
		t.Fatal(err)
	}
	targets = findPanel(cfg["panels"].([]interface{}), "panel-1", "")["targets"].([]interface{})
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1 after deleting B", len(targets))
	}
	if targets[0].(map[string]interface{})["refId"] != "A" {
		t.Fatalf("remaining curve = %#v, want refId A", targets[0])
	}
}

// TestMergeTargets_NextRefIdSkipsTaken ensures new curves don't collide with
// existing refIds even when the existing ones aren't a contiguous A.. prefix.
func TestMergeTargets_NextRefIdSkipsTaken(t *testing.T) {
	existing := []interface{}{
		map[string]interface{}{"refId": "A", "expr": "a"},
		map[string]interface{}{"refId": "C", "expr": "c"},
	}
	out := mergeTargets(existing, []QuerySpec{
		{Ref: "A", PromQL: "a"},
		{Ref: "C", PromQL: "c"},
		{PromQL: "new"},
	})
	got := out[2].(map[string]interface{})["refId"]
	if got != "B" {
		t.Fatalf("new refId = %v, want B (first free letter)", got)
	}
}

// TestApplyPanelPatches_NoRefCurveIsAddedNotOverwritten is the regression for the
// positional-fallback data-loss bug: a panel with 2 curves, patched with ONE
// no-ref curve, must end up with 3 curves — the two originals untouched and the
// new one appended. The old positional fallback would have matched the no-ref
// spec to index 0 and silently rewritten the first curve's expr.
func TestApplyPanelPatches_NoRefCurveIsAddedNotOverwritten(t *testing.T) {
	const raw = `{
		"panels":[
			{"id":"mem","type":"timeseries","name":"内存","datasourceCate":"prometheus",
			 "targets":[
				{"refId":"A","expr":"mem_used_percent","legend":"used"},
				{"refId":"B","expr":"mem_buffered_percent","legend":"buffered"}
			 ]}
		]
	}`
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// "给内存图表加一条 swap 曲线": a single no-ref new curve.
	queries := []QuerySpec{{PromQL: "swap_used_percent", Legend: "swap"}}
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "mem", Queries: &queries}}); err != nil {
		t.Fatal(err)
	}

	targets := findPanel(cfg["panels"].([]interface{}), "mem", "")["targets"].([]interface{})
	if len(targets) != 3 {
		t.Fatalf("targets = %d, want 3 (2 originals + 1 added)", len(targets))
	}
	a := targets[0].(map[string]interface{})
	if a["refId"] != "A" || a["expr"] != "mem_used_percent" || a["legend"] != "used" {
		t.Fatalf("curve A must be untouched, got: %#v", a)
	}
	b := targets[1].(map[string]interface{})
	if b["refId"] != "B" || b["expr"] != "mem_buffered_percent" || b["legend"] != "buffered" {
		t.Fatalf("curve B must be untouched, got: %#v", b)
	}
	c := targets[2].(map[string]interface{})
	if c["expr"] != "swap_used_percent" || c["legend"] != "swap" {
		t.Fatalf("added curve wrong: %#v", c)
	}
	if c["refId"] != "C" {
		t.Fatalf("added curve refId = %v, want C (next free letter)", c["refId"])
	}
}

// TestApplyQuerySpec_FieldsOnlyPatchKeepsExpr is the regression for the
// unconditional expr-write bug: a fields-only patch (e.g. only hide) that omits
// promql must NOT blank the curve's existing query, since PromQL "" can't be
// distinguished from "not provided".
func TestApplyPanelPatches_FieldsOnlyPatchKeepsExpr(t *testing.T) {
	const raw = `{"panels":[{"id":"p","type":"timeseries","name":"x","datasourceCate":"prometheus",
		"targets":[{"refId":"A","expr":"node_load1","legend":"load"}]}]}`
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Hide curve A without resending its promql.
	queries := []QuerySpec{{Ref: "A", Hide: ptrBool(true)}}
	if _, err := applyPanelPatches("", cfg, []panelPatch{{ID: "p", Queries: &queries}}); err != nil {
		t.Fatal(err)
	}

	a := findPanel(cfg["panels"].([]interface{}), "p", "")["targets"].([]interface{})[0].(map[string]interface{})
	if a["expr"] != "node_load1" {
		t.Fatalf("expr = %v, want node_load1 (must be preserved when promql omitted)", a["expr"])
	}
	if a["hide"] != true {
		t.Fatalf("hide = %v, want true", a["hide"])
	}
	if a["legend"] != "load" {
		t.Fatalf("legend = %v, want load (preserved)", a["legend"])
	}
}

// TestLintVariables_FlagsUndefinedRefInCanonicalLegend is the regression for
// lintPanelRefs reading only "legendFormat": real boards store the legend under
// the canonical "legend" key, so an undefined $var there must still be flagged.
func TestLintVariables_FlagsUndefinedRefInCanonicalLegend(t *testing.T) {
	cfg := map[string]interface{}{
		"var": []interface{}{
			map[string]interface{}{"name": "ident", "type": "query"},
		},
		"panels": []interface{}{
			map[string]interface{}{
				"name": "CPU",
				"targets": []interface{}{
					// expr is clean; the undefined ref lives only in canonical "legend".
					map[string]interface{}{"refId": "A", "expr": "up{ident=~\"$ident\"}", "legend": "{{$missingvar}}"},
				},
			},
		},
	}
	issues := lintVariables(cfg)
	hit := false
	for _, s := range issues {
		if contains(s, "missingvar") && contains(s, "legend") {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("lint should flag undefined $missingvar in canonical legend key, got: %v", issues)
	}
}

// contains is a tiny substring helper for assertions (avoids importing strings
// just for tests already linked against the package).
func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
