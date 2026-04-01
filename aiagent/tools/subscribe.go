package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

type alertSubscribeResult struct {
	Id       int64  `json:"id"`
	Name     string `json:"name"`
	GroupId  int64  `json:"group_id"`
	Disabled int    `json:"disabled"`
	Note     string `json:"note,omitempty"`
	CreateBy string `json:"create_by,omitempty"`
}

type alertSubscribeDetailResult struct {
	Id               int64       `json:"id"`
	Name             string      `json:"name"`
	GroupId          int64       `json:"group_id"`
	Disabled         int         `json:"disabled"`
	Note             string      `json:"note,omitempty"`
	Cate             string      `json:"cate,omitempty"`
	Prod             string      `json:"prod,omitempty"`
	Tags             interface{} `json:"tags,omitempty"`
	RedefineSeverity int         `json:"redefine_severity"`
	NewSeverity      int         `json:"new_severity,omitempty"`
	RuleIds          []int64     `json:"rule_ids,omitempty"`
	NotifyRuleIds    []int64     `json:"notify_rule_ids,omitempty"`
	CreateBy         string      `json:"create_by,omitempty"`
	UpdateBy         string      `json:"update_by,omitempty"`
}

func init() {
	register("list_alert_subscribes", aiagent.AgentTool{
		Name:        "list_alert_subscribes",
		Description: "查询当前用户有权限的告警订阅规则列表",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters: []aiagent.ToolParameter{
			{Name: "query", Type: "string", Description: "搜索关键词，匹配订阅名称", Required: false},
			{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
		},
	}, listAlertSubscribes)

	register("get_alert_subscribe_detail", aiagent.AgentTool{
		Name:        "get_alert_subscribe_detail",
		Description: "获取单条告警订阅规则的详细信息",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters: []aiagent.ToolParameter{
			{Name: "id", Type: "integer", Description: "订阅规则ID", Required: true},
		},
	}, getAlertSubscribeDetail)
}

func listAlertSubscribes(_ context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(user, PermAlertSubscribes); err != nil {
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
	var subs []models.AlertSubscribe
	if isAdmin {
		subs, err = models.AlertSubscribeGetsByBGIds(dbCtx, nil)
	} else {
		if len(bgids) == 0 {
			return marshalList(0, []alertSubscribeResult{}), nil
		}
		subs, err = models.AlertSubscribeGetsByBGIds(dbCtx, bgids)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query alert subscribes: %v", err)
	}

	results := make([]alertSubscribeResult, 0)
	for _, s := range subs {
		if query != "" && !containsIgnoreCase(s.Name, query) {
			continue
		}
		results = append(results, alertSubscribeResult{
			Id:       s.Id,
			Name:     s.Name,
			GroupId:  s.GroupId,
			Disabled: s.Disabled,
			Note:     s.Note,
			CreateBy: s.CreateBy,
		})
		if len(results) >= limit {
			break
		}
	}

	logger.Debugf("list_alert_subscribes: user_id=%d, found %d subscribes", user.Id, len(results))
	return marshalList(len(results), results), nil
}

func getAlertSubscribeDetail(_ context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(user, PermAlertSubscribes); err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	dbCtx := aiagent.GetDBCtx()
	sub, err := models.AlertSubscribeGet(dbCtx, "id=?", id)
	if err != nil {
		return "", fmt.Errorf("failed to get alert subscribe: %v", err)
	}
	if sub == nil {
		return fmt.Sprintf(`{"error":"alert subscribe not found: id=%d"}`, id), nil
	}

	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(user)
		if err != nil {
			return "", err
		}
		if !int64SliceContains(bgids, sub.GroupId) {
			return "", fmt.Errorf("forbidden: no access to this alert subscribe")
		}
	}

	result := alertSubscribeDetailResult{
		Id:               sub.Id,
		Name:             sub.Name,
		GroupId:          sub.GroupId,
		Disabled:         sub.Disabled,
		Note:             sub.Note,
		Cate:             sub.Cate,
		Prod:             sub.Prod,
		Tags:             sub.Tags,
		RedefineSeverity: sub.RedefineSeverity,
		NewSeverity:      sub.NewSeverity,
		RuleIds:          sub.RuleIds,
		NotifyRuleIds:    sub.NotifyRuleIds,
		CreateBy:         sub.CreateBy,
		UpdateBy:         sub.UpdateBy,
	}

	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}
