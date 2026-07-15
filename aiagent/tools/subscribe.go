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
	register(defs.UpdateAlertSubscribe, updateAlertSubscribe)
}

// createAlertSubscribe 落库一条订阅规则。入参 config 是与前端/HTTP API 同构的 AlertSubscribe
// JSON（alert-subscribe-copilot skill 文档化了字段形状），直接反序列化进 models.AlertSubscribe，
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
		return "", fmt.Errorf("config is required: a JSON object describing the subscribe (name, severities, tags, notify_version, notify_rule_ids, ...); load the alert-subscribe-copilot skill for the exact shape")
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
		return "", creationFormInterrupt(params["lang"], deps, user, "alert-subscribe-copilot", []string{"busi_group_id"})
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
		// 站内配置页相对路径。最终回复以 [name](url) 链接展示规则名，用户可点击直达。
		"url": fmt.Sprintf("/alert-subscribes/edit/%d", sub.Id),
	}
	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}

// updateAlertSubscribe 增量修改一条订阅规则：config(只含要改的字段)二次 Unmarshal 到 DB2FE
// 后的现有规则副本上（encoding/json 只覆盖 JSON 里出现的字段，数组整体替换）。
// 两阶段写（见 update_proposal.go）：首次调用是提案——算出改动、向用户展示确认文案并中断，
// 不写库；用户确认后运行时以 ResumeArgs 重放本工具走 confirm 腿，门禁通过才真正落库。
// 落库走 AlertSubscribe.UpdateFull 整行替换（内部 Verify + FE2DB，并保 Id/GroupId/CreateAt/
// CreateBy）；订阅不支持跨业务组移动。
func updateAlertSubscribe(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	// 对齐 FE 编辑路由 PUT /busi-group/:id/alert-subscribes 的 perm("/alert-subscribes/put")。
	if err := checkPerm(deps, user, PermAlertSubscribesPut); err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	existing, err := models.AlertSubscribeGet(deps.DBCtx, "id=?", id)
	if err != nil {
		return "", fmt.Errorf("failed to get alert subscribe: %v", err)
	}
	if existing == nil {
		return fmt.Sprintf(`{"error":"alert subscribe not found: id=%d"}`, id), nil
	}
	// merge 底座必须是 FE 形态：datasource_ids/webhooks/extra_config/severities 在 DB 行里是
	// 序列化串，不先 DB2FE，未修改的这些字段会在 Update 的 FE2DB 里被空 FE 值清掉。
	if err := existing.DB2FE(); err != nil {
		return "", fmt.Errorf("failed to parse alert subscribe: %v", err)
	}
	// 提案基线必须在 merge 之前取：merge 的 Unmarshal 会复用 existing 切片的底层数组。
	baseline := updateBaselineHash(existing)

	// 数据级权限同 createAlertSubscribe：要求对规则所属业务组有 rw。
	bg, err := models.BusiGroupGetById(deps.DBCtx, existing.GroupId)
	if err != nil {
		return "", fmt.Errorf("failed to get busi group: %v", err)
	}
	if bg == nil {
		return "", fmt.Errorf("busi group not found: id=%d", existing.GroupId)
	}
	if err := checkBgRW(deps, user, bg); err != nil {
		return "", err
	}

	configJSON := getArgString(args, "config")
	if configJSON == "" {
		return "", fmt.Errorf("config is required: a JSON object with only the fields to change; see update_alert_subscribe tool description")
	}
	patch, changed, err := configPatch(configJSON)
	if err != nil {
		return "", err
	}
	if err := patchGroupIDGuard(patch, existing.GroupId); err != nil {
		return "", err
	}
	if err := rejectEmptyArrayPatch(patch, "webhooks"); err != nil {
		return "", err
	}
	if len(changed) == 0 {
		return "", fmt.Errorf("nothing to update: config contains no changeable fields")
	}

	merged := *existing
	if err := json.Unmarshal([]byte(configJSON), &merged); err != nil {
		return "", fmt.Errorf("invalid config JSON: %v", err)
	}
	merged.Id = existing.Id
	merged.GroupId = existing.GroupId
	// rule_ids 已取代 rule_id：与 alertSubscribePut 路由一样置 0，防遗留的旧字段引发误订阅。
	merged.RuleId = 0
	// 空 datasource_ids 规范化为 [0]（FE 的"全部数据源"哨兵），同 createAlertSubscribe。
	if len(merged.DatasourceIdsJson) == 0 {
		merged.DatasourceIdsJson = []int64{0}
	}

	// 提案前整体校验（Verify 会就地改写副本，用 check 隔离），别让用户确认完才发现 config 不合法。
	check := merged
	if err := check.Verify(); err != nil {
		return "", fmt.Errorf("invalid config: %v", err)
	}

	changeDescs := describePatchChanges(patch, changed)

	// confirm 腿：用户确认后运行时以 ResumeArgs 重放本工具。基线哈希保证此刻重算出的
	// merged 与提案时展示的一致。
	confirmed := getArgBool(args, "confirmed")
	if proposalID := getArgString(args, "proposal_id"); confirmed || proposalID != "" {
		if _, err := confirmUpdateGate(ctx, deps, params, "update_alert_subscribe", "alert_subscribe", id, proposalID, confirmed, baseline); err != nil {
			return "", err
		}

		merged.UpdateBy = user.Username

		if err := existing.UpdateFull(deps.DBCtx, merged); err != nil {
			return "", fmt.Errorf("failed to update alert subscribe: %v", err)
		}

		logger.Infof("update_alert_subscribe: user=%s, id=%d, group_id=%d, applied changes=%v", user.Username, id, existing.GroupId, changed)

		result := map[string]interface{}{
			"id":              existing.Id,
			"name":            merged.Name,
			"group_id":        existing.GroupId,
			"group_name":      bg.Name,
			"disabled":        merged.Disabled,
			"notify_rule_ids": merged.NotifyRuleIds,
			"updated":         changed,
			// changes(人类可读) + applied + name 是确认回执渲染契约
			// （router_ai_interrupt.go formatResumeResult）。
			"changes": changeDescs,
			"applied": true,
			// 站内配置页相对路径。最终回复以 [name](url) 链接展示规则名，用户可点击直达。
			"url": fmt.Sprintf("/alert-subscribes/edit/%d", existing.Id),
		}
		bytes, _ := json.Marshal(result)
		return string(bytes), nil
	}

	// propose 腿：展示改动清单并中断等用户确认，不写库。
	logger.Infof("update_alert_subscribe: user=%s, id=%d, proposed changes=%v", user.Username, id, changed)
	return proposeUpdate(ctx, deps, params, &updateProposal{
		Kind:         "alert_subscribe",
		TargetID:     id,
		BaselineHash: baseline,
		Changes:      changeDescs,
	}, renderUpdateProposalPrompt(params["lang"], fmt.Sprintf(aiagent.LangText(params["lang"],
		"订阅规则 **%s**（id=%d）", "subscription rule **%s** (id=%d)"), existing.Name, id), changeDescs), map[string]interface{}{
		"id":     id,
		"config": configJSON,
	})
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
