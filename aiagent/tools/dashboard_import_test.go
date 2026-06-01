package tools

import (
	"encoding/json"
	"testing"
)

// firstPanelDS returns the datasourceValue of the first panel that has one.
func firstPanelDS(t *testing.T, configs map[string]interface{}) interface{} {
	t.Helper()
	panels, _ := configs["panels"].([]interface{})
	for _, p := range panels {
		pm := p.(map[string]interface{})
		if dv, ok := pm["datasourceValue"]; ok {
			return dv
		}
	}
	t.Fatal("no panel with datasourceValue")
	return nil
}

// dsVars returns the datasource-type variables from configs.var.
func dsVars(configs map[string]interface{}) []map[string]interface{} {
	var out []map[string]interface{}
	vars, _ := configs["var"].([]interface{})
	for _, v := range vars {
		vm, ok := v.(map[string]interface{})
		if ok && vm["type"] == "datasource" {
			out = append(out, vm)
		}
	}
	return out
}

func mustParse(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	cfg, err := parseTemplateConfigs(json.RawMessage(s))
	if err != nil {
		t.Fatalf("parseTemplateConfigs: %v", err)
	}
	return cfg
}

// Native n9e template: already uses a ${prom} datasource var. The var should be
// hardened (definition=prometheus, defaultValue=dsId) and panel refs untouched.
func TestNormalize_NativeProm(t *testing.T) {
	cfg := mustParse(t, `{
		"var":[{"name":"prom","type":"datasource","definition":"prometheus"},
		       {"name":"ident","type":"query","datasource":{"cate":"prometheus","value":"${prom}"},"definition":"label_values(x,ident)"}],
		"panels":[{"type":"timeseries","datasourceCate":"prometheus","datasourceValue":"${prom}"}]
	}`)

	n := normalizeTemplateDatasource(cfg, 7)
	if n != 1 {
		t.Fatalf("panel count = %d, want 1", n)
	}
	if got := firstPanelDS(t, cfg); got != "${prom}" {
		t.Fatalf("panel ds = %v, want ${prom}", got)
	}
	dv := dsVars(cfg)
	if len(dv) != 1 {
		t.Fatalf("datasource vars = %d, want 1", len(dv))
	}
	if dv[0]["definition"] != "prometheus" {
		t.Fatalf("definition = %v, want prometheus", dv[0]["definition"])
	}
	if dv[0]["defaultValue"] != int64(7) {
		t.Fatalf("defaultValue = %v, want 7", dv[0]["defaultValue"])
	}
}

// Grafana-imported template: panels reference ${DS_PROMETHEUS} but there is no
// matching var. The dangling ref must be repointed to the existing datasource
// var (here "datasource").
func TestNormalize_GrafanaDangling(t *testing.T) {
	cfg := mustParse(t, `{
		"var":[{"name":"datasource","type":"datasource","definition":"prometheus"}],
		"panels":[{"type":"timeseries","datasourceValue":"${DS_PROMETHEUS}"}]
	}`)

	normalizeTemplateDatasource(cfg, 3)
	if got := firstPanelDS(t, cfg); got != "${datasource}" {
		t.Fatalf("panel ds = %v, want ${datasource}", got)
	}
}

// Hardcoded numeric datasource id and NO datasource var at all: a canonical
// "prom" var must be injected and the literal id repointed at it.
func TestNormalize_HardcodedIdNoVar(t *testing.T) {
	cfg := mustParse(t, `{
		"panels":[{"type":"timeseries","datasourceValue":1},
		          {"type":"row","name":"分组"}]
	}`)

	n := normalizeTemplateDatasource(cfg, 9)
	if n != 1 { // 1 data panel; the row is not counted
		t.Fatalf("panel count = %d, want 1", n)
	}
	dv := dsVars(cfg)
	if len(dv) != 1 || dv[0]["name"] != "prom" {
		t.Fatalf("expected injected prom var, got %#v", dv)
	}
	if got := firstPanelDS(t, cfg); got != "${prom}" {
		t.Fatalf("panel ds = %v, want ${prom}", got)
	}
}

// Collapsed-row nesting (the ClickHouse case): top-level panels are all rows
// with no datasource binding; the real charts are nested inside them with a
// hardcoded datasourceValue:1 and the template has no datasource var. After
// normalization a prom var is injected, EVERY nested chart is repointed to
// ${prom}, and the count reflects the nested data panels — not the rows.
func TestNormalize_NestedRowPanels(t *testing.T) {
	cfg := mustParse(t, `{
		"var":[{"name":"instance","type":"query"}],
		"panels":[
			{"type":"row","name":"概览","panels":[
				{"type":"timeseries","datasourceValue":1},
				{"type":"stat","datasourceValue":1}
			]},
			{"type":"row","name":"详情","panels":[
				{"type":"timeseries","datasourceValue":1}
			]}
		]
	}`)

	n := normalizeTemplateDatasource(cfg, 6)
	if n != 3 { // 3 nested data panels; 2 rows not counted
		t.Fatalf("panel count = %d, want 3", n)
	}
	dv := dsVars(cfg)
	if len(dv) != 1 || dv[0]["name"] != "prom" {
		t.Fatalf("expected injected prom var, got %#v", dv)
	}
	// Every nested panel must now point at ${prom}, not literal 1.
	rows, _ := cfg["panels"].([]interface{})
	for _, r := range rows {
		nested, _ := r.(map[string]interface{})["panels"].([]interface{})
		for _, p := range nested {
			if got := p.(map[string]interface{})["datasourceValue"]; got != "${prom}" {
				t.Fatalf("nested panel ds = %v, want ${prom}", got)
			}
		}
	}
}

// Optional datasource: with dsId=0 the var is still hardened to prometheus but
// no defaultValue is pinned (FE auto-selects at view time), and panel refs stay.
func TestNormalize_NoDatasourceId(t *testing.T) {
	cfg := mustParse(t, `{
		"var":[{"name":"prom","type":"datasource","definition":"old"}],
		"panels":[{"type":"timeseries","datasourceValue":"${prom}"}]
	}`)

	normalizeTemplateDatasource(cfg, 0)
	dv := dsVars(cfg)
	if len(dv) != 1 || dv[0]["definition"] != "prometheus" {
		t.Fatalf("datasource var not hardened: %#v", dv)
	}
	if _, ok := dv[0]["defaultValue"]; ok {
		t.Fatalf("defaultValue should not be set when dsId=0, got %v", dv[0]["defaultValue"])
	}
	if got := firstPanelDS(t, cfg); got != "${prom}" {
		t.Fatalf("panel ds = %v, want ${prom}", got)
	}
}

// Stringified configs (configs stored as a JSON string) must parse the same.
func TestParseTemplateConfigs_Stringified(t *testing.T) {
	cfg := mustParse(t, `"{\"panels\":[],\"var\":[]}"`)
	if cfg["panels"] == nil {
		t.Fatalf("stringified configs did not parse: %#v", cfg)
	}
}
