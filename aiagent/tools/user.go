package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

type userResult struct {
	Id       int64    `json:"id"`
	Username string   `json:"username"`
	Nickname string   `json:"nickname,omitempty"`
	Phone    string   `json:"phone,omitempty"`
	Email    string   `json:"email,omitempty"`
	Roles    []string `json:"roles,omitempty"`
	Admin    bool     `json:"admin"`
}

func init() {
	register("list_users", aiagent.AgentTool{
		Name:        "list_users",
		Description: "查询用户列表，支持关键词搜索（用户名、昵称、邮箱、手机号）",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters: []aiagent.ToolParameter{
			{Name: "query", Type: "string", Description: "搜索关键词", Required: false},
			{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
		},
	}, listUsers)
}

func listUsers(_ context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(user, PermUsers); err != nil {
		return "", err
	}

	query := getArgString(args, "query")
	limit := getArgInt(args, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	dbCtx := aiagent.GetDBCtx()
	users, err := models.UserGets(dbCtx, query, limit, 0, 0, 0, "username", false, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to query users: %v", err)
	}

	results := make([]userResult, 0, len(users))
	for _, u := range users {
		results = append(results, userResult{
			Id:       u.Id,
			Username: u.Username,
			Nickname: u.Nickname,
			Phone:    u.Phone,
			Email:    u.Email,
			Roles:    strings.Fields(u.Roles),
			Admin:    u.IsAdmin(),
		})
	}

	logger.Debugf("list_users: query=%s, found %d users", query, len(results))
	return marshalList(len(results), results), nil
}
