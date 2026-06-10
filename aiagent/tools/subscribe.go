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
	register(defs.ListAlertSubscribes, listAlertSubscribes)
	register(defs.GetAlertSubscribeDetail, getAlertSubscribeDetail)
	register(defs.CreateAlertSubscribe, createAlertSubscribe)
}

// createAlertSubscribe 落库一条订阅规则。入参 config 是与前端/HTTP API 同构的 AlertSubscribe
// JSON（n9e-create-alert-subscribe skill 文档化了字段形状），直接反序列化进 models.AlertSubscribe，
// 由 AlertSubscribe.Add 内部做 Verify(severities 必填、notify_version 分支校验) + FE2DB + 落库。
// 业务组缺参门同 create_dashboard。
func createAlertSubscribe(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAlertSubscribesAdd); err != nil {
		return "", err
	}

	configJSON := getArgString(args, "config")
	if configJSON == "" {
		return "", fmt.Errorf("config is required: a JSON object describing the subscribe (name, severities, tags, notify_version, notify_rule_ids, ...); load the n9e-create-alert-subscribe skill for the exact shape")
	}

	var sub models.AlertSubscribe
	if err := json.Unmarshal([]byte(configJSON), &sub); err != nil {
		return "", fmt.Errorf("invalid config JSON: %v", err)
	}

	// 业务组缺参门：config 没带 group_id 就回退表单/页面注入的 busi_group_id，仍缺则弹表单。
	groupId := sub.GroupId
	if groupId == 0 {
		groupId = resolveCreationGroupID(args, params)
	}
	if groupId == 0 {
		return "", creationFormInterrupt(deps, user, "n9e-create-alert-subscribe", []string{"busi_group_id"})
	}
	sub.GroupId = groupId

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

	// datasource_ids 缺省为 [0]：同 create_alert_mute，空数组在引擎里已等价"全部"（MatchCluster
	// 见 DatasourceIdsJson 为空即返回 true），[0] 才是 FE 表示"全部数据源"的标准哨兵——规范化
	// 成它，落库后前端能正确回显"全部"。
	if len(sub.DatasourceIdsJson) == 0 {
		sub.DatasourceIdsJson = []int64{0}
	}

	sub.Id = 0 // 防止模型把 id 塞进 config 导致主键冲突
	sub.CreateBy = user.Username
	sub.UpdateBy = user.Username

	if err := sub.Add(deps.DBCtx); err != nil {
		return "", fmt.Errorf("failed to create alert subscribe: %v", err)
	}

	logger.Infof("create_alert_subscribe: user=%s, group_id=%d, name=%s, id=%d", user.Username, groupId, sub.Name, sub.Id)

	result := map[string]interface{}{
		"id":              sub.Id,
		"name":            sub.Name,
		"group_id":        sub.GroupId,
		"group_name":      bg.Name,
		"disabled":        sub.Disabled,
		"notify_rule_ids": sub.NotifyRuleIds,
	}
	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}

func listAlertSubscribes(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAlertSubscribes); err != nil {
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

	var subs []models.AlertSubscribe
	if isAdmin {
		subs, err = models.AlertSubscribeGetsByBGIds(deps.DBCtx, nil)
	} else {
		if len(bgids) == 0 {
			return marshalList(0, []alertSubscribeResult{}), nil
		}
		subs, err = models.AlertSubscribeGetsByBGIds(deps.DBCtx, bgids)
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

func getAlertSubscribeDetail(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAlertSubscribes); err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	sub, err := models.AlertSubscribeGet(deps.DBCtx, "id=?", id)
	if err != nil {
		return "", fmt.Errorf("failed to get alert subscribe: %v", err)
	}
	if sub == nil {
		return fmt.Sprintf(`{"error":"alert subscribe not found: id=%d"}`, id), nil
	}

	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(deps, user)
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
