package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ccfos/nightingale/v6/aiagent"
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
	register("list_dashboards", aiagent.AgentTool{
		Name:        "list_dashboards",
		Description: "查询当前用户有权限的仪表盘列表，支持关键词搜索",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters: []aiagent.ToolParameter{
			{Name: "query", Type: "string", Description: "搜索关键词，匹配仪表盘名称或标签", Required: false},
			{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
		},
	}, listDashboards)

	register("get_dashboard_detail", aiagent.AgentTool{
		Name:        "get_dashboard_detail",
		Description: "获取单个仪表盘的详细信息",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters: []aiagent.ToolParameter{
			{Name: "id", Type: "integer", Description: "仪表盘ID", Required: true},
		},
	}, getDashboardDetail)
}

func listDashboards(_ context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(user, PermDashboards); err != nil {
		return "", err
	}

	bgids, isAdmin, err := getUserBgids(user)
	if err != nil {
		return "", err
	}

	query := getArgString(args, "query")
	limit := getArgInt(args, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	dbCtx := aiagent.GetDBCtx()
	var boards []models.Board
	if isAdmin {
		boards, err = models.BoardGetsByBGIds(dbCtx, nil, query)
	} else {
		if len(bgids) == 0 {
			return marshalList(0, []dashboardResult{}), nil
		}
		boards, err = models.BoardGetsByBGIds(dbCtx, bgids, query)
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

func getDashboardDetail(_ context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(user, PermDashboards); err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	dbCtx := aiagent.GetDBCtx()
	board, err := models.BoardGetByID(dbCtx, id)
	if err != nil {
		return "", fmt.Errorf("failed to get dashboard: %v", err)
	}
	if board == nil {
		return fmt.Sprintf(`{"error":"dashboard not found: id=%d"}`, id), nil
	}

	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(user)
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
