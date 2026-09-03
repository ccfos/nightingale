package tools

import (
	"encoding/json"
	"testing"
)

func TestNormalizeConfigs_GrafanaStyle(t *testing.T) {
	// 这是 AI 实际生成的有问题的 configs（Grafana 风格）
	input := `{
		"version": "1.0",
		"var": [{
			"name": "ident",
			"type": "query",
			"definition": "label_values(cpu_usage_active,ident)",
			"options": [],
			"multi": true
		}],
		"panels": [{
			"type": "timeseries",
			"title": "CPU 使用率",
			"x": 0, "y": 0, "w": 12, "h": 8,
			"targets": [{
				"ref": "A",
				"expr": "cpu_usage_active{cpu=\"cpu-total\",ident=~\"$ident\"}"
			}],
			"options": {
				"legend": {"displayMode": "table", "placement": "bottom"}
			}
		}, {
			"type": "timeseries",
			"title": "内存使用率",
			"x": 12, "y": 0, "w": 12, "h": 8,
			"targets": [{
				"ref": "A",
				"expr": "mem_used_percent{ident=~\"$ident\"}"
			}],
			"options": {
				"legend": {"displayMode": "table", "placement": "bottom"}
			}
		}]
	}`

	result, err := normalizeConfigs(input, 5)
	if err != nil {
		t.Fatalf("normalizeConfigs failed: %v", err)
	}

	var configs map[string]interface{}
	if err := json.Unmarshal([]byte(result), &configs); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	// 检查顶层 version
	if v := configs["version"]; v != "3.4.0" {
		t.Errorf("version: got %v, want 3.4.0", v)
	}

	// 检查 graphTooltip
	if v := configs["graphTooltip"]; v != "sharedCrosshair" {
		t.Errorf("graphTooltip: got %v, want sharedCrosshair", v)
	}

	// 检查变量 datasource
	vars := configs["var"].([]interface{})
	v0 := vars[0].(map[string]interface{})
	ds := v0["datasource"].(map[string]interface{})
	if ds["cate"] != "prometheus" {
		t.Errorf("var datasource.cate: got %v, want prometheus", ds["cate"])
	}
	if ds["value"] != float64(5) {
		t.Errorf("var datasource.value: got %v, want 5", ds["value"])
	}
	// options 应被删除
	if _, ok := v0["options"]; ok {
		t.Error("var should not have 'options' field")
	}

	// 检查面板
	panels := configs["panels"].([]interface{})
	if len(panels) != 2 {
		t.Fatalf("expected 2 panels, got %d", len(panels))
	}

	p0 := panels[0].(map[string]interface{})

	// title → name
	if _, ok := p0["title"]; ok {
		t.Error("panel should not have 'title' field")
	}
	if p0["name"] != "CPU 使用率" {
		t.Errorf("panel name: got %v, want 'CPU 使用率'", p0["name"])
	}

	// version
	if p0["version"] != "3.4.0" {
		t.Errorf("panel version: got %v, want 3.4.0", p0["version"])
	}

	// id
	if _, ok := p0["id"]; !ok {
		t.Error("panel missing 'id' field")
	}

	// layout
	layout, ok := p0["layout"].(map[string]interface{})
	if !ok {
		t.Fatal("panel missing 'layout' object")
	}
	if layout["x"] != float64(0) || layout["y"] != float64(0) || layout["w"] != float64(12) || layout["h"] != float64(8) {
		t.Errorf("layout values wrong: %v", layout)
	}
	if layout["i"] != p0["id"] {
		t.Errorf("layout.i (%v) != panel.id (%v)", layout["i"], p0["id"])
	}
	// x/y/w/h 不应在顶层
	for _, key := range []string{"x", "y", "w", "h"} {
		if _, ok := p0[key]; ok {
			t.Errorf("panel should not have top-level '%s'", key)
		}
	}

	// datasourceCate / datasourceValue
	if p0["datasourceCate"] != "prometheus" {
		t.Errorf("panel datasourceCate: got %v, want prometheus", p0["datasourceCate"])
	}
	if p0["datasourceValue"] != float64(5) {
		t.Errorf("panel datasourceValue: got %v, want 5", p0["datasourceValue"])
	}

	// custom
	custom, ok := p0["custom"].(map[string]interface{})
	if !ok {
		t.Fatal("panel missing 'custom' object")
	}
	if custom["drawStyle"] != "lines" {
		t.Errorf("custom.drawStyle: got %v, want lines", custom["drawStyle"])
	}

	// targets
	targets := p0["targets"].([]interface{})
	t0 := targets[0].(map[string]interface{})
	if _, ok := t0["ref"]; ok {
		t.Error("target should not have 'ref' field")
	}
	if t0["refId"] != "A" {
		t.Errorf("target refId: got %v, want A", t0["refId"])
	}
	if t0["__mode__"] != "__query__" {
		t.Errorf("target __mode__: got %v, want __query__", t0["__mode__"])
	}

	t.Logf("Normalized result (first 500 chars): %.500s...", result)
}

func TestNormalizeConfigs_RealCase(t *testing.T) {
	// 用户反馈的真实错误 configs
	input := `{"version":"1.0","var":[{"name":"ident","type":"query","definition":"label_values(cpu_usage_active,ident)","options":[],"multi":true}],"panels":[{"type":"timeseries","title":"CPU 使用率","x":0,"y":0,"w":12,"h":8,"targets":[{"ref":"A","expr":"cpu_usage_active{cpu=\"cpu-total\",ident=~\"$ident\"}"}],"options":{"legend":{"displayMode":"table","placement":"bottom"}}},{"type":"timeseries","title":"内存使用率","x":12,"y":0,"w":12,"h":8,"targets":[{"ref":"A","expr":"mem_used_percent{ident=~\"$ident\"}"}],"options":{"legend":{"displayMode":"table","placement":"bottom"}}},{"type":"timeseries","title":"系统负载","x":0,"y":8,"w":12,"h":8,"targets":[{"ref":"A","expr":"load1{ident=~\"$ident\"}"},{"ref":"B","expr":"load5{ident=~\"$ident\"}"},{"ref":"C","expr":"load15{ident=~\"$ident\"}"}],"options":{"legend":{"displayMode":"table","placement":"bottom"}}},{"type":"timeseries","title":"磁盘使用率","x":12,"y":8,"w":12,"h":8,"targets":[{"ref":"A","expr":"disk_used_percent{ident=~\"$ident\"}"}],"options":{"legend":{"displayMode":"table","placement":"bottom"}}}]}`

	result, err := normalizeConfigs(input, 3)
	if err != nil {
		t.Fatalf("normalizeConfigs failed: %v", err)
	}

	var configs map[string]interface{}
	json.Unmarshal([]byte(result), &configs)

	// 验证所有面板都有正确结构
	panels := configs["panels"].([]interface{})
	for i, p := range panels {
		pm := p.(map[string]interface{})

		// 不能有 title
		if _, ok := pm["title"]; ok {
			t.Errorf("panel[%d] should not have 'title'", i)
		}
		// 必须有 name
		if _, ok := pm["name"]; !ok {
			t.Errorf("panel[%d] missing 'name'", i)
		}
		// 必须有 layout 对象
		layout, ok := pm["layout"].(map[string]interface{})
		if !ok {
			t.Errorf("panel[%d] missing 'layout' object", i)
		} else if _, ok := layout["i"]; !ok {
			t.Errorf("panel[%d] layout missing 'i'", i)
		}
		// 不能有顶层 x/y/w/h
		for _, key := range []string{"x", "y", "w", "h"} {
			if _, ok := pm[key]; ok {
				t.Errorf("panel[%d] should not have top-level '%s'", i, key)
			}
		}
		// 必须有 datasourceCate
		if pm["datasourceCate"] != "prometheus" {
			t.Errorf("panel[%d] datasourceCate: got %v", i, pm["datasourceCate"])
		}
		// targets 必须有 refId 和 __mode__
		targets := pm["targets"].([]interface{})
		for j, tt := range targets {
			tm := tt.(map[string]interface{})
			if _, ok := tm["refId"]; !ok {
				t.Errorf("panel[%d] target[%d] missing 'refId'", i, j)
			}
			if tm["__mode__"] != "__query__" {
				t.Errorf("panel[%d] target[%d] missing '__mode__'", i, j)
			}
			if _, ok := tm["ref"]; ok {
				t.Errorf("panel[%d] target[%d] should not have 'ref'", i, j)
			}
		}
	}

	t.Logf("All %d panels normalized correctly", len(panels))
}
