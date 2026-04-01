package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

// =============================================================================
// Result types for LLM consumption
// =============================================================================

type busiGroupResult struct {
	Id         int64  `json:"id"`
	Name       string `json:"name"`
	LabelValue string `json:"label_value,omitempty"`
}

// =============================================================================
// Tool registration
// =============================================================================

func init() {
	builtinTools["list_busi_groups"] = &BuiltinTool{
		Definition: AgentTool{
			Name:        "list_busi_groups",
			Description: "查询当前用户有权限的业务组列表，支持关键词模糊搜索",
			Type:        ToolTypeBuiltin,
			Parameters: []ToolParameter{
				{Name: "query", Type: "string", Description: "搜索关键词，模糊匹配业务组名称", Required: false},
				{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
			},
		},
		Handler: listBusiGroups,
	}
}

// =============================================================================
// Tool implementation
// =============================================================================

func listBusiGroups(_ context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	if dbCtx == nil {
		return "", fmt.Errorf("database context not configured")
	}

	// Get user_id from params
	var userId int64
	if uid, ok := params["user_id"]; ok {
		userId, _ = strconv.ParseInt(uid, 10, 64)
	}
	if userId == 0 {
		return "", fmt.Errorf("user_id not found in params")
	}

	query, _ := args["query"].(string)
	limit := 50
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 200 {
			limit = 200
		}
	}

	user, err := models.UserGetById(dbCtx, userId)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %v", err)
	}
	if user == nil {
		return "", fmt.Errorf("user not found: %d", userId)
	}

	groups, err := user.BusiGroups(dbCtx, limit, query)
	if err != nil {
		return "", fmt.Errorf("failed to query busi groups: %v", err)
	}

	results := make([]busiGroupResult, 0, len(groups))
	for _, g := range groups {
		results = append(results, busiGroupResult{
			Id:         g.Id,
			Name:       g.Name,
			LabelValue: g.LabelValue,
		})
	}

	logger.Debugf("list_busi_groups: user_id=%d, query=%s, found %d groups", userId, query, len(results))

	bytes, _ := json.Marshal(map[string]interface{}{
		"total":  len(results),
		"groups": results,
	})
	return string(bytes), nil
}
