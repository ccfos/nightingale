package tools

import (
	"encoding/json"
	"fmt"
)

// ============================================================================
// Simplified spec types — AI 只需要生成这些简单结构
// ============================================================================

// PanelSpec AI 生成的简化面板描述
type PanelSpec struct {
	Name    string      `json:"name"`              // 面板标题
	Type    string      `json:"type"`              // timeseries | stat | gauge | barGauge | pie | table | row | text
	Queries []QuerySpec `json:"queries,omitempty"` // PromQL 查询列表
	W       int         `json:"w,omitempty"`       // 宽度(网格列数)，默认按类型自动设置
	H       int         `json:"h,omitempty"`       // 高度(网格行数)，默认按类型自动设置
	Unit    string      `json:"unit,omitempty"`    // 单位: percent, bytesIEC, bitsIEC, seconds 等
	Stack   bool        `json:"stack,omitempty"`   // 是否堆叠(仅 timeseries)
	Desc    string      `json:"description,omitempty"`
}

// QuerySpec AI 生成的简化查询描述。
//
// Ref/Step/Hide/Instant/Delete are only meaningful for update_dashboard's
// incremental panel edit (see mergeTargets): Ref locates the existing target to
// edit in place — an empty Ref always adds a new curve (no positional matching)
// — the pointer fields (Instant/Step/Hide) distinguish "not provided" (nil →
// keep the existing target's value) from an explicit set (incl. instant=false),
// and Delete removes the ref-matched curve. create_dashboard ignores Ref/Delete
// (refIds are assigned sequentially) but still honors Instant/Step/Hide.
type QuerySpec struct {
	PromQL  string `json:"promql"`            // PromQL 表达式
	Legend  string `json:"legend,omitempty"`  // 图例模板, 如 "{{ident}}"
	Instant *bool  `json:"instant,omitempty"` // 即时查询(用于 stat/table); nil 保留原值, true/false 均可显式设置
	Ref     string `json:"ref,omitempty"`     // 编辑已有曲线时按 refId 匹配(如 "A"); 留空一律视为新增曲线
	Step    *int   `json:"step,omitempty"`    // 固定查询步长(秒); nil 保留原值
	Hide    *bool  `json:"hide,omitempty"`    // 是否隐藏该曲线; nil 保留原值
	Delete  bool   `json:"delete,omitempty"`  // 编辑时删除按 ref 匹配到的曲线(留空 ref 不会匹配任何曲线)
}

// UnmarshalJSON decodes a QuerySpec tolerantly: the LLM often quotes the scalar
// fields ("step":"15", "instant":"true") or writes integers as floats
// ("step":15.0), all of which the default decoder rejects — aborting the entire
// panels/variables parse. The loose fields go through flex* coercion; the rest
// decode normally. Used by both create_dashboard (PanelSpec.Queries) and
// update_dashboard (panelPatch.Queries).
func (q *QuerySpec) UnmarshalJSON(data []byte) error {
	var r struct {
		PromQL  string          `json:"promql"`
		Legend  string          `json:"legend"`
		Ref     string          `json:"ref"`
		Instant json.RawMessage `json:"instant"`
		Step    json.RawMessage `json:"step"`
		Hide    json.RawMessage `json:"hide"`
		Delete  json.RawMessage `json:"delete"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	q.PromQL, q.Legend, q.Ref = r.PromQL, r.Legend, r.Ref

	var err error
	if q.Instant, err = flexBoolPtr(r.Instant); err != nil {
		return fmt.Errorf("query field instant: %w", err)
	}
	if q.Hide, err = flexBoolPtr(r.Hide); err != nil {
		return fmt.Errorf("query field hide: %w", err)
	}
	if q.Step, err = flexIntPtr(r.Step); err != nil {
		return fmt.Errorf("query field step: %w", err)
	}
	if q.Delete, err = flexBool(r.Delete); err != nil {
		return fmt.Errorf("query field delete: %w", err)
	}
	return nil
}

// VariableSpec AI 生成的简化变量描述
type VariableSpec struct {
	Name       string `json:"name"`            // 变量名
	Label      string `json:"label,omitempty"` // 显示标签
	Definition string `json:"definition"`      // 如 label_values(metric, label)
	Multi      *bool  `json:"multi,omitempty"` // 是否多选，默认 true
}

// ============================================================================
// Builder — 从简化 spec 生成完整 n9e configs JSON
// ============================================================================

// datasourceVarName is the built-in datasource-type variable emitted by
// buildConfigs. Panels and query variables reference it via "${prom}" so
// the dashboard header renders a datasource dropdown and the dashboard
// stays reusable across Prometheus instances. This matches the convention
// used by the integration templates (e.g. integrations/Linux/dashboards/
// categraf-detail.json) and n9e built-in boards.
const datasourceVarName = "prom"

// datasourceVarRef is the template interpolation string — literally
// "${prom}" — used wherever a datasource ID would otherwise appear.
const datasourceVarRef = "${" + datasourceVarName + "}"

// buildConfigs 从简化的变量和面板描述生成完整的 n9e dashboard configs JSON
//
// The generated payload always includes a datasource-type variable named
// "prom" at var[0], and every panel / query variable references it via
// "${prom}" instead of a literal int. Without this, the dashboard header
// shows no datasource selector and the dashboard is tied to a single
// hardcoded datasource ID.
//
// datasourceId is still used to populate an initial fallback (so queries
// have a sensible default on first load), but the dashboard is not bound
// to it — users can switch datasources from the header after opening.
func buildConfigs(datasourceId int64, variables []VariableSpec, panels []PanelSpec) (string, error) {
	configs := map[string]interface{}{
		"version":      "3.4.0",
		"graphTooltip": "sharedCrosshair",
		"graphZoom":    "default",
		"links":        []interface{}{},
	}

	// 构建变量：datasource 变量置顶，后跟用户声明的 query 变量
	vars := make([]interface{}, 0, len(variables)+1)
	vars = append(vars, buildDatasourceVariable())
	for _, v := range variables {
		vars = append(vars, buildVariable(v))
	}
	configs["var"] = vars

	// 构建面板（自动计算布局）
	builtPanels := make([]interface{}, 0, len(panels))
	x, y, rowMaxH := 0, 0, 0
	for i, spec := range panels {
		w, h := defaultSize(spec.Type, spec.W, spec.H)

		// row 类型固定全宽
		if spec.Type == "row" {
			if rowMaxH > 0 || x > 0 {
				y += rowMaxH
				x = 0
				rowMaxH = 0
			}
			builtPanels = append(builtPanels, buildRowPanel(spec, i, y))
			y++
			continue
		}

		// 自动换行
		if x+w > 24 {
			y += rowMaxH
			x = 0
			rowMaxH = 0
		}

		panel := buildPanel(spec, i, x, y, w, h)
		builtPanels = append(builtPanels, panel)

		x += w
		if h > rowMaxH {
			rowMaxH = h
		}
	}
	configs["panels"] = builtPanels

	// datasourceId is intentionally unused in the marshalled payload —
	// panels reference ${prom} instead. Touch it to keep the parameter
	// surface stable for future wiring (e.g. defaultValue).
	_ = datasourceId

	result, err := json.Marshal(configs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal configs: %v", err)
	}
	return string(result), nil
}

// buildDatasourceVariable returns the header datasource dropdown variable.
// definition="prometheus" tells n9e to populate the dropdown with all
// configured Prometheus datasources; hide=false makes it visible.
func buildDatasourceVariable() map[string]interface{} {
	return map[string]interface{}{
		"name":       datasourceVarName,
		"type":       "datasource",
		"definition": "prometheus",
		"label":      "数据源",
		"hide":       false,
	}
}

func buildVariable(spec VariableSpec) map[string]interface{} {
	multi := true
	if spec.Multi != nil {
		multi = *spec.Multi
	}

	// Query variables reference the header datasource variable via
	// template interpolation ("${prom}") so they follow whatever
	// datasource the user selects in the dropdown.
	v := map[string]interface{}{
		"name":         spec.Name,
		"type":         "query",
		"definition":   spec.Definition,
		"reg":          "",
		"multi":        multi,
		"allOption":    true,
		"allValue":     "",
		"defaultValue": "",
		"hide":         false,
		"datasource": map[string]interface{}{
			"cate":  "prometheus",
			"value": datasourceVarRef,
		},
	}
	if spec.Label != "" {
		v["label"] = spec.Label
	}
	return v
}

func buildPanel(spec PanelSpec, index, x, y, w, h int) map[string]interface{} {
	panelId := fmt.Sprintf("panel-%d", index)

	// datasourceValue uses the "${prom}" template variable so panels
	// inherit whichever datasource the user picks in the header dropdown.
	panel := map[string]interface{}{
		"version":           "3.4.0",
		"id":                panelId,
		"type":              spec.Type,
		"name":              spec.Name,
		"datasourceCate":    "prometheus",
		"datasourceValue":   datasourceVarRef,
		"layout":            map[string]interface{}{"x": x, "y": y, "w": w, "h": h, "i": panelId, "isResizable": true},
		"targets":           buildTargets(spec.Queries),
		"options":           buildOptions(spec),
		"custom":            buildCustom(spec),
		"overrides":         []interface{}{},
		"transformationsNG": []interface{}{},
	}

	if spec.Desc != "" {
		panel["description"] = spec.Desc
	}

	return panel
}

func buildRowPanel(spec PanelSpec, index, y int) map[string]interface{} {
	panelId := fmt.Sprintf("panel-%d", index)
	return map[string]interface{}{
		"version":   "3.4.0",
		"id":        panelId,
		"type":      "row",
		"name":      spec.Name,
		"layout":    map[string]interface{}{"x": 0, "y": y, "w": 24, "h": 1, "i": panelId, "isResizable": false},
		"targets":   []interface{}{},
		"options":   map[string]interface{}{},
		"custom":    map[string]interface{}{},
		"overrides": []interface{}{},
	}
}

func buildTargets(queries []QuerySpec) []interface{} {
	targets := make([]interface{}, 0, len(queries))
	for i, q := range queries {
		t := map[string]interface{}{
			"refId": string(rune('A' + i)),
			"expr":  q.PromQL,
		}
		if q.Legend != "" {
			// n9e's FE editor, renderer, and every built-in/integration board
			// store a target's legend template under "legend" — NOT the Grafana
			// "legendFormat" key. Writing legendFormat persists silently but the
			// page ignores it, so the legend never renders. See targetLegend for
			// the read side (which still tolerates historical legendFormat).
			t["legend"] = q.Legend
		}
		if q.Instant != nil {
			t["instant"] = *q.Instant
		}
		if q.Step != nil {
			t["step"] = *q.Step
		}
		if q.Hide != nil {
			t["hide"] = *q.Hide
		}
		targets = append(targets, t)
	}
	return targets
}

func buildOptions(spec PanelSpec) map[string]interface{} {
	opts := map[string]interface{}{}

	// standardOptions. Panels are emitted at schema version 3.4.0, and the FE
	// renderer/editor reads the unit from "unit" (the legacy "util" key is only
	// migrated to "unit" for panels older than 3.3.0), so we must write "unit".
	if spec.Unit != "" {
		opts["standardOptions"] = map[string]interface{}{"unit": spec.Unit}
	} else {
		opts["standardOptions"] = map[string]interface{}{}
	}

	for k, v := range typeOptions(spec.Type) {
		opts[k] = v
	}

	return opts
}

// typeOptions returns the type-specific rendering options (legend/tooltip) for
// a panel type. Shared by panel creation (buildOptions) and update_dashboard's
// type change (changePanelType), so both paths emit the same defaults.
func typeOptions(panelType string) map[string]interface{} {
	switch panelType {
	case "timeseries":
		return map[string]interface{}{
			"legend":  map[string]interface{}{"displayMode": "table", "placement": "bottom"},
			"tooltip": map[string]interface{}{"mode": "all", "sort": "desc"},
		}
	case "stat":
		return map[string]interface{}{
			"legend":  map[string]interface{}{"displayMode": "hidden"},
			"tooltip": map[string]interface{}{"mode": "single"},
		}
	case "gauge", "barGauge":
		return map[string]interface{}{
			"legend": map[string]interface{}{"displayMode": "hidden"},
		}
	}
	return map[string]interface{}{}
}

func buildCustom(spec PanelSpec) map[string]interface{} {
	switch spec.Type {
	case "timeseries":
		c := map[string]interface{}{
			"drawStyle":         "lines",
			"lineInterpolation": "smooth",
			"lineWidth":         2,
			"fillOpacity":       0.2,
			"gradientMode":      "none",
			"showPoints":        "none",
			"scaleDistribution": map[string]interface{}{"type": "linear"},
		}
		if spec.Stack {
			c["stack"] = "normal"
		} else {
			c["stack"] = "off"
		}
		return c
	case "stat":
		return map[string]interface{}{
			"textMode":  "valueAndName",
			"colorMode": "value",
			"calc":      "lastNotNull",
			"colSpan":   1,
			"textSize":  map[string]interface{}{},
		}
	case "gauge":
		return map[string]interface{}{
			"calc":     "lastNotNull",
			"min":      0,
			"max":      100,
			"textSize": map[string]interface{}{},
		}
	case "barGauge":
		return map[string]interface{}{
			"calc":        "lastNotNull",
			"displayMode": "basic",
			"orientation": "horizontal",
			"textSize":    map[string]interface{}{},
		}
	case "pie":
		return map[string]interface{}{
			"calc": "lastNotNull",
			// FE reads custom.legengPosition (note the spelling — types.ts /
			// Renderer/Pie/index.tsx); "legentPosition" silently dropped the
			// intended bottom placement on both created and type-converted pies.
			"legengPosition": "bottom",
			"detailUrl":      "",
		}
	case "table":
		return map[string]interface{}{
			"displayMode": "labelsOfSeriesToRows",
			"showHeader":  true,
			"calc":        "lastNotNull",
			"colorMode":   "value",
		}
	case "text":
		return map[string]interface{}{
			"content": spec.Desc,
		}
	default:
		return map[string]interface{}{}
	}
}

// defaultSize 返回面板类型的默认尺寸
func defaultSize(panelType string, specW, specH int) (int, int) {
	if specW > 0 && specH > 0 {
		return specW, specH
	}

	w, h := 12, 8 // 默认半宽
	switch panelType {
	case "stat":
		w, h = 6, 4
	case "gauge":
		w, h = 6, 6
	case "barGauge":
		w, h = 8, 8
	case "pie":
		w, h = 6, 6
	case "table":
		w, h = 12, 10
	case "text":
		w, h = 6, 4
	case "row":
		w, h = 24, 1
	}

	if specW > 0 {
		w = specW
	}
	if specH > 0 {
		h = specH
	}
	return w, h
}
