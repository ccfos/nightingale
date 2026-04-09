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
