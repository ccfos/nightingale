package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
	register(defs.CreateNotifyRule, createNotifyRule)
	register(defs.UpdateNotifyRule, updateNotifyRule)
}

// createNotifyRule 落库一条通知规则。入参 config 是与前端/HTTP API 同构的 NotifyRule
// JSON（notify-rule-copilot skill 文档化了字段形状），直接反序列化进 models.NotifyRule，
// 由 NotifyRule.Verify 做业务校验。通知规则没有业务组维度，权限挂在团队(UserGroup)上：
// config 未带 user_group_ids 时回退表单注入的 team_ids，仍缺则经缺参门弹团队选择表单。
func createNotifyRule(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermNotificationRulesAdd); err != nil {
		return "", err
	}

	configJSON := getArgString(args, "config")
	if configJSON == "" {
		return "", fmt.Errorf("config is required: a JSON object describing the notify rule (name, user_group_ids, notify_configs); load the notify-rule-copilot skill for the exact shape")
	}

	var rule models.NotifyRule
	if err := json.Unmarshal([]byte(configJSON), &rule); err != nil {
		return "", fmt.Errorf("invalid config JSON: %v", err)
	}

	// team(用户组) 缺参门：config 没带就回退表单/页面注入的 team_ids，仍缺则弹团队选择表单
	// 而不是替用户瞎选——和 create_dashboard 的业务组缺参门同一套前端契约。
	rule.UserGroupIds = resolveCreationTeamIDs(rule.UserGroupIds, params)
	if len(rule.UserGroupIds) == 0 {
		return "", creationFormInterrupt(deps, user, "notify-rule-copilot", []string{"team_ids"})
	}

	if rule.Name == "" {
		return "", fmt.Errorf("name is required in config")
	}

	// 非管理员只能建绑定到自己所属团队的规则（与 notifyRulesAdd 路由的越权校验一致）。
	if !user.IsAdmin() {
		myGroupIds, err := getUserGroupIds(deps, user.Id)
		if err != nil {
			return "", err
		}
		if !int64SlicesOverlap(myGroupIds, rule.UserGroupIds) {
			return "", fmt.Errorf("forbidden: you can only create notify rules bound to teams you belong to")
		}
	}

	// enable 缺省为 true：用户明确要创建规则，建出来就该是启用态（与前端默认一致）；
	// 只有 config 顶层显式写了 enable 才尊重其取值（按顶层 key 判断，避免标签名恰为
	// enable 时误判）。
	var topLevel map[string]json.RawMessage
	if json.Unmarshal([]byte(configJSON), &topLevel) == nil {
		if _, ok := topLevel["enable"]; !ok {
			rule.Enable = true
		}
	}

	// template_id 缺省时按渠道 ident 反查默认消息模板回填：普通渠道 template_id=0 会被
	// dispatch 以 "message_template not found" 直接丢通知，规则建出来却发不出（见 fillDefaultTemplates）。
	fillDefaultTemplates(deps, &rule)

	if err := rule.Verify(); err != nil {
		return "", fmt.Errorf("invalid notify rule: %v", err)
	}

	now := time.Now().Unix()
	rule.ID = 0 // 防止模型把 id 塞进 config 导致主键冲突
	rule.CreateBy = user.Username
	rule.CreateAt = now
	rule.UpdateBy = user.Username
	rule.UpdateAt = now

	if err := models.Insert(deps.DBCtx, &rule); err != nil {
		return "", fmt.Errorf("failed to create notify rule: %v", err)
	}

	logger.Infof("create_notify_rule: user=%s, name=%s, teams=%v, configs=%d, id=%d",
		user.Username, rule.Name, rule.UserGroupIds, len(rule.NotifyConfigs), rule.ID)

	result := map[string]interface{}{
		"id":                   rule.ID,
		"name":                 rule.Name,
		"enable":               rule.Enable,
		"user_group_ids":       rule.UserGroupIds,
		"notify_configs_count": len(rule.NotifyConfigs),
		// 站内配置页相对路径。最终回复以 [name](url) 链接展示规则名，用户可点击直达。
		"url": fmt.Sprintf("/notification-rules/edit/%d", rule.ID),
	}
	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}

// updateNotifyRule 增量修改一条通知规则：config(只含要改的字段)二次 Unmarshal 到现有规则
// 副本上（encoding/json 只覆盖 JSON 里出现的字段；notify_configs/user_group_ids 整体替换）。
// 权限对齐 notifyRulePut 路由：非管理员须与现有规则的授权团队有交集；在此之上，改
// user_group_ids 时新列表也必须含自己所属团队——否则可以把规则转绑到任意团队或把自己锁出去。
// 两阶段写（见 update_proposal.go）：首次调用是提案——算出改动、向用户展示确认文案并中断，
// 不写库；用户确认后运行时以 ResumeArgs 重放本工具走 confirm 腿，门禁通过才真正落库
// （NotifyRule.Update 内部 Verify，保 ID/CreateAt/CreateBy）。
func updateNotifyRule(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermNotificationRulesPut); err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	// NotifyRuleGet 不存在时返回 nil,nil；GetNotifyRule 用 First，不存在时以
	// ErrRecordNotFound 报错，走不到下面的 not-found 分支。
	existing, err := models.NotifyRuleGet(deps.DBCtx, "id = ?", id)
	if err != nil {
		return "", fmt.Errorf("failed to get notify rule: %v", err)
	}
	if existing == nil {
		return fmt.Sprintf(`{"error":"notify rule not found: id=%d"}`, id), nil
	}
	// 提案基线必须在 merge 之前取：merge 的 Unmarshal 会复用 existing 切片的底层数组。
	baseline := updateBaselineHash(existing)

	// 数据级权限同样须在 merge 之前做：之后 existing.UserGroupIds 可能已被新值覆写，
	// 再校验等于没校验。
	var myGroupIds []int64
	if !user.IsAdmin() {
		myGroupIds, err = getUserGroupIds(deps, user.Id)
		if err != nil {
			return "", err
		}
		if !int64SlicesOverlap(myGroupIds, existing.UserGroupIds) {
			return "", fmt.Errorf("forbidden: no access to this notify rule")
		}
	}
	subject := fmt.Sprintf("通知规则 **%s**（id=%d）", existing.Name, id)

	configJSON := getArgString(args, "config")
	if configJSON == "" {
		return "", fmt.Errorf("config is required: a JSON object with only the fields to change; see update_notify_rule tool description")
	}
	patch, changed, err := configPatch(configJSON)
	if err != nil {
		return "", err
	}
	if len(changed) == 0 {
		return "", fmt.Errorf("nothing to update: config contains no changeable fields")
	}

	merged := *existing
	if err := json.Unmarshal([]byte(configJSON), &merged); err != nil {
		return "", fmt.Errorf("invalid config JSON: %v", err)
	}
	merged.ID = existing.ID

	if _, ok := patch["user_group_ids"]; ok && !user.IsAdmin() {
		if !int64SlicesOverlap(myGroupIds, merged.UserGroupIds) {
			return "", fmt.Errorf("forbidden: the new user_group_ids must keep at least one team you belong to")
		}
	}

	// 新增/改动的通知配置缺 template_id 时按渠道默认模板兜底，同 createNotifyRule。
	// 仅在 patch 触及 notify_configs 时兜底：存量 template_id=0 的配置（API 建的历史
	// 数据，dispatch 一直静默丢弃）不能借一次不相关的更新被回填激活——那是确认清单
	// 之外的写入，也可能突然放出一条用户已习惯沉默的通知通路。
	if _, ok := patch["notify_configs"]; ok {
		fillDefaultTemplates(deps, &merged)
	}

	// 提案前整体校验，别让用户确认完才发现 config 不合法。
	if err := merged.Verify(); err != nil {
		return "", fmt.Errorf("invalid config: %v", err)
	}

	changeDescs := describePatchChanges(patch, changed)

	// confirm 腿：用户确认后运行时以 ResumeArgs 重放本工具。基线哈希保证此刻重算出的
	// merged 与提案时展示的一致。
	confirmed := getArgBool(args, "confirmed")
	if proposalID := getArgString(args, "proposal_id"); confirmed || proposalID != "" {
		if _, err := confirmUpdateGate(ctx, deps, params, "update_notify_rule", "notify_rule", id, proposalID, confirmed, baseline); err != nil {
			return "", err
		}

		merged.UpdateBy = user.Username
		if err := existing.Update(deps.DBCtx, merged); err != nil {
			return "", fmt.Errorf("failed to update notify rule: %v", err)
		}

		logger.Infof("update_notify_rule: user=%s, id=%d, name=%s, applied changes=%v", user.Username, id, merged.Name, changed)

		result := map[string]interface{}{
			"id":                   existing.ID,
			"name":                 merged.Name,
			"enable":               merged.Enable,
			"user_group_ids":       merged.UserGroupIds,
			"notify_configs_count": len(merged.NotifyConfigs),
			"updated":              changed,
			// changes(人类可读) + applied + name 是确认回执渲染契约
			// （router_ai_interrupt.go formatResumeResult）。
			"changes": changeDescs,
			"applied": true,
			// 站内配置页相对路径。最终回复以 [name](url) 链接展示规则名，用户可点击直达。
			"url": fmt.Sprintf("/notification-rules/edit/%d", existing.ID),
		}
		bytes, _ := json.Marshal(result)
		return string(bytes), nil
	}

	// propose 腿：展示改动清单并中断等用户确认，不写库。
	logger.Infof("update_notify_rule: user=%s, id=%d, proposed changes=%v", user.Username, id, changed)
	return proposeUpdate(ctx, deps, params, &updateProposal{
		Kind:         "notify_rule",
		TargetID:     id,
		BaselineHash: baseline,
		Changes:      changeDescs,
	}, renderUpdateProposalPrompt(subject, changeDescs), map[string]interface{}{
		"id":     id,
		"config": configJSON,
	})
}

// fillDefaultTemplates 给 notify_config 补默认消息模板：template_id 缺省(0)时，按 channel_id
// 反查渠道拿到 ident，再取该 ident 下 weight 最小的消息模板回填——与前端"选完渠道自动选第一个
// 模板"(FE TemplateSelect)完全同路。普通渠道 template_id=0 时 dispatch 会以
// "message_template not found" 直接丢通知，所以即便模型/用户没填也必须兜底。
// flashduty/pagerduty 渠道本就不需要模板，跳过。渠道或模板查不到不致命：保持 0 并记 warning，
// 交回 Verify/dispatch 处理，不阻断创建。
func fillDefaultTemplates(deps *aiagent.ToolDeps, rule *models.NotifyRule) {
	chCache := make(map[int64]*models.NotifyChannelConfig) // channel_id -> channel(可能为 nil 表示查不到)
	tplCache := make(map[string]int64)                     // channel ident -> 默认模板 id(0 表示无)

	for i := range rule.NotifyConfigs {
		nc := &rule.NotifyConfigs[i]
		if nc.TemplateID != 0 || nc.ChannelID <= 0 {
			continue
		}

		ch, ok := chCache[nc.ChannelID]
		if !ok {
			// enabled=-1：禁用渠道也要能反查 ident（规则可引用禁用渠道，模板归属与启用态无关）
			chs, err := models.NotifyChannelGets(deps.DBCtx, nc.ChannelID, "", "", -1)
			if err != nil {
				logger.Warningf("create_notify_rule: load channel %d for default template failed: %v", nc.ChannelID, err)
				continue
			}
			if len(chs) > 0 {
				ch = chs[0]
			}
			chCache[nc.ChannelID] = ch
		}
		if ch == nil {
			continue
		}
		if ch.RequestType == "flashduty" || ch.RequestType == "pagerduty" {
			continue
		}

		tplID, ok := tplCache[ch.Ident]
		if !ok {
			tpls, err := models.MessageTemplatesGetBy(deps.DBCtx, []string{ch.Ident})
			if err != nil {
				logger.Warningf("create_notify_rule: load templates for channel ident %q failed: %v", ch.Ident, err)
			} else if len(tpls) > 0 {
				tplID = tpls[0].ID // MessageTemplatesGetBy 已按 weight asc 排序，首个即默认
			}
			tplCache[ch.Ident] = tplID
		}
		if tplID != 0 {
			nc.TemplateID = tplID
		} else {
			logger.Warningf("create_notify_rule: no default message template for channel %d ident %q", nc.ChannelID, ch.Ident)
		}
	}
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
