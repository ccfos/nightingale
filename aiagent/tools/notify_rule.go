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
	Id            int64                `json:"id"`
	Name          string               `json:"name"`
	Description   string               `json:"description,omitempty"`
	Enable        bool                 `json:"enable"`
	UserGroupIds  []int64              `json:"user_group_ids,omitempty"`
	NotifyConfigs []notifyConfigResult `json:"notify_configs"`
	CreateBy      string               `json:"create_by,omitempty"`
	UpdateBy      string               `json:"update_by,omitempty"`
}

// notifyConfigResult 暴露单条通知配置里"决定一个事件会不会按这条配置发出去"的匹配维度：
// 渠道（含启用状态）、适用级别、生效时段、标签/属性过滤。排查"事件产生了却没通知记录"时，
// 需要拿这些条件去和事件标签/级别/触发时刻逐一比对。Params 含 token/手机号等敏感信息，不暴露。
type notifyConfigResult struct {
	ChannelID      int64               `json:"channel_id"`
	ChannelName    string              `json:"channel_name,omitempty"`
	ChannelIdent   string              `json:"channel_ident,omitempty"`
	ChannelEnabled *bool               `json:"channel_enabled,omitempty"` // 渠道本身是否启用；渠道被禁用同样不会产生通知记录
	TemplateID     int64               `json:"template_id,omitempty"`
	Type           string              `json:"type,omitempty"`
	Severities     []int               `json:"severities"`            // 适用告警级别；注意：空数组=匹配不到任何事件（不是"不限"），引擎对空 severities 直接判不匹配
	TimeRanges     []models.TimeRanges `json:"time_ranges,omitempty"` // 适用时段；空=不限时段（匹配全部）；非空且事件触发时刻不在任一时段内则不发
	LabelKeys      []tagFilterResult   `json:"label_keys"`            // 标签过滤；空=不按标签过滤（匹配全部）
	Attributes     []tagFilterResult   `json:"attributes,omitempty"`  // 属性过滤
}

type tagFilterResult struct {
	Key   string      `json:"key"`
	Op    string      `json:"op"`
	Value interface{} `json:"value"`
}

func mapTagFilters(in []models.TagFilter) []tagFilterResult {
	out := make([]tagFilterResult, 0, len(in))
	for _, t := range in {
		op := t.Op
		if op == "" {
			op = t.Func
		}
		out = append(out, tagFilterResult{Key: t.Key, Op: op, Value: t.Value})
	}
	return out
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

	// 解析渠道：一次性把全部渠道捞出来建索引，给每条通知配置补上渠道名/类型/启用状态。
	// 用 enabled=-1 把禁用渠道也捞进来（NotifyChannelGetsAll 只返回启用的），否则没法把
	// channel_enabled 报成 false——而渠道被禁用恰恰是"通知发不出"的常见原因之一。
	// 渠道查询失败不致命——退化为只返回 channel_id。
	chMap := make(map[int64]*models.NotifyChannelConfig)
	if channels, cerr := models.NotifyChannelGets(deps.DBCtx, 0, "", "", -1); cerr == nil {
		for _, ch := range channels {
			chMap[ch.ID] = ch
		}
	} else {
		logger.Warningf("get_notify_rule_detail: load notify channels failed: %v", cerr)
	}

	configs := make([]notifyConfigResult, 0, len(rule.NotifyConfigs))
	for _, nc := range rule.NotifyConfigs {
		cr := notifyConfigResult{
			ChannelID:  nc.ChannelID,
			TemplateID: nc.TemplateID,
			Type:       nc.Type,
			Severities: nc.Severities,
			TimeRanges: nc.TimeRanges,
			LabelKeys:  mapTagFilters(nc.LabelKeys),
			Attributes: mapTagFilters(nc.Attributes),
		}
		if ch, ok := chMap[nc.ChannelID]; ok {
			cr.ChannelName = ch.Name
			cr.ChannelIdent = ch.Ident
			enabled := ch.Enable
			cr.ChannelEnabled = &enabled
		}
		configs = append(configs, cr)
	}

	result := notifyRuleDetailResult{
		Id:            rule.ID,
		Name:          rule.Name,
		Description:   rule.Description,
		Enable:        rule.Enable,
		UserGroupIds:  rule.UserGroupIds,
		NotifyConfigs: configs,
		CreateBy:      rule.CreateBy,
		UpdateBy:      rule.UpdateBy,
	}

	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}
