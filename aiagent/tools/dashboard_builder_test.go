package tools

import (
	"encoding/json"
	"testing"
)

// helper: 从 layout 获取 float64 值
func layoutVal(panel map[string]interface{}, key string) float64 {
	layout := panel["layout"].(map[string]interface{})
	v, _ := layout[key].(float64)
	return v
}

func TestBuildConfigs_Basic(t *testing.T) {
	variables := []VariableSpec{
		{Name: "ident", Label: "主机", Definition: "label_values(cpu_usage_idle, ident)"},
	}
	panels := []PanelSpec{
		{Name: "CPU使用率", Type: "stat", Queries: []QuerySpec{{PromQL: `avg(cpu_usage_active{ident=~"$ident"})`, Legend: "CPU"}}, Unit: "percent"},
		{Name: "内存使用率", Type: "stat", Queries: []QuerySpec{{PromQL: `avg(mem_used_percent{ident=~"$ident"})`, Legend: "内存"}}, Unit: "percent"},
		{Name: "磁盘使用率", Type: "stat", Queries: []QuerySpec{{PromQL: `max(disk_used_percent{ident=~"$ident"})`, Legend: "磁盘"}}, Unit: "percent"},
		{Name: "运行时间", Type: "stat", Queries: []QuerySpec{{PromQL: `min(system_uptime{ident=~"$ident"})`, Legend: "Uptime"}}, Unit: "seconds"},
		{Name: "CPU趋势", Type: "timeseries", Queries: []QuerySpec{{PromQL: `cpu_usage_active{cpu="cpu-total",ident=~"$ident"}`, Legend: "{{ident}}"}}, Unit: "percent"},
		{Name: "内存趋势", Type: "timeseries", Queries: []QuerySpec{{PromQL: `mem_used_percent{ident=~"$ident"}`, Legend: "{{ident}}"}}, Unit: "percent"},
	}

	result, err := buildConfigs(5, variables, panels)
	if err != nil {
		t.Fatalf("buildConfigs failed: %v", err)
	}

	var configs map[string]interface{}
	if err := json.Unmarshal([]byte(result), &configs); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	// 顶层字段
	if configs["version"] != "3.4.0" {
		t.Errorf("version: got %v", configs["version"])
	}
	if configs["graphTooltip"] != "sharedCrosshair" {
		t.Errorf("graphTooltip: got %v", configs["graphTooltip"])
	}

	// 变量: var[0] 是内置的 datasource 变量，var[1] 才是用户声明的 ident
	vars := configs["var"].([]interface{})
	if len(vars) != 2 {
		t.Fatalf("expected 2 vars (datasource + ident), got %d", len(vars))
	}
	dsVar := vars[0].(map[string]interface{})
	if dsVar["name"] != "prom" || dsVar["type"] != "datasource" || dsVar["definition"] != "prometheus" {
		t.Errorf("datasource var unexpected: %+v", dsVar)
	}
	if dsVar["defaultValue"] != float64(5) {
		t.Errorf("datasource var defaultValue: got %v, want 5", dsVar["defaultValue"])
	}
	v := vars[1].(map[string]interface{})
	if v["name"] != "ident" {
		t.Errorf("query var name: got %v, want ident", v["name"])
	}
	ds := v["datasource"].(map[string]interface{})
	if ds["cate"] != "prometheus" {
		t.Errorf("query var datasource.cate: got %v", ds["cate"])
	}
	// Query variables reference the datasource variable via template interp,
	// not a literal ID — this is what makes the dropdown actually switch.
	if ds["value"] != "${prom}" {
		t.Errorf("query var datasource.value: got %v, want ${prom}", ds["value"])
	}

	// 面板
	builtPanels := configs["panels"].([]interface{})
	if len(builtPanels) != 6 {
		t.Fatalf("expected 6 panels, got %d", len(builtPanels))
	}

	// 检查所有面板有必填字段
	for i, p := range builtPanels {
		pm := p.(map[string]interface{})
		for _, field := range []string{"version", "id", "name", "type", "datasourceCate", "datasourceValue", "layout", "targets", "options", "custom"} {
			if _, ok := pm[field]; !ok {
				t.Errorf("panel[%d] missing field: %s", i, field)
			}
		}
		layout := pm["layout"].(map[string]interface{})
		if layout["i"] != pm["id"] {
			t.Errorf("panel[%d] layout.i (%v) != id (%v)", i, layout["i"], pm["id"])
		}
		if pm["datasourceValue"] != "${prom}" {
			t.Errorf("panel[%d] datasourceValue: got %v, want ${prom}", i, pm["datasourceValue"])
		}
	}

	// 4个stat应该在一行 (w=6, x=0,6,12,18, y=0)
	for i, wantX := range []float64{0, 6, 12, 18} {
		pm := builtPanels[i].(map[string]interface{})
		if layoutVal(pm, "x") != wantX || layoutVal(pm, "y") != 0 {
			t.Errorf("stat[%d] layout: x=%v y=%v, want %v,0", i, layoutVal(pm, "x"), layoutVal(pm, "y"), wantX)
		}
	}

	// 2个timeseries: y = stat.h(4), x=0,12
	p4 := builtPanels[4].(map[string]interface{})
	p5 := builtPanels[5].(map[string]interface{})
	if layoutVal(p4, "x") != 0 || layoutVal(p4, "y") != 4 {
		t.Errorf("timeseries[0]: x=%v y=%v, want 0,4", layoutVal(p4, "x"), layoutVal(p4, "y"))
	}
	if layoutVal(p5, "x") != 12 || layoutVal(p5, "y") != 4 {
		t.Errorf("timeseries[1]: x=%v y=%v, want 12,4", layoutVal(p5, "x"), layoutVal(p5, "y"))
	}

	// target 结构
	targets := builtPanels[0].(map[string]interface{})["targets"].([]interface{})
	t0 := targets[0].(map[string]interface{})
	if t0["refId"] != "A" {
		t.Errorf("target refId: got %v, want A", t0["refId"])
	}
	if t0["expr"] == nil || t0["expr"] == "" {
		t.Error("target expr is empty")
	}
}

func TestBuildConfigs_RefIDsFollowFrontendSequence(t *testing.T) {
	queries := make([]QuerySpec, 28)
	for i := range queries {
		queries[i] = QuerySpec{PromQL: "up"}
	}

	result, err := buildConfigs(1, nil, []PanelSpec{{Name: "many queries", Type: "timeseries", Queries: queries}})
	if err != nil {
		t.Fatalf("buildConfigs failed: %v", err)
	}
	var configs map[string]interface{}
	if err := json.Unmarshal([]byte(result), &configs); err != nil {
		t.Fatalf("unmarshal configs: %v", err)
	}
	targets := configs["panels"].([]interface{})[0].(map[string]interface{})["targets"].([]interface{})
	for index, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB"} {
		if got := targets[index].(map[string]interface{})["refId"]; got != want {
			t.Errorf("target[%d] refId = %v, want %s", index, got, want)
		}
	}
}

func TestBuildConfigs_WithRows(t *testing.T) {
	panels := []PanelSpec{
		{Name: "概览", Type: "row"},
		{Name: "CPU", Type: "stat", Queries: []QuerySpec{{PromQL: "avg(cpu_usage_active)"}}, Unit: "percent"},
		{Name: "详情", Type: "row"},
		{Name: "CPU趋势", Type: "timeseries", Queries: []QuerySpec{{PromQL: "cpu_usage_active"}}},
	}

	result, err := buildConfigs(1, nil, panels)
	if err != nil {
		t.Fatalf("buildConfigs failed: %v", err)
	}

	var configs map[string]interface{}
	json.Unmarshal([]byte(result), &configs)

	builtPanels := configs["panels"].([]interface{})

	// row at y=0, w=24, h=1
	row0 := builtPanels[0].(map[string]interface{})
	if layoutVal(row0, "y") != 0 || layoutVal(row0, "w") != 24 || layoutVal(row0, "h") != 1 {
		t.Errorf("row[0]: y=%v w=%v h=%v", layoutVal(row0, "y"), layoutVal(row0, "w"), layoutVal(row0, "h"))
	}

	// stat at y=1
	stat := builtPanels[1].(map[string]interface{})
	if layoutVal(stat, "y") != 1 {
		t.Errorf("stat after row: y=%v, want 1", layoutVal(stat, "y"))
	}

	// second row at y=1+4=5
	row1 := builtPanels[2].(map[string]interface{})
	if layoutVal(row1, "y") != 5 {
		t.Errorf("row[1]: y=%v, want 5", layoutVal(row1, "y"))
	}

	// timeseries at y=6
	ts := builtPanels[3].(map[string]interface{})
	if layoutVal(ts, "y") != 6 {
		t.Errorf("timeseries after row: y=%v, want 6", layoutVal(ts, "y"))
	}
}

// TestBuildConfigs_MixedPanelTypesKeepAlignedRows protects the default layout
// from ReactGridLayout's vertical compaction. Every position here is blocked
// by an overlapping panel above it, so loading the dashboard cannot pull a
// panel into a visual gap and make the generated dashboard look staggered.
func TestBuildConfigs_MixedPanelTypesKeepAlignedRows(t *testing.T) {
	panels := []PanelSpec{
		{Name: "trend", Type: "timeseries"},
		{Name: "stat", Type: "stat"},
		{Name: "gauge", Type: "gauge"},
		{Name: "bar", Type: "barGauge"},
		{Name: "pie", Type: "pie"},
		{Name: "table", Type: "tableNG"},
		{Name: "text", Type: "text"},
	}
	result, err := buildConfigs(1, nil, panels)
	if err != nil {
		t.Fatal(err)
	}
	var configs map[string]interface{}
	if err := json.Unmarshal([]byte(result), &configs); err != nil {
		t.Fatal(err)
	}

	byName := make(map[string]map[string]interface{})
	for _, raw := range configs["panels"].([]interface{}) {
		panel := raw.(map[string]interface{})
		byName[panel["name"].(string)] = panel
	}
	for name, want := range map[string][4]float64{
		"trend": {0, 0, 12, 8},
		"stat":  {12, 0, 6, 4},
		"gauge": {18, 0, 6, 8},
		"bar":   {0, 8, 12, 8},
		"pie":   {12, 8, 12, 8},
		"table": {0, 16, 12, 10},
		"text":  {12, 16, 12, 4},
	} {
		got := [4]float64{layoutVal(byName[name], "x"), layoutVal(byName[name], "y"), layoutVal(byName[name], "w"), layoutVal(byName[name], "h")}
		if got != want {
			t.Errorf("%s layout = %v, want %v", name, got, want)
		}
	}
}

func TestBuildConfigs_PanelDefaultsAndTableNG(t *testing.T) {
	falseValue := false
	panels := []PanelSpec{
		{Name: "trend", Type: "timeseries"},
		{Name: "stat", Type: "stat"},
		{Name: "gauge", Type: "gauge"},
		{Name: "bar", Type: "barGauge"},
		{Name: "pie", Type: "pie"},
		{Name: "legacy table", Type: "table", Queries: []QuerySpec{{PromQL: "up", Instant: &falseValue}}},
		{Name: "new table", Type: "tableNG", Queries: []QuerySpec{{PromQL: "up", Instant: &falseValue}}},
		{Name: "note", Type: "text", Desc: "runbook"},
	}
	result, err := buildConfigs(1, nil, panels)
	if err != nil {
		t.Fatal(err)
	}
	var configs map[string]interface{}
	if err := json.Unmarshal([]byte(result), &configs); err != nil {
		t.Fatal(err)
	}
	built := configs["panels"].([]interface{})
	byName := make(map[string]map[string]interface{}, len(built))
	for _, raw := range built {
		p := raw.(map[string]interface{})
		byName[p["name"].(string)] = p
		if _, ok := p["transformationsNG"]; ok {
			t.Fatalf("%q unexpectedly has transformationsNG", p["name"])
		}
		if _, ok := p["layout"].(map[string]interface{})["isResizable"]; ok {
			t.Fatalf("%q layout unexpectedly has isResizable", p["name"])
		}
		thresholds := p["options"].(map[string]interface{})["thresholds"].(map[string]interface{})
		steps := thresholds["steps"].([]interface{})
		if len(steps) == 0 || steps[0].(map[string]interface{})["type"] != "base" {
			t.Fatalf("%q thresholds has no base step: %#v", p["name"], thresholds)
		}
	}

	trend := byName["trend"]["custom"].(map[string]interface{})
	for key, want := range map[string]interface{}{"fillOpacity": 0.01, "pointSize": float64(5), "barAlignment": float64(0), "barWidthFactor": 0.6} {
		if trend[key] != want {
			t.Errorf("timeseries custom.%s = %v, want %v", key, trend[key], want)
		}
	}
	stat := byName["stat"]["custom"].(map[string]interface{})
	if stat["colSpan"] != float64(0) || stat["valueField"] != "Value" || stat["orientation"] != "auto" {
		t.Errorf("unexpected stat custom: %#v", stat)
	}
	gauge := byName["gauge"]
	if _, ok := gauge["custom"].(map[string]interface{})["min"]; ok {
		t.Error("gauge min must live in standardOptions, not custom")
	}
	gaugeOptions := gauge["options"].(map[string]interface{})
	gaugeStandard := gaugeOptions["standardOptions"].(map[string]interface{})
	if gaugeStandard["min"] != float64(0) || gaugeStandard["max"] != float64(100) {
		t.Errorf("unexpected gauge range: %#v", gaugeStandard)
	}
	if len(gaugeOptions["thresholds"].(map[string]interface{})["steps"].([]interface{})) != 3 {
		t.Errorf("gauge should have three thresholds: %#v", gaugeOptions["thresholds"])
	}
	bar := byName["bar"]["custom"].(map[string]interface{})
	if _, ok := bar["orientation"]; ok {
		t.Error("barGauge must not write orientation")
	}
	for _, key := range []string{"valueField", "sortOrder", "otherPosition", "valueMode"} {
		if _, ok := bar[key]; !ok {
			t.Errorf("barGauge missing custom.%s", key)
		}
	}
	pie := byName["pie"]["custom"].(map[string]interface{})
	if pie["valueField"] != "Value" || pie["detailName"] != "" ||
		pie["textMode"] != "valueAndName" || pie["colorMode"] != "value" ||
		pie["legengPosition"] != "right" {
		t.Errorf("unexpected pie custom: %#v", pie)
	}
	if textSize, ok := pie["textSize"].(map[string]interface{}); !ok || len(textSize) != 0 {
		t.Errorf("pie custom.textSize = %#v, want empty object", pie["textSize"])
	}
	for _, name := range []string{"legacy table", "new table"} {
		p := byName[name]
		if p["type"] != "tableNG" || layoutVal(p, "w") != 12 || layoutVal(p, "h") != 10 {
			t.Errorf("%s should be a 12x10 tableNG: %#v", name, p)
		}
		custom := p["custom"].(map[string]interface{})
		if custom["showHeader"] != true || custom["filterable"] != false || custom["cellOptions"].(map[string]interface{})["type"] != "none" {
			t.Errorf("unexpected tableNG custom: %#v", custom)
		}
		if p["targets"].([]interface{})[0].(map[string]interface{})["instant"] != true {
			t.Errorf("%s target must be instant", name)
		}
	}
	note := byName["note"]
	if _, ok := note["description"]; ok {
		t.Error("text panel must keep its content only in custom.content")
	}
	if note["custom"].(map[string]interface{})["textColor"] != "#000000" {
		t.Errorf("text defaults not applied: %#v", note["custom"])
	}
}
