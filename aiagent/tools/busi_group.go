package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/toolkits/pkg/logger"
)

type busiGroupResult struct {
	Id         int64  `json:"id"`
	Name       string `json:"name"`
	LabelValue string `json:"label_value,omitempty"`
	// IsDefault hints that this group is the conventional default (name contains
	// "default" case-insensitively, or contains "默认"). The LLM should prefer
	// is_default=true groups when the user did not specify one, to avoid
	// accidentally picking a test/scratch group that happens to sort first.
	IsDefault bool `json:"is_default,omitempty"`
}

// isDefaultBusiGroupName reports whether a busi group name looks like a
// default group by naming convention. Exposed as a helper so the hint in
// list_busi_groups results stays consistent with any future selection logic.
func isDefaultBusiGroupName(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "default") || strings.Contains(name, "默认")
}

func init() {
	register(defs.ListBusiGroups, listBusiGroups)
}

func listBusiGroups(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}

	query := getArgString(args, "query")
	limit := getArgInt(args, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	groups, err := user.BusiGroups(deps.DBCtx, limit, query)
	if err != nil {
		return "", fmt.Errorf("failed to query busi groups: %v", err)
	}

	results := make([]busiGroupResult, 0, len(groups))
	for _, g := range groups {
		results = append(results, busiGroupResult{
			Id:         g.Id,
			Name:       g.Name,
			LabelValue: g.LabelValue,
			IsDefault:  isDefaultBusiGroupName(g.Name),
		})
	}

	logger.Debugf("list_busi_groups: user_id=%d, query=%s, found %d groups", user.Id, query, len(results))
	return marshalList(len(results), results), nil
}
