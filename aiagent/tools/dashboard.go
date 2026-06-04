package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

type dashboardResult struct {
	Id      int64  `json:"id"`
	GroupId int64  `json:"group_id"`
	Name    string `json:"name"`
	Tags    string `json:"tags,omitempty"`
	Ident   string `json:"ident,omitempty"`
	Public  int    `json:"public"`
}

type dashboardDetailResult struct {
	Id       int64  `json:"id"`
	GroupId  int64  `json:"group_id"`
	Name     string `json:"name"`
	Tags     string `json:"tags,omitempty"`
	Ident    string `json:"ident,omitempty"`
	Note     string `json:"note,omitempty"`
	Public   int    `json:"public"`
	BuiltIn  int    `json:"built_in"`
	CreateBy string `json:"create_by,omitempty"`
	UpdateBy string `json:"update_by,omitempty"`
}

func init() {
	register(defs.ListDashboards, listDashboards)
	register(defs.GetDashboardDetail, getDashboardDetail)
	register(defs.CreateDashboard, createDashboard)
	register(defs.ImportDashboardTemplate, importDashboardTemplate)
}

func listDashboards(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermDashboards); err != nil {
		return "", err
	}

	bgids, isAdmin, err := getUserBgids(deps, user)
	if err != nil {
		return "", err
	}

	query := getArgString(args, "query")
	limit := getArgInt(args, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	var boards []models.Board
	if isAdmin {
		boards, err = models.BoardGetsByBGIds(deps.DBCtx, nil, query)
	} else {
		if len(bgids) == 0 {
			return marshalList(0, []dashboardResult{}), nil
		}
		boards, err = models.BoardGetsByBGIds(deps.DBCtx, bgids, query)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query dashboards: %v", err)
	}

	results := make([]dashboardResult, 0)
	for _, b := range boards {
		results = append(results, dashboardResult{
			Id:      b.Id,
			GroupId: b.GroupId,
			Name:    b.Name,
			Tags:    b.Tags,
			Ident:   b.Ident,
			Public:  b.Public,
		})
		if len(results) >= limit {
			break
		}
	}

	logger.Debugf("list_dashboards: user_id=%d, query=%s, found %d boards", user.Id, query, len(results))
	return marshalList(len(results), results), nil
}

func createDashboard(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	// Match the FE route: /dashboards/add role permission + bgrw on the group.
	if err := checkPerm(deps, user, PermDashboardsAdd); err != nil {
		return "", err
	}

	// 缺参门：缺业务组时以表单中断向用户取值，而非纯错误回给模型让它替用户瞎选。
	groupId := resolveCreationGroupID(args, params)
	if groupId == 0 {
		return "", creationFormInterrupt(deps, user, "n9e-create-dashboard", []string{"busi_group_id"})
	}

	name := getArgString(args, "name")
	if name == "" {
		return "", fmt.Errorf("name is required")
	}

	// Check business group exists and the user has rw permission on it.
	bg, err := models.BusiGroupGetById(deps.DBCtx, groupId)
	if err != nil {
		return "", fmt.Errorf("failed to get busi group: %v", err)
	}
	if bg == nil {
		return "", fmt.Errorf("busi group not found: id=%d", groupId)
	}
	if err := checkBgRW(deps, user, bg); err != nil {
		return "", err
	}

	// 获取数据源 ID：先看工具参数，再回退到 page/preflight 注入的 params。
	// 不再静默兜底到 1——错误的数据源会创建出无法查询的面板，必须显式指定。
	dsId := getArgInt64(args, "datasource_id")
	if dsId == 0 {
		dsId = getDatasourceId(params)
	}
	if dsId == 0 {
		return "", fmt.Errorf("datasource_id is required: call list_datasources to pick one, or ask the user")
	}

	// 构建 configs
	panelsJSON := getArgString(args, "panels")
	if panelsJSON == "" {
		return "", fmt.Errorf("panels is required")
	}

	var panelSpecs []PanelSpec
	if err := json.Unmarshal([]byte(panelsJSON), &panelSpecs); err != nil {
		return "", fmt.Errorf("invalid panels JSON: %v", err)
	}

	var varSpecs []VariableSpec
	if varsJSON := getArgString(args, "variables"); varsJSON != "" {
		if err := json.Unmarshal([]byte(varsJSON), &varSpecs); err != nil {
			return "", fmt.Errorf("invalid variables JSON: %v", err)
		}
	}

	configs, err := buildConfigs(dsId, varSpecs, panelSpecs)
	if err != nil {
		return "", fmt.Errorf("failed to build configs: %v", err)
	}

	board := &models.Board{
		GroupId:  groupId,
		Name:     name,
		Tags:     getArgString(args, "tags"),
		CreateBy: user.Username,
		UpdateBy: user.Username,
	}

	if err := board.AtomicAdd(deps.DBCtx, configs); err != nil {
		// Instructive error on name conflict: tell the LLM exactly how to recover
		// so it doesn't waste iterations calling list_dashboards to investigate.
		if strings.Contains(err.Error(), "Name duplicate") {
			return "", fmt.Errorf(
				"dashboard name %q already exists in busi_group %d. "+
					"DO NOT call list_dashboards. "+
					"Retry create_dashboard immediately with a different name, "+
					"e.g. %q or %q",
				name, groupId, name+"-v2", name+"-AI",
			)
		}
		return "", fmt.Errorf("failed to create dashboard: %v", err)
	}

	logger.Infof("create_dashboard: user=%s, group_id=%d, name=%s, id=%d", user.Username, groupId, name, board.Id)

	result := map[string]interface{}{
		"id":              board.Id,
		"group_id":        board.GroupId,
		"group_name":      bg.Name,
		"name":            board.Name,
		"tags":            board.Tags,
		"datasource_id":   dsId,
		"panels_count":    len(panelSpecs),
		"variables_count": len(varSpecs),
	}
	if ds, dsErr := models.DatasourceGet(deps.DBCtx, dsId); dsErr == nil && ds != nil {
		result["datasource_name"] = ds.Name
	}
	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}

// importDashboardTemplate clones a validated dashboard from the integrations/
// tree into a busi group. Unlike create_dashboard (which rebuilds a crude board
// from a few PromQL strings), this preserves the template's full hand-tuned
// configs — layout, thresholds, units, overrides, value mappings — and only
// rewrites the datasource binding so the board works against the user's
// Prometheus instance. The template JSON is read server-side, so large files
// (>64KB) that read_file would truncate are handled correctly.
func importDashboardTemplate(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermDashboardsAdd); err != nil {
		return "", err
	}

	// 缺参门：同 create_dashboard。
	groupId := resolveCreationGroupID(args, params)
	if groupId == 0 {
		return "", creationFormInterrupt(deps, user, "n9e-create-dashboard", []string{"busi_group_id"})
	}

	component := strings.TrimSpace(getArgString(args, "component"))
	if component == "" {
		return "", fmt.Errorf("component is required (e.g. \"Linux\"); call list_files(base=\"integrations\") to discover")
	}
	file := strings.TrimSpace(getArgString(args, "file"))
	if file == "" {
		return "", fmt.Errorf("file is required (e.g. \"categraf-overview.json\"); call list_files(base=\"integrations/%s\", path=\"dashboards\")", component)
	}

	bg, err := models.BusiGroupGetById(deps.DBCtx, groupId)
	if err != nil {
		return "", fmt.Errorf("failed to get busi group: %v", err)
	}
	if bg == nil {
		return "", fmt.Errorf("busi group not found: id=%d", groupId)
	}
	if err := checkBgRW(deps, user, bg); err != nil {
		return "", err
	}

	// datasource_id is optional for import: the template carries its own
	// datasource variable, so the board still works (the FE auto-selects the
	// first Prometheus datasource). When provided, we set it as the variable's
	// default so the dashboard queries the intended datasource on first open.
	dsId := getArgInt64(args, "datasource_id")
	if dsId == 0 {
		dsId = getDatasourceId(params)
	}

	// Resolve via the same path logic read_file/list_files use, so the file the
	// agent inspected and the file we import are guaranteed identical.
	tplPath, err := resolveBasePath(deps, "integrations/"+component, "dashboards/"+file)
	if err != nil {
		return "", fmt.Errorf("template not found: %v", err)
	}
	raw, err := os.ReadFile(tplPath)
	if err != nil {
		return "", fmt.Errorf("failed to read template %s/%s: %v", component, file, err)
	}

	var tpl struct {
		Name    string          `json:"name"`
		Tags    string          `json:"tags"`
		Configs json.RawMessage `json:"configs"`
	}
	if err := json.Unmarshal(raw, &tpl); err != nil {
		return "", fmt.Errorf("invalid template JSON in %s/%s: %v", component, file, err)
	}
	if len(tpl.Configs) == 0 {
		return "", fmt.Errorf("template %s/%s has no configs", component, file)
	}

	configs, err := parseTemplateConfigs(tpl.Configs)
	if err != nil {
		return "", fmt.Errorf("failed to parse template configs: %v", err)
	}

	panelCount := normalizeTemplateDatasource(configs, dsId)
	if configs["version"] == nil {
		configs["version"] = "3.4.0"
	}

	// Count user-facing template variables (exclude the datasource selector)
	// to match the create_dashboard result shape so the UI card renders fully.
	varCount := 0
	if vars, ok := configs["var"].([]interface{}); ok {
		for _, v := range vars {
			if vm, ok := v.(map[string]interface{}); ok && vm["type"] != "datasource" {
				varCount++
			}
		}
	}

	configsJSON, err := json.Marshal(configs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal configs: %v", err)
	}

	name := getArgString(args, "name")
	if name == "" {
		name = tpl.Name
	}
	if name == "" {
		return "", fmt.Errorf("template %s/%s has no name; pass an explicit name", component, file)
	}
	tags := getArgString(args, "tags")
	if tags == "" {
		tags = tpl.Tags
	}

	board := &models.Board{
		GroupId:  groupId,
		Name:     name,
		Tags:     tags,
		CreateBy: user.Username,
		UpdateBy: user.Username,
	}

	if err := board.AtomicAdd(deps.DBCtx, string(configsJSON)); err != nil {
		if strings.Contains(err.Error(), "Name duplicate") {
			return "", fmt.Errorf(
				"dashboard name %q already exists in busi_group %d. "+
					"DO NOT call list_dashboards. "+
					"Retry import_dashboard_template immediately with a different name, "+
					"e.g. %q or %q",
				name, groupId, name+"-v2", name+"-AI",
			)
		}
		return "", fmt.Errorf("failed to create dashboard: %v", err)
	}

	logger.Infof("import_dashboard_template: user=%s, group_id=%d, name=%s, template=%s/%s, id=%d",
		user.Username, groupId, name, component, file, board.Id)

	result := map[string]interface{}{
		"id":              board.Id,
		"group_id":        board.GroupId,
		"group_name":      bg.Name,
		"name":            board.Name,
		"tags":            board.Tags,
		"panels_count":    panelCount,
		"variables_count": varCount,
		"source":          component + "/" + file,
	}
	// Only surface datasource info when one was actually pinned; otherwise the
	// dashboard's datasource is chosen from the header dropdown at view time.
	if dsId > 0 {
		result["datasource_id"] = dsId
		if ds, dsErr := models.DatasourceGet(deps.DBCtx, dsId); dsErr == nil && ds != nil {
			result["datasource_name"] = ds.Name
		}
	}
	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}

// parseTemplateConfigs accepts either a configs object or a JSON-stringified
// configs (both shapes appear across integration template files) and returns
// the decoded map.
func parseTemplateConfigs(rawConfigs json.RawMessage) (map[string]interface{}, error) {
	// Stringified form: configs is a JSON string whose contents are the object.
	var asString string
	if err := json.Unmarshal(rawConfigs, &asString); err == nil {
		rawConfigs = json.RawMessage(asString)
	}
	var configs map[string]interface{}
	if err := json.Unmarshal(rawConfigs, &configs); err != nil {
		return nil, err
	}
	if configs == nil {
		return nil, fmt.Errorf("configs is empty")
	}
	return configs, nil
}

// normalizeTemplateDatasource rewrites a template's datasource binding so the
// imported board queries the user's Prometheus instance regardless of how the
// original template referenced its datasource (native ${prom}/${datasource},
// Grafana-style ${DS_PROMETHEUS}, or a hardcoded numeric id). It guarantees a
// single canonical datasource-type variable exists, points it at the chosen
// datasource as the default, and repoints every dangling/literal panel and
// query-variable reference at it. Returns the panel count.
func normalizeTemplateDatasource(configs map[string]interface{}, dsId int64) int {
	vars, _ := configs["var"].([]interface{})

	// Pass 1: find existing datasource-type vars, harden them, collect names.
	dsVarNames := map[string]bool{}
	primary := ""
	for _, v := range vars {
		vm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if vm["type"] != "datasource" {
			continue
		}
		vm["definition"] = "prometheus"
		vm["hide"] = false
		if dsId > 0 {
			vm["defaultValue"] = dsId
		}
		if name, _ := vm["name"].(string); name != "" {
			dsVarNames[name] = true
			if primary == "" {
				primary = name
			}
		}
	}

	// No datasource var at all → inject the canonical "prom" var at the front.
	if primary == "" {
		primary = datasourceVarName
		dsVar := buildDatasourceVariable()
		if dsId > 0 {
			dsVar["defaultValue"] = dsId
		}
		vars = append([]interface{}{dsVar}, vars...)
		configs["var"] = vars
		dsVarNames[primary] = true
	}

	primaryRef := "${" + primary + "}"

	// Pass 2: repoint query-variable datasource refs that dangle.
	for _, v := range vars {
		vm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		ds, ok := vm["datasource"].(map[string]interface{})
		if !ok {
			continue
		}
		if !refResolves(ds["value"], dsVarNames) {
			ds["value"] = primaryRef
			if ds["cate"] == nil {
				ds["cate"] = "prometheus"
			}
		}
	}

	// Pass 3: repoint every panel's datasourceValue that dangles or is literal,
	// recursing into collapsed-row nested panels. Some templates (e.g.
	// ClickHouse) keep ALL real chart panels nested inside top-level rows with
	// hardcoded datasourceValue:1 — walking only the top level would leave
	// every chart pinned to the wrong datasource. Returns the data-panel count.
	panels, _ := configs["panels"].([]interface{})
	return repointPanels(panels, dsVarNames, primaryRef)
}

// repointPanels walks a panel list (recursing into the nested panels of
// collapsed rows) and repoints any datasourceValue that doesn't resolve to a
// known datasource variable. Returns the number of non-row (data) panels seen
// at all depths, so panels_count reflects actual charts rather than just the
// top-level container rows.
func repointPanels(panels []interface{}, dsVarNames map[string]bool, primaryRef string) int {
	count := 0
	for _, p := range panels {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if pm["type"] != "row" {
			count++
		}
		if _, has := pm["datasourceValue"]; has {
			if !refResolves(pm["datasourceValue"], dsVarNames) {
				pm["datasourceValue"] = primaryRef
				pm["datasourceCate"] = "prometheus"
			}
		}
		// Collapsed rows hold their children in a nested "panels" array.
		if nested, ok := pm["panels"].([]interface{}); ok && len(nested) > 0 {
			count += repointPanels(nested, dsVarNames, primaryRef)
		}
	}
	return count
}

// refResolves reports whether a datasourceValue is a "${name}" reference whose
// variable name is a known datasource variable. Literal ids, empty values, and
// references to non-datasource/undeclared vars do not resolve and get repointed.
func refResolves(val interface{}, dsVarNames map[string]bool) bool {
	s, ok := val.(string)
	if !ok {
		return false
	}
	if !strings.HasPrefix(s, "${") || !strings.HasSuffix(s, "}") {
		return false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(s, "${"), "}")
	return dsVarNames[name]
}

func getDashboardDetail(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermDashboards); err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	board, err := models.BoardGetByID(deps.DBCtx, id)
	if err != nil {
		return "", fmt.Errorf("failed to get dashboard: %v", err)
	}
	if board == nil {
		return fmt.Sprintf(`{"error":"dashboard not found: id=%d"}`, id), nil
	}

	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(deps, user)
		if err != nil {
			return "", err
		}
		if !int64SliceContains(bgids, board.GroupId) {
			return "", fmt.Errorf("forbidden: no access to this dashboard")
		}
	}

	result := dashboardDetailResult{
		Id:       board.Id,
		GroupId:  board.GroupId,
		Name:     board.Name,
		Tags:     board.Tags,
		Ident:    board.Ident,
		Note:     board.Note,
		Public:   board.Public,
		BuiltIn:  board.BuiltIn,
		CreateBy: board.CreateBy,
		UpdateBy: board.UpdateBy,
	}

	// include_config surfaces the variable/panel summary + variable lint so the
	// agent can show a "before → after" diff before calling update_dashboard.
	// Default-off keeps the common read path lean (payloads can be large).
	// getArgBool (not a raw .(bool)) so a string-form "true" from the LLM still
	// turns the config summary on — otherwise the edit flow's "before" snapshot
	// silently comes back empty.
	if getArgBool(args, "include_config") {
		out := map[string]interface{}{"dashboard": result}
		// BoardGetByID does not hydrate the board_payload row, so the config
		// lives in a separate table — load it explicitly. Without this the
		// variables/panels/lint summary would always be empty for real
		// dashboards and the "before → after" diff would have no "before".
		payload, err := models.BoardPayloadGet(deps.DBCtx, id)
		if err != nil {
			return "", fmt.Errorf("failed to get dashboard config: %v", err)
		}
		if strings.TrimSpace(payload) != "" {
			var configs map[string]interface{}
			if err := json.Unmarshal([]byte(payload), &configs); err != nil {
				out["config_error"] = fmt.Sprintf("invalid config payload: %v", err)
			} else {
				vars, panels := summarizeConfigs(configs)
				out["variables"] = vars
				out["panels"] = panels
				out["variable_lint"] = lintVariables(configs)
			}
		}
		bytes, _ := json.Marshal(out)
		return string(bytes), nil
	}

	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}
