package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

type notifyRuleResult struct {
	Id           int64   `json:"id"`
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	Enable       bool    `json:"enable"`
	UserGroupIds []int64 `json:"user_group_ids,omitempty"`
}

type notifyRuleDetailResult struct {
	Id           int64   `json:"id"`
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	Enable       bool    `json:"enable"`
	UserGroupIds []int64 `json:"user_group_ids,omitempty"`
	CreateBy     string  `json:"create_by,omitempty"`
	UpdateBy     string  `json:"update_by,omitempty"`
}

func init() {
	register(defs.ListNotifyRules, listNotifyRules)
	register(defs.GetNotifyRuleDetail, getNotifyRuleDetail)
}

func listNotifyRules(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermNotificationRules); err != nil {
		return "", err
	}

	query := getArgString(args, "query")
	limit := getArgInt(args, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	// NotifyRule has no group_id; permission is based on UserGroupIds intersection
	allRules, err := models.NotifyRulesGet(deps.DBCtx, "", nil)
	if err != nil {
		return "", fmt.Errorf("failed to query notify rules: %v", err)
	}

	// For non-admin, filter by user's team membership
	var myGroupIds []int64
	if !user.IsAdmin() {
		myGroupIds, err = getUserGroupIds(deps, user.Id)
		if err != nil {
			return "", err
		}
	}

	results := make([]notifyRuleResult, 0)
	for _, nr := range allRules {
		// Non-admin: only show rules whose UserGroupIds overlap with user's teams
		if !user.IsAdmin() {
			if !int64SlicesOverlap(myGroupIds, nr.UserGroupIds) {
				continue
			}
		}
		if query != "" && !containsIgnoreCase(nr.Name, query) {
			continue
		}
		results = append(results, notifyRuleResult{
			Id:           nr.ID,
			Name:         nr.Name,
			Description:  nr.Description,
			Enable:       nr.Enable,
			UserGroupIds: nr.UserGroupIds,
		})
		if len(results) >= limit {
			break
		}
	}

	logger.Debugf("list_notify_rules: user_id=%d, found %d rules", user.Id, len(results))
	return marshalList(len(results), results), nil
}

func getNotifyRuleDetail(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermNotificationRules); err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	rule, err := models.GetNotifyRule(deps.DBCtx, id)
	if err != nil {
		return "", fmt.Errorf("failed to get notify rule: %v", err)
	}
	if rule == nil {
		return fmt.Sprintf(`{"error":"notify rule not found: id=%d"}`, id), nil
	}

	if !user.IsAdmin() {
		myGroupIds, err := getUserGroupIds(deps, user.Id)
		if err != nil {
			return "", err
		}
		if !int64SlicesOverlap(myGroupIds, rule.UserGroupIds) {
			return "", fmt.Errorf("forbidden: no access to this notify rule")
		}
	}

	result := notifyRuleDetailResult{
		Id:           rule.ID,
		Name:         rule.Name,
		Description:  rule.Description,
		Enable:       rule.Enable,
		UserGroupIds: rule.UserGroupIds,
		CreateBy:     rule.CreateBy,
		UpdateBy:     rule.UpdateBy,
	}

	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}
