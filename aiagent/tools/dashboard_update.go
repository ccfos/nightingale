package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
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
		Queries *[]QuerySpec    `json:"queries"`
		Delete  json.RawMessage `json:"delete"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	p.ID, p.Name, p.NewName, p.Unit, p.Desc, p.Queries = r.ID, r.Name, r.NewName, r.Unit, r.Desc, r.Queries

	var err error
	if p.Delete, err = flexBool(r.Delete); err != nil {
		return fmt.Errorf("panel field delete: %w", err)
	}
	return nil
}

// applyVariablePatches mutates configs["var"] per the patches and returns a list
// of human-readable change descriptions. Patches match existing variables by
// name; an unmatched, non-delete patch adds a new query variable.
func applyVariablePatches(configs map[string]interface{}, patches []variablePatch) ([]string, error) {
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
			changes = append(changes, fmt.Sprintf("删除变量 %q", p.Name))

		case idx >= 0:
			vm := vars[idx].(map[string]interface{})
			applyVarFields(vm, p)
			changes = append(changes, fmt.Sprintf("更新变量 %q", p.Name))

		default: // add new variable
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
			if p.Type != nil {
				nv["type"] = *p.Type
			}
			vars = append(vars, nv)
			changes = append(changes, fmt.Sprintf("新增变量 %q", p.Name))
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
func applyPanelPatches(configs map[string]interface{}, patches []panelPatch) ([]string, error) {
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
			changes = append(changes, fmt.Sprintf("删除图表 %q", label))
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
		applyPanelFields(pm, p)
		changes = append(changes, fmt.Sprintf("更新图表 %q", label))
	}

	configs["panels"] = panels
	return changes, nil
}

func applyPanelFields(pm map[string]interface{}, p panelPatch) {
	if p.NewName != nil {
		pm["name"] = *p.NewName
	}
	if p.Desc != nil {
		pm["description"] = *p.Desc
	}
	if p.Unit != nil {
		setPanelUnit(pm, *p.Unit)
	}
	if p.Queries != nil {
		existing, _ := pm["targets"].([]interface{})
		pm["targets"] = mergeTargets(existing, *p.Queries)
	}
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

	allChanges := make([]string, 0)

	if varsJSON := getArgString(args, "variables"); varsJSON != "" {
		var patches []variablePatch
		if err := json.Unmarshal([]byte(varsJSON), &patches); err != nil {
			return "", fmt.Errorf("invalid variables JSON: %v", err)
		}
		changes, err := applyVariablePatches(configs, patches)
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
		changes, err := applyPanelPatches(configs, patches)
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
		allChanges = append(allChanges, "修复数据源引用（统一重指到大盘数据源变量）")
	}

	if len(allChanges) == 0 {
		return "", fmt.Errorf("nothing to change: pass variables, panels, or fix_datasource=true")
	}

	newConfigs, err := json.Marshal(configs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal updated config: %v", err)
	}

	// Propose phase: stash the computed change set + resulting payload and return
	// without writing. The write happens only on a later confirm call that quotes
	// this proposal_id (see confirmDashboardProposal).
	proposalID, err = newProposalID()
	if err != nil {
		return "", fmt.Errorf("failed to generate proposal id: %v", err)
	}
	seqID, _ := strconv.ParseInt(params["seq_id"], 10, 64)
	if err := dashboardProposals.put(ctx, deps.Redis, &dashboardProposal{
		ID:           proposalID,
		ChatID:       params["chat_id"],
		SeqID:        seqID,
		BoardID:      id,
		BaselineHash: hashConfigs(board.Configs),
		NewConfigs:   string(newConfigs),
		Changes:      allChanges,
		CreatedAt:    time.Now(),
	}); err != nil {
		return "", fmt.Errorf("failed to stash dashboard proposal: %v", err)
	}

	logger.Infof("update_dashboard: user=%s, id=%d, proposed changes=%v, proposal_id=%s", user.Username, id, allChanges, proposalID)

	// 人在环中断：确认文案由工具确定性渲染、重放参数
	// 一次性备好。运行时把本轮以该文案收尾并持久化 Pending；用户下一轮明确确认时
	// 由运行时直接以 ResumeArgs 重放本工具（confirmDashboardProposal 腿）——确认
	// 环节零 LLM 参与，模型无需记忆/复述 proposal_id。原有服务端门（晚于提案轮、
	// 单次消费、基线哈希）全部保留生效。
	resumeArgs, _ := json.Marshal(map[string]interface{}{
		"id":          board.Id,
		"proposal_id": proposalID,
		"confirmed":   true,
	})
	return "", &aiagent.ToolInterrupt{
		Kind:       aiagent.InterruptKindApproval,
		Prompt:     renderDashboardProposalPrompt(board.Name, board.Id, allChanges),
		ResumeArgs: string(resumeArgs),
	}
}

// renderDashboardProposalPrompt 把提案改动渲染成给用户的确认文案（markdown）。
// 由工具确定性生成，不再依赖模型转述，保证用户看到的就是将要写入的全部改动。
func renderDashboardProposalPrompt(name string, id int64, changes []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("即将修改仪表盘 **%s**（id=%d），改动如下：\n", name, id))
	for _, c := range changes {
		sb.WriteString("\n- ")
		sb.WriteString(c)
	}
	sb.WriteString("\n\n以上改动尚未写入。回复「确认」立即生效，回复「取消」放弃本次修改，也可以直接提出新的调整要求。")
	return sb.String()
}

// confirmDashboardProposal applies a previously proposed change set. It enforces
// the guarantees the prompt alone couldn't: the proposal must exist (and be
// single-use), it must target this dashboard, the confirmation must land in a
// LATER chat turn than the proposal (so a single model turn can't propose and
// confirm itself), and the dashboard's payload must be unchanged since the
// proposal baseline (so a concurrent edit isn't silently overwritten).
func confirmDashboardProposal(ctx context.Context, deps *aiagent.ToolDeps, params map[string]string, user *models.User, board *models.Board, id int64, proposalID string, confirmed bool) (string, error) {
	if !confirmed {
		return "", fmt.Errorf("proposal_id was provided but confirmed is not true: pass confirmed=true to apply the proposal")
	}
	if strings.TrimSpace(proposalID) == "" {
		return "", fmt.Errorf("confirmed=true requires the proposal_id returned by the initial call; first call update_dashboard without confirmed to generate a proposal, show the diff, then confirm")
	}

	// Validate against a PEEKED copy (no delete yet): a confirm rejected by the
	// gate below must NOT burn the proposal, or the user's genuine confirm next
	// turn would fail with "not found". The proposal is consumed only once every
	// check passes and we're about to write (the take() right before the save).
	p := dashboardProposals.peek(ctx, deps.Redis, proposalID)
	if p == nil {
		return "", fmt.Errorf("proposal %q not found or expired; call update_dashboard again without confirmed to regenerate a fresh proposal against the current config, show the diff, and confirm again", proposalID)
	}
	if p.BoardID != id {
		return "", fmt.Errorf("proposal %q is for dashboard id=%d, not id=%d", proposalID, p.BoardID, id)
	}

	// Turn-identity gate — FAIL CLOSED. A destructive dashboard edit is applied
	// only when the confirmation provably arrives in a LATER chat turn than the
	// proposal (proof that a real user message landed in between). The assistant
	// router always injects a non-empty chat_id + numeric seq_id (chat_id is
	// required upstream). A caller that supplies neither — a headless workflow or
	// CLI agent — can't prove a human confirmed, so we REFUSE rather than let one
	// model turn both propose and confirm: the model already sees the proposal_id
	// in the propose response, so it is no barrier on its own. Rejection here is
	// recoverable — the proposal was only peeked, not consumed.
	curSeq, seqErr := strconv.ParseInt(strings.TrimSpace(params["seq_id"]), 10, 64)
	switch {
	case params["chat_id"] == "" || params["chat_id"] != p.ChatID:
		return "", fmt.Errorf("this dashboard update can only be confirmed inside the same chat conversation that proposed it; regenerate the proposal here, show the diff, and confirm")
	case seqErr != nil || curSeq <= p.SeqID:
		return "", fmt.Errorf("a dashboard update must be confirmed by the user in a later turn than the proposal; show the diff, wait for the user to confirm, then call update_dashboard again with the proposal_id")
	}

	// Conflict guard: refuse if the board changed since the proposal baseline.
	if hashConfigs(board.Configs) != p.BaselineHash {
		return "", fmt.Errorf("dashboard id=%d has changed since this proposal was generated, so it is stale; call update_dashboard again without confirmed to regenerate against the current config, show the new diff, and confirm again", id)
	}

	// All checks passed — consume now (atomic single-use). If another confirm
	// won the race, or the proposal expired between the peek and here, take()
	// returns nil and we don't write twice.
	if dashboardProposals.take(ctx, deps.Redis, proposalID) == nil {
		return "", fmt.Errorf("proposal %q is no longer available (already confirmed or expired); call update_dashboard again without confirmed to regenerate", proposalID)
	}

	if err := models.BoardPayloadSave(deps.DBCtx, id, p.NewConfigs); err != nil {
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
