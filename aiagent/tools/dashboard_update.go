package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

func init() {
	register(defs.UpdateDashboard, updateDashboard)
}

// ============================================================================
// Read-side: compact config summary + variable lint
//
// get_dashboard_detail(include_config=true) surfaces these so the agent can
// show a "before → after" diff in chat without dumping the full (potentially
// huge) board payload into the LLM context.
// ============================================================================

// variableSummary is the compact, LLM-facing view of a dashboard variable.
type variableSummary struct {
	Name            string      `json:"name"`
	Type            string      `json:"type,omitempty"`
	Label           string      `json:"label,omitempty"`
	Definition      string      `json:"definition,omitempty"`
	Multi           interface{} `json:"multi,omitempty"`
	DefaultValue    interface{} `json:"default_value,omitempty"`
	DatasourceValue interface{} `json:"datasource_value,omitempty"`
}

// querySummary is the compact view of one curve (target) inside a panel. The
// optional fields are pointers so the "before" snapshot can distinguish "not
// set" (nil → omitted) from an explicit value — update_dashboard advertises
// editing instant/step/hide, so a curve carrying instant=false / hide=true /
// step=30 must show it, or the "改动前 → 改动后" diff the user approves is wrong
// for the very fields being changed.
type querySummary struct {
	Ref     string `json:"ref,omitempty"`
	PromQL  string `json:"promql,omitempty"`
	Legend  string `json:"legend,omitempty"`
	Instant *bool  `json:"instant,omitempty"`
	Step    *int   `json:"step,omitempty"`
	Hide    *bool  `json:"hide,omitempty"`
}

// panelSummary is the compact, LLM-facing view of a dashboard panel. Row panels
// are included (type=row) so the agent can see structure, but carry no queries.
type panelSummary struct {
	Id      string         `json:"id,omitempty"`
	Name    string         `json:"name,omitempty"`
	Type    string         `json:"type,omitempty"`
	Unit    string         `json:"unit,omitempty"`
	Queries []querySummary `json:"queries,omitempty"`
}

// summarizeConfigs walks a decoded board payload and returns compact variable
// and panel summaries. Panels nested inside collapsed rows are flattened in so
// the agent sees every chart it might be asked to edit.
func summarizeConfigs(configs map[string]interface{}) ([]variableSummary, []panelSummary) {
	vars := make([]variableSummary, 0)
	if rawVars, ok := configs["var"].([]interface{}); ok {
		for _, v := range rawVars {
			vm, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			vs := variableSummary{
				Name:         stringVal(vm, "name"),
				Type:         stringVal(vm, "type"),
				Label:        stringVal(vm, "label"),
				Definition:   stringVal(vm, "definition"),
				Multi:        vm["multi"],
				DefaultValue: vm["defaultValue"],
			}
			if ds, ok := vm["datasource"].(map[string]interface{}); ok {
				vs.DatasourceValue = ds["value"]
			}
			vars = append(vars, vs)
		}
	}

	panels := make([]panelSummary, 0)
	if rawPanels, ok := configs["panels"].([]interface{}); ok {
		collectPanelSummaries(rawPanels, &panels)
	}
	return vars, panels
}

func collectPanelSummaries(panels []interface{}, out *[]panelSummary) {
	for _, p := range panels {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		ps := panelSummary{
			Id:   stringVal(pm, "id"),
			Name: stringVal(pm, "name"),
			Type: stringVal(pm, "type"),
			Unit: panelUnit(pm),
		}
		if targets, ok := pm["targets"].([]interface{}); ok {
			for _, t := range targets {
				tm, ok := t.(map[string]interface{})
				if !ok {
					continue
				}
				q := querySummary{
					Ref:    stringVal(tm, "refId"),
					PromQL: stringVal(tm, "expr"),
					Legend: targetLegend(tm),
				}
				if b, ok := tm["instant"].(bool); ok {
					q.Instant = &b
				}
				if b, ok := tm["hide"].(bool); ok {
					q.Hide = &b
				}
				// step is a JSON number → float64 after decode.
				if f, ok := tm["step"].(float64); ok {
					n := int(f)
					q.Step = &n
				}
				ps.Queries = append(ps.Queries, q)
			}
		}
		*out = append(*out, ps)
		// Collapsed rows carry their child charts in a nested "panels" array.
		if nested, ok := pm["panels"].([]interface{}); ok && len(nested) > 0 {
			collectPanelSummaries(nested, out)
		}
	}
}

// targetLegend reads a target's legend template. n9e's canonical key is
// "legend" (what the FE editor, renderer, and built-in boards persist, and
// what buildTargets writes); older or Grafana-imported targets may instead
// carry "legendFormat", so fall back to it. Mirrors the write side in
// buildTargets (see its comment).
func targetLegend(tm map[string]interface{}) string {
	if s := stringVal(tm, "legend"); s != "" {
		return s
	}
	return stringVal(tm, "legendFormat")
}

// panelUnit extracts the panel unit from options.standardOptions. Current n9e
// panels (schema version >= 3.3.0) store it under "unit"; older exports use the
// legacy "util" key, which the FE migrates to "unit" on load. We read "unit"
// first and fall back to "util" so the diff's "before" value is correct for both.
func panelUnit(pm map[string]interface{}) string {
	opts, ok := pm["options"].(map[string]interface{})
	if !ok {
		return ""
	}
	so, ok := opts["standardOptions"].(map[string]interface{})
	if !ok {
		return ""
	}
	if u, ok := so["unit"].(string); ok && u != "" {
		return u
	}
	if u, ok := so["util"].(string); ok {
		return u
	}
	return ""
}

// varRefRe matches dashboard variable references in PromQL / legends /
// definitions: $name or ${name}. The captured group is the bare name.
var varRefRe = regexp.MustCompile(`\$\{?([a-zA-Z_][a-zA-Z0-9_]*)\}?`)

// lintVariables scans the board for common variable smells and returns a list
// of human-readable issues (in Chinese, matching the rest of the AI surface):
//   - panel queries / legends referencing an undefined variable
//   - a variable's own definition referencing an undefined variable
//
// The agent uses this as the "check" half of "检查并修复变量"; the actual fix is
// applied via update_dashboard (rename/redefine the var, or fix_datasource).
func lintVariables(configs map[string]interface{}) []string {
	defined := map[string]bool{}
	if rawVars, ok := configs["var"].([]interface{}); ok {
		for _, v := range rawVars {
			if vm, ok := v.(map[string]interface{}); ok {
				if name := stringVal(vm, "name"); name != "" {
					defined[name] = true
				}
			}
		}
	}

	// undefinedRefs returns the set of referenced-but-undefined variable names
	// in a string, ignoring built-in template vars (__interval, __range, ...).
	undefinedRefs := func(s string) []string {
		var out []string
		seen := map[string]bool{}
		for _, m := range varRefRe.FindAllStringSubmatch(s, -1) {
			name := m[1]
			if strings.HasPrefix(name, "__") || defined[name] || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
		return out
	}

	issues := make([]string, 0)

	// Variable definitions referencing undefined vars (broken cascading vars).
	if rawVars, ok := configs["var"].([]interface{}); ok {
		for _, v := range rawVars {
			vm, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			name := stringVal(vm, "name")
			for _, ref := range undefinedRefs(stringVal(vm, "definition")) {
				issues = append(issues, fmt.Sprintf("变量 %q 的取值表达式引用了未定义的变量 $%s", name, ref))
			}
		}
	}

	// Panel queries / legends referencing undefined vars.
	if rawPanels, ok := configs["panels"].([]interface{}); ok {
		lintPanelRefs(rawPanels, undefinedRefs, &issues)
	}

	return issues
}

func lintPanelRefs(panels []interface{}, undefinedRefs func(string) []string, issues *[]string) {
	for _, p := range panels {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		pname := stringVal(pm, "name")
		if targets, ok := pm["targets"].([]interface{}); ok {
			for _, t := range targets {
				tm, ok := t.(map[string]interface{})
				if !ok {
					continue
				}
				for _, ref := range undefinedRefs(stringVal(tm, "expr")) {
					*issues = append(*issues, fmt.Sprintf("图表 %q 的查询表达式引用了未定义的变量 $%s", pname, ref))
				}
				// Read via targetLegend so the lint matches what the FE/builder
				// actually persist: the canonical "legend" key (with the historical
				// "legendFormat" as fallback). Reading "legendFormat" alone would
				// miss undefined refs in every real board's legend.
				for _, ref := range undefinedRefs(targetLegend(tm)) {
					*issues = append(*issues, fmt.Sprintf("图表 %q 的 legend 引用了未定义的变量 $%s", pname, ref))
				}
			}
		}
		if nested, ok := pm["panels"].([]interface{}); ok && len(nested) > 0 {
			lintPanelRefs(nested, undefinedRefs, issues)
		}
	}
}

// ============================================================================
// Write-side: surgical patch application
// ============================================================================

// variablePatch is one entry of update_dashboard's `variables` argument. Pointer
// fields distinguish "not provided" (nil → keep existing) from "set to zero".
type variablePatch struct {
	Name         string  `json:"name"`
	Delete       bool    `json:"delete"`
	Label        *string `json:"label"`
	Definition   *string `json:"definition"`
	Multi        *bool   `json:"multi"`
	DefaultValue *string `json:"default_value"`
	Type         *string `json:"type"`
}

// UnmarshalJSON decodes a variablePatch tolerantly so a string-form multi/delete
// ("multi":"true") from the LLM doesn't abort the whole `variables` parse.
func (v *variablePatch) UnmarshalJSON(data []byte) error {
	var r struct {
		Name         string          `json:"name"`
		Label        *string         `json:"label"`
		Definition   *string         `json:"definition"`
		DefaultValue *string         `json:"default_value"`
		Type         *string         `json:"type"`
		Multi        json.RawMessage `json:"multi"`
		Delete       json.RawMessage `json:"delete"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	v.Name, v.Label, v.Definition, v.DefaultValue, v.Type = r.Name, r.Label, r.Definition, r.DefaultValue, r.Type

	var err error
	if v.Multi, err = flexBoolPtr(r.Multi); err != nil {
		return fmt.Errorf("variable field multi: %w", err)
	}
	if v.Delete, err = flexBool(r.Delete); err != nil {
		return fmt.Errorf("variable field delete: %w", err)
	}
	return nil
}

// panelPatch is one entry of update_dashboard's `panels` argument. Queries is a
// pointer-to-slice so an omitted field (nil) leaves the curves untouched, while
// a non-nil array is merged onto the existing targets by mergeTargets (ref-keyed
// edits + no-ref additions), NOT a wholesale replace.
type panelPatch struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Delete  bool         `json:"delete"`
	NewName *string      `json:"new_name"`
	Unit    *string      `json:"unit"`
	Desc    *string      `json:"description"`
	Type    *string      `json:"type"`
	Queries *[]QuerySpec `json:"queries"`
}

// UnmarshalJSON decodes a panelPatch tolerantly so a string-form delete
// ("delete":"true") doesn't abort the whole `panels` parse. Queries decode via
// QuerySpec's own tolerant UnmarshalJSON.
func (p *panelPatch) UnmarshalJSON(data []byte) error {
	var r struct {
		ID      string          `json:"id"`
		Name    string          `json:"name"`
		NewName *string         `json:"new_name"`
		Unit    *string         `json:"unit"`
		Desc    *string         `json:"description"`
		Type    *string         `json:"type"`
		Queries *[]QuerySpec    `json:"queries"`
		Delete  json.RawMessage `json:"delete"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	p.ID, p.Name, p.NewName, p.Unit, p.Desc, p.Type, p.Queries = r.ID, r.Name, r.NewName, r.Unit, r.Desc, r.Type, r.Queries

	var err error
	if p.Delete, err = flexBool(r.Delete); err != nil {
		return fmt.Errorf("panel field delete: %w", err)
	}
	return nil
}

// noEditableField reports whether a non-delete panel patch carries nothing to
// apply. The tolerant decoder above silently drops unknown keys, so a patch
// like {"id":"panel-0","fontSize":20} would otherwise sail through as a
// confirmable "更新图表" that writes back an unchanged payload — the agent then
// announces an edit that never happened.
func (p *panelPatch) noEditableField() bool {
	return !p.Delete && p.NewName == nil && p.Unit == nil && p.Desc == nil && p.Type == nil && p.Queries == nil
}

// applyVariablePatches mutates configs["var"] per the patches and returns a list
// of human-readable change descriptions. Patches match existing variables by
// name; an unmatched, non-delete patch adds a new query variable.
func applyVariablePatches(lang string, configs map[string]interface{}, patches []variablePatch) ([]string, error) {
	changes := make([]string, 0)
	vars, _ := configs["var"].([]interface{})

	for _, p := range patches {
		if strings.TrimSpace(p.Name) == "" {
			return nil, fmt.Errorf("each variable patch requires a non-empty name")
		}

		idx := -1
		for i, v := range vars {
			if vm, ok := v.(map[string]interface{}); ok && stringVal(vm, "name") == p.Name {
				idx = i
				break
			}
		}

		switch {
		case p.Delete:
			if idx < 0 {
				return nil, fmt.Errorf("cannot delete variable %q: not found", p.Name)
			}
			vars = append(vars[:idx], vars[idx+1:]...)
			changes = append(changes, fmt.Sprintf(aiagent.LangText(lang, "删除变量 %q", "delete variable %q"), p.Name))

		case idx >= 0:
			// Same lie-prevention as panel patches: unknown fields were dropped
			// by the decoder, so an all-nil patch must error instead of being
			// announced as a (no-op) "更新变量".
			if p.Label == nil && p.Definition == nil && p.Multi == nil && p.DefaultValue == nil && p.Type == nil {
				return nil, fmt.Errorf("variable patch for %q has no editable field (unknown fields are dropped): supported fields are label / definition / multi / default_value / type / delete", p.Name)
			}
			// Mirror the add-path restriction below: flipping an existing
			// variable to a structural type like datasource leaves a malformed
			// var (stale label_values definition, no datasource plugin object)
			// that silently breaks every panel referencing it.
			if p.Type != nil {
				switch *p.Type {
				case "query", "custom", "textbox", "constant":
				default:
					return nil, fmt.Errorf("cannot change variable %q to type %q: only query / custom / textbox / constant are supported", p.Name, *p.Type)
				}
			}
			vm := vars[idx].(map[string]interface{})
			applyVarFields(vm, p)
			changes = append(changes, fmt.Sprintf(aiagent.LangText(lang, "更新变量 %q", "update variable %q"), p.Name))

		default: // add new variable
			// buildVariable 只会构造 query 型骨架（definition/datasource 等字段）。
			// 放任 type 覆盖会产出字段布局错乱的畸形变量（如 type=datasource 却
			// 自带 datasource 子对象自指），直接拒绝，让模型改走其他途径。
			if p.Type != nil && *p.Type != "query" {
				return nil, fmt.Errorf("cannot add variable %q: only query-type variables can be added (got type %q)", p.Name, *p.Type)
			}
			// Same lie-prevention as the update branch above: a name that matched
			// nothing plus no definition almost always means a typo'd name whose
			// real fields were dropped by the tolerant decoder. Without this guard
			// we would silently create a junk query variable with an empty
			// definition (which can never resolve values) and announce "新增变量"
			// for what the model believed was an edit.
			if p.Definition == nil {
				return nil, fmt.Errorf("variable %q does not match any existing variable; adding a new query variable requires definition (if you meant to edit an existing variable, check the name)", p.Name)
			}
			spec := VariableSpec{Name: p.Name}
			if p.Definition != nil {
				spec.Definition = *p.Definition
			}
			if p.Label != nil {
				spec.Label = *p.Label
			}
			if p.Multi != nil {
				spec.Multi = p.Multi
			}
			nv := buildVariable(spec)
			// buildVariable hard-codes the create-time datasource ref "${prom}",
			// but most existing boards name their datasource variable differently
			// (datasource / DS_PROMETHEUS / ...). Repoint the new query variable at
			// THIS board's actual datasource variable so it resolves a datasource
			// instead of dangling on a non-existent ${prom} and silently returning
			// no values.
			if ds, ok := nv["datasource"].(map[string]interface{}); ok {
				ds["value"] = boardDatasourceVarRef(configs)
			}
			if p.DefaultValue != nil {
				nv["defaultValue"] = *p.DefaultValue
			}
			vars = append(vars, nv)
			changes = append(changes, fmt.Sprintf(aiagent.LangText(lang, "新增变量 %q", "add variable %q"), p.Name))
		}
	}

	configs["var"] = vars
	return changes, nil
}

// boardDatasourceVarRef returns the "${name}" template reference for the
// board's datasource-type variable, so a newly-added query variable follows the
// SAME datasource selector the board already uses. Falls back to the builder
// default ("${prom}") only when the board has no datasource variable at all.
func boardDatasourceVarRef(configs map[string]interface{}) string {
	if rawVars, ok := configs["var"].([]interface{}); ok {
		for _, v := range rawVars {
			vm, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			if vm["type"] == "datasource" {
				if name := stringVal(vm, "name"); name != "" {
					return "${" + name + "}"
				}
			}
		}
	}
	return datasourceVarRef
}

func applyVarFields(vm map[string]interface{}, p variablePatch) {
	if p.Label != nil {
		vm["label"] = *p.Label
	}
	if p.Definition != nil {
		vm["definition"] = *p.Definition
	}
	if p.Multi != nil {
		vm["multi"] = *p.Multi
	}
	if p.DefaultValue != nil {
		vm["defaultValue"] = *p.DefaultValue
	}
	if p.Type != nil {
		vm["type"] = *p.Type
	}
}

// applyPanelPatches mutates configs["panels"] per the patches (recursing into
// collapsed rows) and returns human-readable change descriptions. Each patch
// locates its target by id (preferred) or current name.
func applyPanelPatches(lang string, configs map[string]interface{}, patches []panelPatch) ([]string, error) {
	changes := make([]string, 0)
	panels, _ := configs["panels"].([]interface{})

	for _, p := range patches {
		if strings.TrimSpace(p.ID) == "" && strings.TrimSpace(p.Name) == "" {
			return nil, fmt.Errorf("each panel patch requires an id or a name to locate the target panel")
		}
		label := p.ID
		if label == "" {
			label = p.Name
		}

		// Reject patches that carry nothing applicable BEFORE reporting them as
		// changes: unknown fields were silently dropped by the tolerant decoder,
		// so an "empty" patch almost always means the model tried to edit an
		// unsupported field — fail loudly so it can tell the user instead of
		// announcing a no-op write as success.
		if p.noEditableField() {
			return nil, fmt.Errorf("panel patch for %q has no editable field (unknown fields are dropped): supported fields are new_name / unit / description / type / queries / delete", label)
		}

		// Ambiguity guard for name-based locators. Panel names can repeat within
		// a board (and across collapsed rows), so a bare name may match >1 panel.
		// findPanel/deletePanel return the FIRST match, so without this guard a
		// name-based update/delete would silently mutate or drop the wrong chart.
		// id is unique per panel, so only the id-less (name) path needs the check.
		if strings.TrimSpace(p.ID) == "" {
			switch n := countPanels(panels, "", p.Name); {
			case n == 0:
				return nil, fmt.Errorf("cannot locate panel %q: no panel with that name", p.Name)
			case n > 1:
				return nil, fmt.Errorf("panel name %q is ambiguous: %d panels share it; pass the panel id instead (get_dashboard_detail with include_config=true lists each panel's id)", p.Name, n)
			}
		}

		if p.Delete {
			var deleted bool
			panels, deleted = deletePanel(panels, p.ID, p.Name)
			if !deleted {
				return nil, fmt.Errorf("cannot delete panel %q: not found", label)
			}
			changes = append(changes, fmt.Sprintf(aiagent.LangText(lang, "删除图表 %q", "delete panel %q"), label))
			continue
		}

		pm := findPanel(panels, p.ID, p.Name)
		if pm == nil {
			return nil, fmt.Errorf("cannot update panel %q: not found", label)
		}
		// buildTargets emits the Prometheus expr/refId/legend target schema.
		// Replacing queries on a SQL/log panel (mysql, ck, es, ...) would
		// clobber its bespoke query structure, datasource fields, and params,
		// so refuse query edits on non-Prometheus panels.
		if p.Queries != nil {
			if cate := stringVal(pm, "datasourceCate"); !promLikeCate(cate) {
				return nil, fmt.Errorf("cannot update queries on panel %q: query editing only supports Prometheus/VictoriaMetrics panels, but this panel uses datasource type %q", label, cate)
			}
		}
		// Models routinely echo the panel's current type alongside real edits
		// (read config → patch back), so a same-type re-statement is a no-op
		// to drop, not a reason to reject the queries/unit/new_name riding in
		// the same patch. Only when type is the patch's ONLY field does it
		// stay an error — dropping it there would announce a no-op write as a
		// successful edit. Compare against the EFFECTIVE type so a type-less
		// panel (the FE renders it as timeseries) restated as "timeseries" is
		// recognised as the no-op it is, instead of running a full changePanelType
		// that would wipe its authored custom render config.
		if p.Type != nil && effectivePanelType(pm) == *p.Type {
			typ := *p.Type
			p.Type = nil
			if p.noEditableField() {
				return nil, fmt.Errorf("panel %q is already type %q: nothing to change (re-sending the current type would only reset its style options)", label, typ)
			}
		}
		if p.Type != nil {
			if err := checkPanelTypeChange(pm, label, *p.Type); err != nil {
				return nil, err
			}
		}
		parts := applyPanelFields(lang, pm, p)
		changes = append(changes, fmt.Sprintf(aiagent.LangText(lang, "更新图表 %q（%s）", "update panel %q (%s)"),
			label, strings.Join(parts, aiagent.LangText(lang, "、", ", "))))
	}

	configs["panels"] = panels
	return changes, nil
}

// updatablePanelTypes are the chart types a panel can be switched to via
// update_dashboard — the types buildCustom/typeOptions know how to emit
// defaults for. "row" is a layout container and "text" keeps its content in
// custom.content, so converting a query panel to/from either would strand its
// config rather than re-render it.
var updatablePanelTypes = map[string]bool{
	"timeseries": true, "stat": true, "gauge": true, "barGauge": true, "pie": true, "table": true,
}

// effectivePanelType is the type the FE actually renders a panel as. A panel
// with no "type" key renders as timeseries, EXCEPT one carrying authored
// custom.content (markdown), which renders as text — changePanelType
// wholesale-replaces custom, so that shape must be treated as text (and refused)
// rather than silently converted.
func effectivePanelType(pm map[string]interface{}) string {
	if t := stringVal(pm, "type"); t != "" {
		return t
	}
	if custom, ok := pm["custom"].(map[string]interface{}); ok {
		if s, _ := custom["content"].(string); strings.TrimSpace(s) != "" {
			return "text"
		}
	}
	return "timeseries"
}

func checkPanelTypeChange(pm map[string]interface{}, label, newType string) error {
	cur := effectivePanelType(pm)
	switch {
	case !updatablePanelTypes[newType]:
		return fmt.Errorf("cannot change panel %q to type %q: supported chart types are timeseries / stat / gauge / barGauge / pie / table", label, newType)
	case cur == "row":
		return fmt.Errorf("cannot change type of %q: it is a layout row, not a chart", label)
	case !updatablePanelTypes[cur]:
		// changePanelType wholesale-replaces `custom`: on a text panel that
		// would destroy custom.content (the authored markdown) with no way
		// back, since non-chart types are not valid conversion targets.
		return fmt.Errorf("cannot change type of %q: it is a %q panel whose config (e.g. a text panel's content) would be destroyed by the conversion; only chart panels (timeseries / stat / gauge / barGauge / pie / table) can switch types", label, cur)
	}
	// Same stance as the queries path above: the defaults changePanelType
	// writes (buildCustom/typeOptions) are designed and tested for Prometheus
	// chart panels, so refuse type flips on SQL/log panels too.
	if cate := stringVal(pm, "datasourceCate"); !promLikeCate(cate) {
		return fmt.Errorf("cannot change type of panel %q: type changes reset the chart render config, which only supports Prometheus/VictoriaMetrics panels, but this panel uses datasource type %q", label, cate)
	}
	return nil
}

// changePanelType flips a panel's visualization type and resets the
// type-specific render config — `custom` plus the legend/tooltip options — to
// the new type's defaults: leftover knobs from the old type (a stat panel's
// textMode/colorMode on a now-timeseries panel) would confuse the FE editor.
// standardOptions (unit/decimals), layout and overrides are type-agnostic and
// survive untouched; targets are left to the caller, which clears the instant
// flag AFTER any same-patch query merge (see applyPanelFields).
func changePanelType(pm map[string]interface{}, newType string) {
	pm["type"] = newType
	pm["custom"] = buildCustom(PanelSpec{Type: newType})
	opts, _ := pm["options"].(map[string]interface{})
	if opts == nil {
		opts = map[string]interface{}{}
	}
	delete(opts, "legend")
	delete(opts, "tooltip")
	for k, v := range typeOptions(newType) {
		opts[k] = v
	}
	pm["options"] = opts
}

// clearTargetsInstant drops the instant flag from every target. The FE picks
// instant-vs-range purely by target.instant (Renderer/datasource/prometheus.ts),
// never by panel type — a leftover instant:true from a stat/table panel would
// render a now-timeseries panel as a single dot instead of a curve. Range
// targets render fine on every type (value panels calc over the series), so this
// is applied only when converting TO timeseries.
func clearTargetsInstant(pm map[string]interface{}) {
	targets, ok := pm["targets"].([]interface{})
	if !ok {
		return
	}
	for _, t := range targets {
		if tm, ok := t.(map[string]interface{}); ok {
			delete(tm, "instant")
		}
	}
}

// applyPanelFields mutates the panel per the patch and returns one
// human-readable fragment per field actually applied — the change list shown
// at confirm time must describe what will really be written, never a bare
// "更新图表" for fields the tool dropped.
func applyPanelFields(lang string, pm map[string]interface{}, p panelPatch) []string {
	parts := make([]string, 0, 5)
	if p.NewName != nil {
		pm["name"] = *p.NewName
		parts = append(parts, fmt.Sprintf(aiagent.LangText(lang, "标题→%q", "title→%q"), *p.NewName))
	}
	if p.Desc != nil {
		pm["description"] = *p.Desc
		parts = append(parts, aiagent.LangText(lang, "说明", "description"))
	}
	if p.Unit != nil {
		setPanelUnit(pm, *p.Unit)
		parts = append(parts, fmt.Sprintf(aiagent.LangText(lang, "单位→%s", "unit→%s"), *p.Unit))
	}
	if p.Type != nil {
		parts = append(parts, fmt.Sprintf(aiagent.LangText(lang, "类型 %s→%s", "type %s→%s"), stringVal(pm, "type"), *p.Type))
		changePanelType(pm, *p.Type)
	}
	if p.Queries != nil {
		existing, _ := pm["targets"].([]interface{})
		pm["targets"] = mergeTargets(existing, *p.Queries)
		parts = append(parts, aiagent.LangText(lang, "曲线", "queries"))
	}
	// Clear instant AFTER the query merge: a →timeseries conversion must leave
	// no instant targets, but a queries patch riding in the same call re-adds
	// instant:true via mergeTargets, so the clear has to run once the final
	// targets are settled — not inside changePanelType.
	if p.Type != nil && *p.Type == "timeseries" {
		clearTargetsInstant(pm)
	}
	return parts
}

// mergeTargets produces the targets slice for an *edited* Prometheus panel by
// applying the incoming QuerySpecs on top of the panel's existing targets.
//
// It is a true incremental merge, NOT a whole-panel replace: the output starts
// from the existing targets, so any curve the caller did not mention survives
// untouched. A spec edits an existing curve ONLY when it carries a ref (refId)
// that matches an existing target; a spec with an empty ref is ALWAYS a
// brand-new curve. There is deliberately no positional/index fallback — that
// fallback used to make "add a swap curve" (a single no-ref spec) silently
// overwrite the existing curve at the same index. Concretely:
//   - a ref-matched, non-delete spec overwrites only the fields it carries (expr
//     when non-empty, and any of legend/instant/step/hide it sets), keeping the
//     rest of that target — refId, time range, __mode__, datasource, and any
//     other persisted keys — so expression queries, hidden curves, fixed steps,
//     and refId-keyed overrides/transformations keep working after the edit;
//   - a ref-matched spec with delete=true removes that curve;
//   - a spec whose ref is empty (or names a refId not present) becomes a
//     brand-new target: it keeps its requested ref when free, else gets a
//     freshly allocated refId (a delete that matches nothing is a no-op).
//
// A matched target keeps its existing refId; only brand-new targets get one
// assigned. Because unmentioned curves are preserved and no-ref specs only ever
// add, the ONLY way to drop or overwrite an existing curve is to address it by
// ref (delete=true to drop, or a ref-matched edit) — adding a curve never
// silently clobbers its panel-mates.
//
// Note: Legend is value-typed, so an edit can only set it (a non-empty legend),
// never clear it back — to fully reset a curve, delete it and re-add it. Instant,
// Step, and Hide are pointers and so can be set independently (instant true↔false
// included); nil leaves the existing value untouched. expr is likewise only
// overwritten when the spec carries a non-empty promql, so a {ref,hide:true}
// patch keeps the curve's query intact (see applyQuerySpec).
func mergeTargets(existing []interface{}, specs []QuerySpec) []interface{} {
	// Base = the existing targets, cloned so edits don't alias the board payload.
	// We build from existing (not from specs) so unmentioned curves are kept.
	out := make([]interface{}, len(existing))
	refToPos := make(map[string]int, len(existing))
	used := make(map[string]bool, len(existing))
	for i, t := range existing {
		tm, ok := t.(map[string]interface{})
		if !ok {
			out[i] = t // preserve any non-object entry untouched
			continue
		}
		c := cloneTarget(tm)
		out[i] = c
		if ref := stringVal(c, "refId"); ref != "" {
			refToPos[ref] = i
			used[ref] = true
		}
	}

	dropped := make(map[int]bool) // positions removed by an explicit delete spec

	for _, q := range specs {
		// A spec edits an existing curve ONLY when its ref matches an existing
		// target. An empty ref is never matched positionally — it always adds a
		// new curve below, so "add a curve" can't overwrite an existing one.
		pos := -1
		if q.Ref != "" {
			if p, ok := refToPos[q.Ref]; ok {
				pos = p
			}
		}

		if pos >= 0 {
			if q.Delete {
				dropped[pos] = true
				continue
			}
			applyQuerySpec(out[pos].(map[string]interface{}), q, used)
			continue
		}

		// No existing curve matched (empty ref, or a ref not present). A delete
		// here is a no-op; otherwise this spec adds a brand-new curve.
		if q.Delete {
			continue
		}
		nt := map[string]interface{}{}
		applyQuerySpec(nt, q, used)
		out = append(out, nt)
	}

	if len(dropped) == 0 {
		return out
	}
	kept := make([]interface{}, 0, len(out))
	for i, t := range out {
		if dropped[i] {
			continue
		}
		kept = append(kept, t)
	}
	return kept
}

// applyQuerySpec writes a QuerySpec's fields onto a target map (an existing
// curve being edited, or a freshly created one). expr is written only when the
// spec carries a non-empty promql: PromQL is a value-typed string, so an empty
// value can't be told apart from "not provided", and unconditionally writing it
// would blank out an existing curve's query on a fields-only patch (e.g.
// {ref:"A",hide:true}). legend is likewise only set, never cleared;
// instant/step/hide are honored when their pointer is non-nil (instant can be
// flipped true↔false). A target with no refId
// yet (a brand-new curve) gets the spec's ref when free, else the next free
// letter; an existing target keeps its refId.
func applyQuerySpec(t map[string]interface{}, q QuerySpec, used map[string]bool) {
	if q.PromQL != "" {
		t["expr"] = q.PromQL
	}

	if stringVal(t, "refId") == "" {
		ref := q.Ref
		if ref == "" || used[ref] {
			ref = nextRefId(used)
		}
		t["refId"] = ref
		used[ref] = true
	}

	if q.Legend != "" {
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
}

// cloneTarget returns a shallow copy of a target map (nil → fresh empty map) so
// merge edits don't alias the original board payload's target objects.
func cloneTarget(t map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(t)+1)
	for k, v := range t {
		out[k] = v
	}
	return out
}

// nextRefId returns the first refId not in used, preferring single letters
// A..Z and falling back to Q0, Q1, ... once the alphabet is exhausted. The
// caller is responsible for marking the returned id used.
func nextRefId(used map[string]bool) string {
	for i := 0; i < 26; i++ {
		r := string(rune('A' + i))
		if !used[r] {
			return r
		}
	}
	for i := 0; ; i++ {
		r := fmt.Sprintf("Q%d", i)
		if !used[r] {
			return r
		}
	}
}

// setPanelUnit writes options.standardOptions.unit, creating the nested maps if
// the panel doesn't have them yet. Current n9e panels (schema version >= 3.3.0)
// read the unit from "unit"; the legacy "util" key is only migrated to "unit"
// for panels older than 3.3.0. We write "unit" so the change takes effect on the
// live panel, and strip any stale "util" so a pre-3.3.0 panel's load-time
// migration (which copies util→unit) can't clobber the value we just set.
func setPanelUnit(pm map[string]interface{}, unit string) {
	opts, ok := pm["options"].(map[string]interface{})
	if !ok {
		opts = map[string]interface{}{}
		pm["options"] = opts
	}
	so, ok := opts["standardOptions"].(map[string]interface{})
	if !ok {
		so = map[string]interface{}{}
		opts["standardOptions"] = so
	}
	so["unit"] = unit
	delete(so, "util")
}

// prometheusLikeDashboard reports whether a board's datasource config is safe to
// run through normalizeTemplateDatasource (which is Prometheus-centric). It scans
// datasource-type variables' definitions and every panel's datasourceCate; the
// board is "Prometheus-like" only if no non-Prometheus/VictoriaMetrics cate is
// found. When it returns false, the second value is a sample of the offending
// cate for the error message.
func prometheusLikeDashboard(configs map[string]interface{}) (string, bool) {
	if rawVars, ok := configs["var"].([]interface{}); ok {
		for _, v := range rawVars {
			vm, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			if vm["type"] != "datasource" {
				continue
			}
			if def := stringVal(vm, "definition"); !promLikeCate(def) {
				return def, false
			}
		}
	}

	if rawPanels, ok := configs["panels"].([]interface{}); ok {
		if cate, ok := panelsDatasourceCate(rawPanels); !ok {
			return cate, false
		}
	}

	return "", true
}

// promLikeCate reports whether a datasourceCate (or datasource-variable
// definition) names a Prometheus-style metrics source whose targets use the
// expr/refId/legend schema that buildTargets emits. An empty cate is treated as
// Prometheus (the historical default). SQL/log sources (mysql, ck, es, ...)
// return false so callers can refuse to overwrite their bespoke target schema.
func promLikeCate(cate string) bool {
	switch cate {
	case "", "prometheus", "victoriametrics":
		return true
	}
	return false
}

// panelsDatasourceCate walks panels (recursing into collapsed rows) and returns
// the first non-Prometheus-like datasourceCate, or ("", true) if all panels are
// Prometheus-like.
func panelsDatasourceCate(panels []interface{}) (string, bool) {
	for _, p := range panels {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if cate := stringVal(pm, "datasourceCate"); !promLikeCate(cate) {
			return cate, false
		}
		if nested, ok := pm["panels"].([]interface{}); ok && len(nested) > 0 {
			if cate, ok := panelsDatasourceCate(nested); !ok {
				return cate, false
			}
		}
	}
	return "", true
}

// findPanel returns the panel map matching id (preferred) or name, recursing
// into nested row panels. Returns nil if not found.
func findPanel(panels []interface{}, id, name string) map[string]interface{} {
	for _, p := range panels {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if panelMatches(pm, id, name) {
			return pm
		}
		if nested, ok := pm["panels"].([]interface{}); ok {
			if found := findPanel(nested, id, name); found != nil {
				return found
			}
		}
	}
	return nil
}

// countPanels counts how many panels match the locator across the whole tree
// (recursing into nested row panels). Used to reject ambiguous name-based
// locators before findPanel/deletePanel silently act on the first match.
func countPanels(panels []interface{}, id, name string) int {
	n := 0
	for _, p := range panels {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if panelMatches(pm, id, name) {
			n++
		}
		if nested, ok := pm["panels"].([]interface{}); ok {
			n += countPanels(nested, id, name)
		}
	}
	return n
}

// deletePanel removes the panel matching id/name (recursing into rows) and
// returns the rebuilt slice plus whether a panel was removed.
func deletePanel(panels []interface{}, id, name string) ([]interface{}, bool) {
	out := make([]interface{}, 0, len(panels))
	deleted := false
	for _, p := range panels {
		pm, ok := p.(map[string]interface{})
		if !ok {
			out = append(out, p)
			continue
		}
		if !deleted && panelMatches(pm, id, name) {
			deleted = true
			continue
		}
		// Recurse into nested row panels only while nothing has been deleted yet.
		// Without this guard, a name-based delete could remove a top-level panel
		// AND a same-named panel inside a later row in one call (names can repeat),
		// silently dropping more than the single panel applyPanelPatches reports.
		if !deleted {
			if nested, ok := pm["panels"].([]interface{}); ok && len(nested) > 0 {
				var childDeleted bool
				nested, childDeleted = deletePanel(nested, id, name)
				if childDeleted {
					pm["panels"] = nested
					deleted = true
				}
			}
		}
		out = append(out, pm)
	}
	return out, deleted
}

// panelMatches reports whether a panel matches the locator. id takes precedence;
// when id is empty, the current name is matched.
func panelMatches(pm map[string]interface{}, id, name string) bool {
	if id != "" {
		return stringVal(pm, "id") == id
	}
	return name != "" && stringVal(pm, "name") == name
}

// ============================================================================
// Handler
// ============================================================================

func updateDashboard(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	// Editing an existing dashboard's payload is the FE /dashboards/put route
	// (boardPutConfigs), not /dashboards/add — using add would let an
	// add-only user mutate existing boards while rejecting edit-only users.
	// Plus rw on the owning busi group below.
	if err := checkPerm(deps, user, PermDashboardsPut); err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	board, err := models.BoardGet(deps.DBCtx, "id = ?", id)
	if err != nil {
		return "", fmt.Errorf("failed to get dashboard: %v", err)
	}
	if board == nil {
		return fmt.Sprintf(`{"error":"dashboard not found: id=%d"}`, id), nil
	}

	bg, err := models.BusiGroupGetById(deps.DBCtx, board.GroupId)
	if err != nil {
		return "", fmt.Errorf("failed to get busi group: %v", err)
	}
	if bg == nil {
		return "", fmt.Errorf("busi group not found: id=%d", board.GroupId)
	}
	if err := checkBgRW(deps, user, bg); err != nil {
		return "", err
	}

	// Two-phase write: a confirm call (proposal_id + confirmed=true) applies a
	// proposal stashed by an earlier propose call; everything below this branch
	// is the propose phase, which computes the change set but never writes.
	confirmed := getArgBool(args, "confirmed")
	proposalID := getArgString(args, "proposal_id")
	if confirmed || proposalID != "" {
		return confirmDashboardProposal(ctx, deps, params, user, board, id, proposalID, confirmed)
	}

	if strings.TrimSpace(board.Configs) == "" {
		return "", fmt.Errorf("dashboard id=%d has no config payload to modify", id)
	}

	var configs map[string]interface{}
	if err := json.Unmarshal([]byte(board.Configs), &configs); err != nil {
		return "", fmt.Errorf("invalid dashboard config payload: %v", err)
	}

	// Canonical snapshot of the UNpatched config, taken before any patch
	// mutates `configs` in place. Compared against the patched marshal below:
	// patches that only restate current values would otherwise produce a
	// confirmable "change set" whose write is byte-identical to what's already
	// stored — the user confirms, the agent announces success, nothing changed.
	baselineCanon, err := json.Marshal(configs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal current config: %v", err)
	}

	allChanges := make([]string, 0)

	if varsJSON := getArgString(args, "variables"); varsJSON != "" {
		var patches []variablePatch
		if err := json.Unmarshal([]byte(varsJSON), &patches); err != nil {
			return "", fmt.Errorf("invalid variables JSON: %v", err)
		}
		changes, err := applyVariablePatches(params["lang"], configs, patches)
		if err != nil {
			return "", err
		}
		allChanges = append(allChanges, changes...)
	}

	if panelsJSON := getArgString(args, "panels"); panelsJSON != "" {
		var patches []panelPatch
		if err := json.Unmarshal([]byte(panelsJSON), &patches); err != nil {
			return "", fmt.Errorf("invalid panels JSON: %v", err)
		}
		changes, err := applyPanelPatches(params["lang"], configs, patches)
		if err != nil {
			return "", err
		}
		allChanges = append(allChanges, changes...)
	}

	// getArgBool (not a raw .(bool)) so a string-form "true" from the LLM still
	// triggers the repair instead of silently no-op'ing into "nothing to change".
	if getArgBool(args, "fix_datasource") {
		// normalizeTemplateDatasource is Prometheus-centric: it rewrites every
		// datasource variable's definition to "prometheus", and injects a
		// Prometheus var when none exists. Running it on a MySQL/ClickHouse/ES
		// board would silently corrupt the datasource config, so refuse unless
		// the board is actually Prometheus/VictoriaMetrics.
		if cate, ok := prometheusLikeDashboard(configs); !ok {
			return "", fmt.Errorf("fix_datasource only supports Prometheus/VictoriaMetrics dashboards, but this dashboard uses datasource type %q; aborting to avoid corrupting its datasource config", cate)
		}
		normalizeTemplateDatasource(configs, 0)
		allChanges = append(allChanges, aiagent.LangText(params["lang"],
			"修复数据源引用（统一重指到大盘数据源变量）",
			"fix datasource references (repoint to the board's datasource variable)"))
	}

	if len(allChanges) == 0 {
		return "", fmt.Errorf("nothing to change: pass variables, panels, or fix_datasource=true")
	}

	newConfigs, err := json.Marshal(configs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal updated config: %v", err)
	}

	// No-diff backstop: refuse to propose a write that changes nothing. Don't
	// re-submit the same call — tell the user the config already matches.
	if string(newConfigs) == string(baselineCanon) {
		return "", fmt.Errorf("no-op update: the requested changes produce a config identical to the current one, nothing would be written — the values passed all match the dashboard's current state")
	}

	// Propose phase: stash the computed change set + resulting payload and return
	// without writing — proposeUpdate ends the turn with an approval interrupt:
	// 确认文案由工具确定性渲染、重放参数一次性备好，运行时把本轮以该文案收尾并持久化
	// Pending；用户下一轮明确确认时由运行时直接以 ResumeArgs 重放本工具
	// （confirmDashboardProposal 腿）——确认环节零 LLM 参与，模型无需记忆/复述
	// proposal_id。服务端门（晚于提案轮、单次消费、基线哈希）见 confirmUpdateGate。
	logger.Infof("update_dashboard: user=%s, id=%d, proposed changes=%v", user.Username, id, allChanges)
	return proposeUpdate(ctx, deps, params, &updateProposal{
		Kind:         "dashboard",
		TargetID:     id,
		BaselineHash: hashConfigs(board.Configs),
		Payload:      string(newConfigs),
		Changes:      allChanges,
	}, renderUpdateProposalPrompt(params["lang"], fmt.Sprintf(aiagent.LangText(params["lang"],
		"仪表盘 **%s**（id=%d）", "dashboard **%s** (id=%d)"), board.Name, board.Id), allChanges), map[string]interface{}{
		"id": board.Id,
	})
}

// confirmDashboardProposal applies a previously proposed change set once
// confirmUpdateGate passes (single-use proposal, same chat, strictly later
// turn, unchanged baseline — see update_proposal.go for the guarantees).
func confirmDashboardProposal(ctx context.Context, deps *aiagent.ToolDeps, params map[string]string, user *models.User, board *models.Board, id int64, proposalID string, confirmed bool) (string, error) {
	p, err := confirmUpdateGate(ctx, deps, params, "update_dashboard", "dashboard", id, proposalID, confirmed, hashConfigs(board.Configs))
	if err != nil {
		return "", err
	}

	if err := models.BoardPayloadSave(deps.DBCtx, id, p.Payload); err != nil {
		return "", fmt.Errorf("failed to save dashboard config: %v", err)
	}

	board.UpdateBy = user.Username
	board.UpdateAt = time.Now().Unix()
	if err := board.Update(deps.DBCtx, "update_by", "update_at"); err != nil {
		// Payload already persisted; surface the metadata-update failure but
		// don't pretend the change didn't land.
		logger.Warningf("update_dashboard: payload saved but board meta update failed: id=%d, err=%v", id, err)
	}

	logger.Infof("update_dashboard: user=%s, id=%d, applied changes=%v, proposal_id=%s", user.Username, id, p.Changes, proposalID)

	result := map[string]interface{}{
		"id":       board.Id,
		"group_id": board.GroupId,
		"name":     board.Name,
		"changes":  p.Changes,
		"applied":  true,
	}
	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}
