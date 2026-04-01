package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

type alertRuleResult struct {
	Id       int64  `json:"id"`
	GroupId  int64  `json:"group_id"`
	Name     string `json:"name"`
	Severity int    `json:"severity"`
	Disabled int    `json:"disabled"`
	Cate     string `json:"cate,omitempty"`
	PromQl   string `json:"prom_ql,omitempty"`
	Note     string `json:"note,omitempty"`
}

type alertRuleDetailResult struct {
	Id            int64             `json:"id"`
	GroupId       int64             `json:"group_id"`
	Name          string            `json:"name"`
	Note          string            `json:"note,omitempty"`
	Severity      int               `json:"severity"`
	Disabled      int               `json:"disabled"`
	Cate          string            `json:"cate,omitempty"`
	PromQl        string            `json:"prom_ql,omitempty"`
	AppendTags    []string          `json:"append_tags,omitempty"`
	Annotations   map[string]string `json:"annotations,omitempty"`
	RunbookUrl    string            `json:"runbook_url,omitempty"`
	NotifyRuleIds []int64           `json:"notify_rule_ids,omitempty"`
	CreateBy      string            `json:"create_by,omitempty"`
	UpdateBy      string            `json:"update_by,omitempty"`
}

func init() {
	register("list_alert_rules", aiagent.AgentTool{
		Name:        "list_alert_rules",
		Description: "查询当前用户有权限的告警规则列表，支持关键词搜索和状态过滤",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters: []aiagent.ToolParameter{
			{Name: "query", Type: "string", Description: "搜索关键词，匹配规则名称", Required: false},
			{Name: "disabled", Type: "integer", Description: "状态过滤: 0=启用, 1=禁用, -1=全部（默认-1）", Required: false},
			{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
		},
	}, listAlertRules)

	register("get_alert_rule_detail", aiagent.AgentTool{
		Name:        "get_alert_rule_detail",
		Description: "获取单条告警规则的详细信息",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters: []aiagent.ToolParameter{
			{Name: "id", Type: "integer", Description: "告警规则ID", Required: true},
		},
	}, getAlertRuleDetail)
}

func listAlertRules(_ context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(user, PermAlertRules); err != nil {
		return "", err
	}

	bgids, isAdmin, err := getUserBgids(user)
	if err != nil {
		return "", err
	}

	query := getArgString(args, "query")
	disabled := -1
	if d, ok := args["disabled"].(float64); ok {
		disabled = int(d)
	}
	limit := getArgInt(args, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	dbCtx := aiagent.GetDBCtx()
	var rules []models.AlertRule
	if isAdmin {
		rules, err = models.AlertRuleGetsByBGIds(dbCtx, nil)
	} else {
		if len(bgids) == 0 {
			return marshalList(0, []alertRuleResult{}), nil
		}
		rules, err = models.AlertRuleGetsByBGIds(dbCtx, bgids)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query alert rules: %v", err)
	}

	results := make([]alertRuleResult, 0)
	for _, r := range rules {
		if disabled != -1 && r.Disabled != disabled {
			continue
		}
		if query != "" && !containsIgnoreCase(r.Name, query) {
			continue
		}
		results = append(results, alertRuleResult{
			Id:       r.Id,
			GroupId:  r.GroupId,
			Name:     r.Name,
			Severity: r.Severity,
			Disabled: r.Disabled,
			Cate:     r.Cate,
			PromQl:   r.PromQl,
		})
		if len(results) >= limit {
			break
		}
	}

	logger.Debugf("list_alert_rules: user_id=%d, query=%s, found %d rules", user.Id, query, len(results))
	return marshalList(len(results), results), nil
}

func getAlertRuleDetail(_ context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(user, PermAlertRules); err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	dbCtx := aiagent.GetDBCtx()
	rule, err := models.AlertRuleGetById(dbCtx, id)
	if err != nil {
		return "", fmt.Errorf("failed to get alert rule: %v", err)
	}
	if rule == nil {
		return fmt.Sprintf(`{"error":"alert rule not found: id=%d"}`, id), nil
	}

	// Check data-level permission
	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(user)
		if err != nil {
			return "", err
		}
		if !int64SliceContains(bgids, rule.GroupId) {
			return "", fmt.Errorf("forbidden: no access to this alert rule")
		}
	}

	result := alertRuleDetailResult{
		Id:            rule.Id,
		GroupId:       rule.GroupId,
		Name:          rule.Name,
		Note:          rule.Note,
		Severity:      rule.Severity,
		Disabled:      rule.Disabled,
		Cate:          rule.Cate,
		PromQl:        rule.PromQl,
		AppendTags:    rule.AppendTagsJSON,
		Annotations:   rule.AnnotationsJSON,
		RunbookUrl:    rule.RunbookUrl,
		NotifyRuleIds: rule.NotifyRuleIds,
		CreateBy:      rule.CreateBy,
		UpdateBy:      rule.UpdateBy,
	}

	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}
