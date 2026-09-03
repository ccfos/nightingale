package tools

import (
	"context"
	"fmt"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/toolkits/pkg/logger"
)

type teamResult struct {
	Id       int64  `json:"id"`
	Name     string `json:"name"`
	Note     string `json:"note,omitempty"`
	CreateBy string `json:"create_by,omitempty"`
}

func init() {
	register(defs.ListTeams, listTeams)
}

func listTeams(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	// Team listing has no perm middleware in router

	query := getArgString(args, "query")
	limit := getArgInt(args, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	// User.UserGroups() handles permission: admin sees all, non-admin sees own + created
	groups, err := user.UserGroups(deps.DBCtx, limit, query)
	if err != nil {
		return "", fmt.Errorf("failed to query teams: %v", err)
	}

	results := make([]teamResult, 0, len(groups))
	for _, g := range groups {
		results = append(results, teamResult{
			Id:       g.Id,
			Name:     g.Name,
			Note:     g.Note,
			CreateBy: g.CreateBy,
		})
	}

	logger.Debugf("list_teams: user_id=%d, query=%s, found %d teams", user.Id, query, len(results))
	return marshalList(len(results), results), nil
}
