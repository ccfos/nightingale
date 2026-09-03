package tools

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/toolkits/pkg/logger"
)

const n9eVersion = "3.4.0"

// defaultTimeseriesCustom timeseries 面板的默认 custom 配置
var defaultTimeseriesCustom = map[string]interface{}{
	"version":           n9eVersion,
	"drawStyle":         "lines",
	"lineInterpolation": "smooth",
	"lineWidth":         float64(2),
	"fillOpacity":       0.01,
	"gradientMode":      "none",
	"stack":             "off",
	"showPoints":        "none",
	"scaleDistribution": map[string]interface{}{"type": "linear"},
}

// defaultStatCustom stat 面板的默认 custom 配置
var defaultStatCustom = map[string]interface{}{
	"version":    n9eVersion,
	"textMode":   "valueAndName",
	"colorMode":  "value",
	"calc":       "lastNotNull",
	"colSpan":    1,
	"textSize":   map[string]interface{}{},
	"orientation": "",
}

// normalizeConfigs 将 AI 生成的 configs JSON 规范化为 n9e 标准格式
// 自动修正 Grafana 风格字段、补全缺失的必填字段
func normalizeConfigs(configsJSON string, datasourceId int64) (string, error) {
	var configs map[string]interface{}
	if err := json.Unmarshal([]byte(configsJSON), &configs); err != nil {
		return "", fmt.Errorf("invalid configs JSON: %v", err)
	}

	fixed := 0

	// 1. 修正顶层 version
	if v, _ := configs["version"].(string); v != n9eVersion {
		configs["version"] = n9eVersion
		fixed++
	}

	// 2. 补全顶层字段
	if _, ok := configs["graphTooltip"]; !ok {
		configs["graphTooltip"] = "sharedCrosshair"
		fixed++
	}
	if _, ok := configs["graphZoom"]; !ok {
		configs["graphZoom"] = "default"
	}
	if _, ok := configs["links"]; !ok {
		configs["links"] = []interface{}{}
	}

	// 3. 修正变量
	if vars, ok := configs["var"].([]interface{}); ok {
		for _, v := range vars {
			vm, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			fixed += normalizeVariable(vm, datasourceId)
		}
	}

	// 4. 修正面板
	if panels, ok := configs["panels"].([]interface{}); ok {
		for i, p := range panels {
			pm, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			fixed += normalizePanel(pm, i, datasourceId)
		}
	}

	result, err := json.Marshal(configs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal normalized configs: %v", err)
	}

	if fixed > 0 {
		logger.Infof("normalizeConfigs: auto-fixed %d issues", fixed)
	}

	return string(result), nil
}

// normalizeVariable 修正单个变量定义
func normalizeVariable(v map[string]interface{}, datasourceId int64) int {
	fixed := 0

	// datasource: 字符串或缺失 → 标准对象
	ds, dsOk := v["datasource"]
	needFix := !dsOk
	if dsOk {
		switch ds.(type) {
		case string:
			needFix = true
		case map[string]interface{}:
			// 已经是对象，检查内容
			dsMap := ds.(map[string]interface{})
			if _, ok := dsMap["cate"]; !ok {
				needFix = true
			}
		default:
			needFix = true
		}
	}
	if needFix {
		v["datasource"] = map[string]interface{}{
			"cate":  "prometheus",
			"value": datasourceId,
		}
		fixed++
	}

	// 补全常用字段默认值
	setDefault(v, "reg", "")
	setDefault(v, "allOption", true)
	setDefault(v, "allValue", "")
	setDefault(v, "defaultValue", "")
	setDefault(v, "hide", false)

	// 移除非标准字段
	delete(v, "options")

	return fixed
}

// normalizePanel 修正单个面板定义
func normalizePanel(p map[string]interface{}, index int, datasourceId int64) int {
	fixed := 0
	panelType := stringVal(p, "type")

	// --- version ---
	if v, _ := p["version"].(string); v != n9eVersion {
		p["version"] = n9eVersion
		fixed++
	}

	// --- title → name ---
	if _, hasName := p["name"]; !hasName {
		if title, hasTitle := p["title"]; hasTitle {
			p["name"] = title
			delete(p, "title")
			fixed++
		} else {
			p["name"] = fmt.Sprintf("Panel %d", index+1)
			fixed++
		}
	}

	// --- id ---
	panelId := stringVal(p, "id")
	if panelId == "" {
		// 从 name 生成 id，或用索引
		name := stringVal(p, "name")
		if name != "" {
			panelId = fmt.Sprintf("panel-%d", index)
		} else {
			panelId = fmt.Sprintf("panel-%d", index)
		}
		p["id"] = panelId
		fixed++
	}

	// --- layout ---
	if _, hasLayout := p["layout"]; !hasLayout {
		layout := map[string]interface{}{"i": panelId}

		// 从顶层散放的 x/y/w/h 或 gridPos 中提取
		if gp, ok := p["gridPos"].(map[string]interface{}); ok {
			layout["x"] = gp["x"]
			layout["y"] = gp["y"]
			layout["w"] = gp["w"]
			layout["h"] = gp["h"]
			delete(p, "gridPos")
			fixed++
		} else {
			// x/y/w/h 散放在顶层
			moved := false
			for _, key := range []string{"x", "y", "w", "h"} {
				if val, ok := p[key]; ok {
					layout[key] = val
					delete(p, key)
					moved = true
				}
			}
			if moved {
				fixed++
			} else {
				// 完全没有位置信息，按序排列
				layout["x"] = float64((index % 2) * 12)
				layout["y"] = float64((index / 2) * 8)
				layout["w"] = float64(12)
				layout["h"] = float64(8)
				fixed++
			}
		}

		layout["i"] = panelId
		p["layout"] = layout
	} else {
		// layout 已存在，确保 i 与 id 一致
		if lm, ok := p["layout"].(map[string]interface{}); ok {
			lm["i"] = panelId
		}
	}

	// --- datasourceCate / datasourceValue ---
	if _, ok := p["datasourceCate"]; !ok {
		p["datasourceCate"] = "prometheus"
		fixed++
	}
	if _, ok := p["datasourceValue"]; !ok {
		if datasourceId > 0 {
			p["datasourceValue"] = datasourceId
		} else {
			p["datasourceValue"] = float64(1)
		}
		fixed++
	}

	// --- targets ---
	if targets, ok := p["targets"].([]interface{}); ok {
		for j, t := range targets {
			if tm, ok := t.(map[string]interface{}); ok {
				fixed += normalizeTarget(tm, j)
			}
		}
	}

	// --- custom ---
	if _, ok := p["custom"]; !ok {
		switch panelType {
		case "timeseries":
			p["custom"] = copyMap(defaultTimeseriesCustom)
		case "stat":
			p["custom"] = copyMap(defaultStatCustom)
		default:
			p["custom"] = map[string]interface{}{"version": n9eVersion}
		}
		fixed++
	} else {
		// 确保 custom 里有 version
		if cm, ok := p["custom"].(map[string]interface{}); ok {
			setDefault(cm, "version", n9eVersion)
		}
	}

	// --- options 补全 ---
	if _, ok := p["options"]; !ok {
		p["options"] = map[string]interface{}{}
	}
	if _, ok := p["overrides"]; !ok {
		p["overrides"] = []interface{}{}
	}
	if _, ok := p["transformationsNG"]; !ok {
		p["transformationsNG"] = []interface{}{}
	}

	return fixed
}

// normalizeTarget 修正单个 target
func normalizeTarget(t map[string]interface{}, index int) int {
	fixed := 0

	// ref → refId
	if _, ok := t["refId"]; !ok {
		if ref, ok := t["ref"]; ok {
			t["refId"] = ref
			delete(t, "ref")
			fixed++
		} else {
			t["refId"] = string(rune('A' + index))
			fixed++
		}
	}

	// __mode__
	if _, ok := t["__mode__"]; !ok {
		t["__mode__"] = "__query__"
		fixed++
	}

	return fixed
}

// --- helpers ---

func stringVal(m map[string]interface{}, key string) string {
	switch v := m[key].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

func setDefault(m map[string]interface{}, key string, val interface{}) {
	if _, ok := m[key]; !ok {
		m[key] = val
	}
}

func copyMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		if sub, ok := v.(map[string]interface{}); ok {
			dst[k] = copyMap(sub)
		} else {
			dst[k] = v
		}
	}
	return dst
}
