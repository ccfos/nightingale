package tools

import (
	"context"
	"encoding/json"
	"fmt"
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

	groupId := getArgInt64(args, "group_id")
	if groupId == 0 {
		return "", fmt.Errorf("group_id is required")
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
		"id":               board.Id,
		"group_id":         board.GroupId,
		"group_name":       bg.Name,
		"name":             board.Name,
		"tags":             board.Tags,
		"datasource_id":    dsId,
		"panels_count":     len(panelSpecs),
		"variables_count":  len(varSpecs),
	}
	if ds, dsErr := models.DatasourceGet(deps.DBCtx, dsId); dsErr == nil && ds != nil {
		result["datasource_name"] = ds.Name
	}
	bytes, _ := json.Marshal(result)
	return string(bytes), nil
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

	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}
