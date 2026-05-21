package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
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
	register(defs.ListAlertRules, listAlertRules)
	register(defs.GetAlertRuleDetail, getAlertRuleDetail)
	register(defs.CreateAlertRule, createAlertRule)
	register(defs.ListLegacyNotifyAlertRules, listLegacyNotifyAlertRules)
	register(defs.ImportPromRuleYAML, importPromRuleYAML)
	register(defs.PreviewPromRuleYAML, previewPromRuleYAML)
}

// legacyAlertRuleResult 是 list_legacy_notify_alert_rules 的返回单元。
// 与 alertRuleResult 拆开是有意为之：审计场景需要看到老/新两侧的具体值
// （NotifyGroups / NotifyRuleIds），日常列表场景不应暴露 deprecated 字段。
type legacyAlertRuleResult struct {
	Id            int64    `json:"id"`
	GroupId       int64    `json:"group_id"`
	GroupName     string   `json:"group_name,omitempty"`
	Name          string   `json:"name"`
	Severity      int      `json:"severity"`
	Disabled      int      `json:"disabled"`
	Cate          string   `json:"cate,omitempty"`
	NotifyVersion int      `json:"notify_version"`
	NotifyGroups  []string `json:"notify_groups"`            // 老式接收组（user_group id 字符串列表），空数组表示该规则虽然标 v0 但没人收
	NotifyRuleIds []int64  `json:"notify_rule_ids"`          // 新式通知规则（v0 规则下应该为空，列出来便于人工核对）
	UpdateAt      int64    `json:"update_at"`
	UpdateBy      string   `json:"update_by,omitempty"`
}

// listLegacyNotifyAlertRules scans alert rules still on the legacy notify path
// (notify_version=0). Single-shot — does not paginate or follow up with detail
// calls. The summary fields are precomputed so the LLM doesn't have to bucket
// the items itself.
func listLegacyNotifyAlertRules(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAlertRules); err != nil {
		return "", err
	}

	bgids, isAdmin, err := getUserBgids(deps, user)
	if err != nil {
		return "", err
	}

	includeDisabled := false
	if v, ok := args["include_disabled"].(bool); ok {
		includeDisabled = v
	}
	filterGroupId := getArgInt64(args, "group_id")
	limit := getArgInt(args, "limit", 500)
	if limit > 2000 {
		limit = 2000
	}

	// Permission gating: same shape as listAlertRules — admin sees all, others
	// restricted to their bgids. If filterGroupId is set, intersect with that.
	var scopeBgids []int64
	if isAdmin {
		if filterGroupId > 0 {
			scopeBgids = []int64{filterGroupId}
		}
	} else {
		if len(bgids) == 0 {
			return marshalList(0, []legacyAlertRuleResult{}), nil
		}
		if filterGroupId > 0 {
			if !int64SliceContains(bgids, filterGroupId) {
				return "", fmt.Errorf("forbidden: no access to busi group %d", filterGroupId)
			}
			scopeBgids = []int64{filterGroupId}
		} else {
			scopeBgids = bgids
		}
	}

	rules, err := models.AlertRuleGetsLegacyNotifyByBGIds(deps.DBCtx, scopeBgids, includeDisabled)
	if err != nil {
		return "", fmt.Errorf("failed to query legacy alert rules: %v", err)
	}

	// Cache busi groups so we can fill GroupName without N+1 lookups.
	bgCache, _ := models.BusiGroupGetMap(deps.DBCtx)

	results := make([]legacyAlertRuleResult, 0)
	var enabled, disabled, withGroups, emptyLegacy int

	for i := range rules {
		r := rules[i]

		groups := r.NotifyGroupsJSON
		if groups == nil {
			groups = []string{}
		}
		ruleIds := r.NotifyRuleIds
		if ruleIds == nil {
			ruleIds = []int64{}
		}

		item := legacyAlertRuleResult{
			Id:            r.Id,
			GroupId:       r.GroupId,
			Name:          r.Name,
			Severity:      r.Severity,
			Disabled:      r.Disabled,
			Cate:          r.Cate,
			NotifyVersion: r.NotifyVersion,
			NotifyGroups:  groups,
			NotifyRuleIds: ruleIds,
			UpdateAt:      r.UpdateAt,
			UpdateBy:      r.UpdateBy,
		}
		if bg, ok := bgCache[r.GroupId]; ok && bg != nil {
			item.GroupName = bg.Name
		}

		if r.Disabled == 0 {
			enabled++
		} else {
			disabled++
		}
		if len(groups) > 0 {
			withGroups++
		} else {
			emptyLegacy++
		}

		results = append(results, item)
		if len(results) >= limit {
			break
		}
	}

	logger.Debugf("list_legacy_notify_alert_rules: user_id=%d include_disabled=%v group_id=%d found=%d",
		user.Id, includeDisabled, filterGroupId, len(results))

	payload, _ := json.Marshal(map[string]interface{}{
		"total": len(results),
		"items": results,
		"summary": map[string]int{
			"total":                  len(results),
			"enabled":                enabled,
			"disabled":               disabled,
			"with_groups_configured": withGroups,
			"empty_legacy":           emptyLegacy,
		},
		"note": "notify_version=0 即老版本（写入时与 notify_rule_ids 互斥）。empty_legacy 是 v0 但 notify_groups 也空的，等于谁都不通知，建议优先治理。",
	})
	return string(payload), nil
}

// prodForCate returns the default "prod" value for a given datasource cate,
// matching the frontend/service-API conventions. Callers can override via
// the explicit prod parameter when needed (e.g. clickhouse can be either
// metric or logging depending on the query).
func prodForCate(cate string) string {
	switch cate {
	case "prometheus", "mysql", "pgsql", "tdengine", "ck":
		return "metric"
	case "loki", "elasticsearch", "opensearch", "victorialogs", "doris":
		return "logging"
	case "host":
		return "host"
	}
	return "metric"
}

// isValidCate reports whether a cate is one of the supported datasource
// types. An unknown cate would likely be rejected downstream anyway, but
// catching it at the tool boundary gives a clearer error message.
func isValidCate(cate string) bool {
	switch cate {
	case "prometheus", "loki", "elasticsearch", "opensearch", "tdengine",
		"ck", "mysql", "pgsql", "doris", "victorialogs", "host":
		return true
	}
	return false
}

func listAlertRules(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAlertRules); err != nil {
		return "", err
	}

	bgids, isAdmin, err := getUserBgids(deps, user)
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

	var rules []models.AlertRule
	if isAdmin {
		rules, err = models.AlertRuleGetsByBGIds(deps.DBCtx, nil)
	} else {
		if len(bgids) == 0 {
			return marshalList(0, []alertRuleResult{}), nil
		}
		rules, err = models.AlertRuleGetsByBGIds(deps.DBCtx, bgids)
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

func getAlertRuleDetail(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAlertRules); err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	rule, err := models.AlertRuleGetById(deps.DBCtx, id)
	if err != nil {
		return "", fmt.Errorf("failed to get alert rule: %v", err)
	}
	if rule == nil {
		return fmt.Sprintf(`{"error":"alert rule not found: id=%d"}`, id), nil
	}

	// Check data-level permission
	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(deps, user)
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

// createAlertRule persists an alert rule via models.AlertRule.Add. It
// supports two input modes:
//
//  1. Simple Prometheus threshold path — caller supplies prom_ql +
//     threshold + operator, and the tool synthesises a v2 rule_config
//     (queries + triggers + expressions) automatically. This is the
//     common case for "alert when CPU > 80" style requests.
//
//  2. Generic path — caller supplies cate (e.g. "mysql", "loki", "host")
//     and a pre-built rule_config_json string. The LLM is expected to
//     read skill/n9e-create-alert-rule/datasources/<cate>.md via the
//     read_file builtin tool to get the exact structure for that type,
//     assemble it, and pass it through. This keeps the tool small while
//     covering every datasource type the platform supports.
//
// Host type is special-cased: it has no datasource, so datasource_id is
// optional and datasource_queries is left empty.
func createAlertRule(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	// Match the FE route: /alert-rules/add role permission + bgrw on the group.
	if err := checkPerm(deps, user, PermAlertRulesAdd); err != nil {
		return "", err
	}

	groupId := getArgInt64(args, "group_id")
	if groupId == 0 {
		return "", fmt.Errorf("group_id is required")
	}

	name := getArgString(args, "name")
	if name == "" {
		return "", fmt.Errorf("name is required")
	}

	cate := getArgString(args, "cate")
	if cate == "" {
		cate = "prometheus"
	}
	if !isValidCate(cate) {
		return "", fmt.Errorf("invalid cate %q (allowed: prometheus|loki|elasticsearch|opensearch|tdengine|ck|mysql|pgsql|doris|victorialogs|host)", cate)
	}

	prod := getArgString(args, "prod")
	if prod == "" {
		prod = prodForCate(cate)
	}

	severity := getArgInt(args, "severity", 2)
	if severity < 1 || severity > 3 {
		return "", fmt.Errorf("severity must be 1 (critical), 2 (warning) or 3 (info)")
	}

	evalInterval := getArgInt(args, "eval_interval", 30)
	forDuration := getArgInt(args, "for_duration", 60)

	// Verify business group exists and the user has rw permission on it.
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

	// Datasource is required for everything except host type. Prefer the
	// explicit tool arg; fall back to preflight/page-injected params so the
	// LLM doesn't have to re-thread datasource_id through every call. Mirrors
	// createDashboard so "创建告警规则" with a preselected datasource works
	// even when the LLM omits it from Action Input.
	dsId := getArgInt64(args, "datasource_id")
	if dsId == 0 {
		dsId = getDatasourceId(params)
	}
	if cate != "host" && dsId == 0 {
		return "", fmt.Errorf("datasource_id is required for cate=%s", cate)
	}

	// Determine rule_config: either synthesise from the simple prometheus
	// path, or accept a raw JSON object from the caller for any other type.
	var ruleConfig interface{}
	var simplePromQL, simpleOp string
	var simpleThreshold float64

	rawRuleConfig := getArgString(args, "rule_config_json")
	if rawRuleConfig != "" {
		// Generic path — trust the LLM's JSON. Unmarshal into a map so
		// FE2DB can re-marshal it consistently.
		var rc map[string]interface{}
		if err := json.Unmarshal([]byte(rawRuleConfig), &rc); err != nil {
			return "", fmt.Errorf("rule_config_json must be a valid JSON object (got parse error: %v). Re-check the structure against skill/n9e-create-alert-rule/datasources/%s.md", err, cate)
		}
		// Normalize query interval fields. The frontend expects
		// queries[i].interval to be a total number of seconds and does
		// NOT persist interval_unit — on save it calls normalizeTime
		// (which multiplies minutes/hours into seconds) and strips the
		// unit, and on load it calls parseTimeToValueAndUnit to derive
		// the display unit from the raw seconds. If we store the pair
		// verbatim, the FE reads interval=5 and interval_unit=min but
		// its parseTimeToValueAndUnit(5) sees <60 and renders "5 秒",
		// not "5 min". So collapse any `{interval, interval_unit}` pair
		// into a single seconds value here.
		normalizeQueryIntervals(rc)
		ruleConfig = rc
	} else {
		// Simple path — only works for Prometheus threshold rules.
		if cate != "prometheus" {
			return "", fmt.Errorf(
				"for cate=%s, rule_config_json is required. "+
					"First call read_file(base=\"n9e-create-alert-rule\", path=\"datasources/%s.md\") "+
					"to fetch the exact rule_config structure, then pass it as rule_config_json",
				cate, cate)
		}

		promQL := getArgString(args, "prom_ql")
		if promQL == "" {
			return "", fmt.Errorf("prom_ql is required when cate=prometheus and rule_config_json is empty")
		}
		threshold, hasThreshold := getArgFloat(args, "threshold")
		if !hasThreshold {
			return "", fmt.Errorf("threshold is required when cate=prometheus and rule_config_json is empty")
		}
		op := getArgString(args, "operator")
		if op == "" {
			op = ">"
		}
		if !isValidOperator(op) {
			return "", fmt.Errorf("invalid operator %q (allowed: > >= < <= == !=)", op)
		}

		simplePromQL = promQL
		simpleOp = op
		simpleThreshold = threshold

		// Emit v1 rule_config (threshold baked into prom_ql). v2 format would
		// be cleaner conceptually, but the frontend's v2 editor is gated by
		// IS_PLUS — in OSS n9e it's always false, so a v2 rule loads into the
		// v1 form which reads `prom_ql` and finds nothing (v2 uses `query`),
		// producing an empty editor. v1 format is the only one that renders
		// correctly in both editions.
		//
		// Wrap the PromQL in parentheses when it contains operators, so e.g.
		// `a/b > 0.5` parses as `(a/b) > 0.5`, not `a/(b > 0.5)`.
		bakedPromQL := fmt.Sprintf("%s %s %v", wrapIfComplex(promQL), op, threshold)
		ruleConfig = map[string]interface{}{
			"queries": []map[string]interface{}{
				{
					"prom_ql":  bakedPromQL,
					"severity": severity,
				},
			},
		}
	}

	// Optional notify_rule_ids
	var notifyRuleIds []int64
	if raw := getArgString(args, "notify_rule_ids"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &notifyRuleIds); err != nil {
			return "", fmt.Errorf("notify_rule_ids must be a JSON array of integers (got %q): %v", raw, err)
		}
	}

	// Tags: split on whitespace, drop empties.
	var appendTags []string
	if raw := getArgString(args, "append_tags"); raw != "" {
		for _, t := range strings.Fields(raw) {
			if t != "" {
				appendTags = append(appendTags, t)
			}
		}
	}

	// datasource_queries is empty for host (no datasource); otherwise an
	// exact-match query on the supplied datasource_id.
	var dsQueries []models.DatasourceQuery
	var dsIdsJson []int64
	if cate != "host" {
		dsQueries = []models.DatasourceQuery{
			{MatchType: 0, Op: "in", Values: []interface{}{dsId}},
		}
		dsIdsJson = []int64{dsId}
	}

	rule := &models.AlertRule{
		GroupId:               groupId,
		Cate:                  cate,
		Prod:                  prod,
		Name:                  name,
		Note:                  getArgString(args, "note"),
		Severity:              severity,
		Disabled:              0,
		PromEvalInterval:      evalInterval,
		PromForDuration:       forDuration,
		RuleConfigJson:        ruleConfig,
		DatasourceIdsJson:     dsIdsJson,
		DatasourceQueries:     dsQueries,
		EnableInBG:            0,
		EnableStimesJSON:      []string{"00:00"},
		EnableEtimesJSON:      []string{"00:00"},
		EnableDaysOfWeeksJSON: [][]string{{"0", "1", "2", "3", "4", "5", "6"}},
		NotifyRecovered:       1,
		NotifyRepeatStep:      60,
		NotifyMaxNumber:       0,
		NotifyVersion:         1,
		NotifyRuleIds:         notifyRuleIds,
		AppendTagsJSON:        appendTags,
		AnnotationsJSON:       map[string]string{},
		RunbookUrl:            getArgString(args, "runbook_url"),
		CreateBy:              user.Username,
		UpdateBy:              user.Username,
	}

	if err := rule.FE2DB(); err != nil {
		return "", fmt.Errorf("failed to convert rule fields: %v", err)
	}

	if err := rule.Add(deps.DBCtx); err != nil {
		// Map the upstream "AlertRule already exists" error to an instructive
		// message so the LLM retries with a different name immediately
		// instead of querying list_alert_rules to investigate.
		if strings.Contains(err.Error(), "already exists") {
			return "", fmt.Errorf(
				"alert rule name %q already exists in busi_group %d. "+
					"DO NOT call list_alert_rules. "+
					"Retry create_alert_rule immediately with a different name, "+
					"e.g. %q or %q",
				name, groupId, name+"-v2", name+"-AI",
			)
		}
		return "", fmt.Errorf("failed to create alert rule: %v", err)
	}

	logger.Infof("create_alert_rule: user=%s, cate=%s, group_id=%d, name=%s, id=%d",
		user.Username, cate, groupId, name, rule.Id)

	result := map[string]interface{}{
		"id":         rule.Id,
		"group_id":   rule.GroupId,
		"group_name": bg.Name,
		"name":       rule.Name,
		"cate":       rule.Cate,
		"prod":       rule.Prod,
		"severity":   rule.Severity,
		"note":       rule.Note,
	}
	if cate != "host" {
		result["datasource_id"] = dsId
		if ds, dsErr := models.DatasourceGet(deps.DBCtx, dsId); dsErr == nil && ds != nil {
			result["datasource_name"] = ds.Name
		}
	}
	if simplePromQL != "" {
		result["prom_ql"] = simplePromQL
		result["operator"] = simpleOp
		result["threshold"] = simpleThreshold
	}
	if forDuration > 0 {
		result["for_duration"] = forDuration
	}
	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}

// isValidOperator returns true for the comparison operators the n9e
// trigger evaluator understands.
func isValidOperator(op string) bool {
	switch op {
	case ">", ">=", "<", "<=", "==", "!=":
		return true
	}
	return false
}

// normalizeQueryIntervals walks a rule_config map and collapses every
// {interval, interval_unit} query field pair into a single `interval`
// value expressed in total seconds, then deletes `interval_unit`. This
// matches the wire format the frontend expects on load — see the comment
// at the call site for the full context.
//
// Accepted unit strings: "second"/"sec"/"s", "min"/"minute"/"m",
// "hour"/"h", "day"/"d". Unknown units default to "minute" (the FE form
// component's initial value) so the result is at least sensible.
func normalizeQueryIntervals(rc map[string]interface{}) {
	queriesRaw, ok := rc["queries"].([]interface{})
	if !ok {
		return
	}
	for _, q := range queriesRaw {
		qm, ok := q.(map[string]interface{})
		if !ok {
			continue
		}
		unit, hasUnit := qm["interval_unit"].(string)
		intervalRaw, hasInterval := qm["interval"]
		if !hasInterval {
			// Nothing to normalize; still strip a dangling interval_unit.
			delete(qm, "interval_unit")
			continue
		}
		var intervalFloat float64
		switch v := intervalRaw.(type) {
		case float64:
			intervalFloat = v
		case int:
			intervalFloat = float64(v)
		case int64:
			intervalFloat = float64(v)
		default:
			// Unknown type — leave alone but still drop the unit.
			delete(qm, "interval_unit")
			continue
		}
		if hasUnit {
			switch strings.ToLower(unit) {
			case "second", "sec", "s":
				// already seconds
			case "min", "minute", "m":
				intervalFloat *= 60
			case "hour", "h":
				intervalFloat *= 3600
			case "day", "d":
				intervalFloat *= 86400
			default:
				// Unknown unit — assume minutes (the FE form default).
				intervalFloat *= 60
			}
			delete(qm, "interval_unit")
		}
		// If no unit was provided and interval is already >=60, assume it
		// was pre-converted to seconds and leave as-is. If it's <60 and
		// no unit, it was almost certainly minutes (1, 5, 15 are common)
		// — multiply by 60 to match intent. Heuristic, but matches how
		// humans think about alert intervals.
		if !hasUnit && intervalFloat > 0 && intervalFloat < 60 {
			intervalFloat *= 60
		}
		qm["interval"] = int(intervalFloat)
	}
}

// =============================================================================
// import_prom_rule_yaml / preview_prom_rule_yaml
// =============================================================================

type promRulePreviewItem struct {
	Name            string            `json:"name"`
	Severity        int               `json:"severity"`
	PromQl          string            `json:"prom_ql"`
	ForDurationSec  int               `json:"for_duration_sec"`
	EvalIntervalSec int               `json:"eval_interval_sec"`
	AppendTags      []string          `json:"append_tags,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty"`
}

// promRuleImportItem 区分三种结局：
//   - status="created"          ：新建成功，Id 非零
//   - status="skipped_duplicate"：同名规则已存在，未做任何改动；Error 字段为空
//   - status="failed"           ：其他错误（校验失败、DB 异常等）；Error 描述原因
//
// 区分 skipped vs failed 很关键，避免 LLM 看到 "failed: already exists" 就触发
// "用 name_prefix 重试整份 YAML" 的错误纠正动作——那会让没冲突的规则全部多写
// 一遍，造成 N+冲突项 vs 2N 条总量。
type promRuleImportItem struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Id     int64  `json:"id,omitempty"`
	Error  string `json:"error,omitempty"`
}

const (
	promRuleStatusCreated   = "created"
	promRuleStatusSkipped   = "skipped_duplicate"
	promRuleStatusFailed    = "failed"
	promRuleDuplicateErrStr = "AlertRule already exists" // 来自 models.AlertRule.Add
)

// resolvePromRulePayload returns the YAML payload for preview/import. Caller
// must pass exactly one of `payload` (inline text) or `payload_file` (path to
// a temp file produced by http_fetch save_to_file=true). The file path is
// validated via ReadFetchTempFile — must live under os.TempDir and have the
// http-fetch prefix, so the LLM cannot read arbitrary files.
func resolvePromRulePayload(args map[string]interface{}) (string, error) {
	payload := getArgString(args, "payload")
	payloadFile := getArgString(args, "payload_file")
	switch {
	case payload != "" && payloadFile != "":
		return "", fmt.Errorf("pass either payload or payload_file, not both")
	case payload != "":
		return payload, nil
	case payloadFile != "":
		raw, err := ReadFetchTempFile(payloadFile)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	default:
		return "", fmt.Errorf("payload or payload_file is required")
	}
}

// previewPromRuleYAML parses a Prometheus rule YAML payload and returns the
// rules it would produce, without touching the DB. Same parsing path as the
// import handler (models.ParsePromRuleYAML + models.DealPromGroup), so the
// preview can't drift from what import will actually write.
func previewPromRuleYAML(_ context.Context, _ *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	payload, err := resolvePromRulePayload(args)
	if err != nil {
		return "", err
	}

	groups, err := models.ParsePromRuleYAML(payload)
	if err != nil {
		return "", err
	}

	rules := models.DealPromGroup(groups, nil, 0)

	items := make([]promRulePreviewItem, 0, len(rules))
	for _, r := range rules {
		items = append(items, promRulePreviewItem{
			Name:            r.Name,
			Severity:        r.Severity,
			PromQl:          r.PromQl,
			ForDurationSec:  r.PromForDuration,
			EvalIntervalSec: parseCronEverySeconds(r.CronPattern),
			AppendTags:      r.AppendTagsJSON,
			Annotations:     r.AnnotationsJSON,
		})
	}

	return marshalList(len(items), items), nil
}

// importPromRuleYAML parses a Prometheus rule YAML payload and persists each
// rule to the named busi group. Mirrors the permission checks of
// alertRuleAddByImportPromRule (perm /alert-rules/add + bgrw) and reuses the
// same conversion (ParsePromRuleYAML + DealPromGroup) so behavior matches the
// HTTP import endpoint. Returns one entry per rule with id or error so the
// LLM can summarise partial successes.
func importPromRuleYAML(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAlertRulesAdd); err != nil {
		return "", err
	}

	groupId := getArgInt64(args, "group_id")
	if groupId == 0 {
		return "", fmt.Errorf("group_id is required")
	}

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

	rawIds := getArgString(args, "datasource_ids")
	if rawIds == "" {
		return "", fmt.Errorf("datasource_ids is required (JSON array, e.g. \"[1]\")")
	}
	var dsIds []int64
	if err := json.Unmarshal([]byte(rawIds), &dsIds); err != nil {
		return "", fmt.Errorf("datasource_ids must be a JSON array of integers (got %q): %v", rawIds, err)
	}
	if len(dsIds) == 0 {
		return "", fmt.Errorf("datasource_ids cannot be empty")
	}

	// Verify each datasource exists and the user can see it. Without this
	// check the LLM could spray rules at a datasource the caller doesn't
	// actually have rights on (mirrors the visibility check in
	// getDatasourceDetail).
	for _, dsId := range dsIds {
		ds, err := models.DatasourceGet(deps.DBCtx, dsId)
		if err != nil {
			return "", fmt.Errorf("failed to get datasource %d: %v", dsId, err)
		}
		if ds == nil {
			return "", fmt.Errorf("datasource not found: id=%d", dsId)
		}
		if deps.FilterDatasources != nil {
			filtered := deps.FilterDatasources([]*models.Datasource{ds}, user)
			if len(filtered) == 0 {
				return "", fmt.Errorf("forbidden: no access to datasource %d", dsId)
			}
		}
	}

	// 严校验 disabled：必须是 0 或 1。不能用 getArgInt——它把负数和非数字 silently
	// coerce 成默认 0，原本写的 `disabled < 0` 分支因此永远不可达。
	disabled, err := parseDisabledFlag(args["disabled"])
	if err != nil {
		return "", err
	}

	payload, err := resolvePromRulePayload(args)
	if err != nil {
		return "", err
	}

	groups, err := models.ParsePromRuleYAML(payload)
	if err != nil {
		return "", err
	}

	dsValues := make([]interface{}, 0, len(dsIds))
	for _, id := range dsIds {
		dsValues = append(dsValues, id)
	}
	dsQueries := []models.DatasourceQuery{{MatchType: 0, Op: "in", Values: dsValues}}

	rules := models.DealPromGroup(groups, dsQueries, disabled)
	if len(rules) == 0 {
		return "", fmt.Errorf("no alert rules parsed from payload")
	}

	prefix := getArgString(args, "name_prefix")
	suffix := getArgString(args, "name_suffix")

	items := make([]promRuleImportItem, 0, len(rules))
	for i := range rules {
		r := &rules[i]
		r.Id = 0
		r.GroupId = groupId
		r.CreateBy = user.Username
		r.UpdateBy = user.Username
		r.DatasourceIdsJson = dsIds
		if prefix != "" {
			r.Name = prefix + r.Name
		}
		if suffix != "" {
			r.Name = r.Name + suffix
		}

		item := promRuleImportItem{Name: r.Name}
		if err := r.FE2DB(); err != nil {
			item.Status = promRuleStatusFailed
			item.Error = err.Error()
			items = append(items, item)
			continue
		}
		if err := r.Add(deps.DBCtx); err != nil {
			// 重名是预期分支：DB 里已经有同名规则，不当真正失败，标 skipped。
			// 否则 LLM 看到 "failed: already exists" 容易触发"用 name_prefix
			// 重试整份 YAML"的纠正动作 —— 那会让没冲突的规则全部多写一遍。
			if err.Error() == promRuleDuplicateErrStr {
				item.Status = promRuleStatusSkipped
			} else {
				item.Status = promRuleStatusFailed
				item.Error = err.Error()
			}
			items = append(items, item)
			continue
		}
		item.Status = promRuleStatusCreated
		item.Id = r.Id
		items = append(items, item)
	}

	var created, skipped, failed int
	for _, it := range items {
		switch it.Status {
		case promRuleStatusCreated:
			created++
		case promRuleStatusSkipped:
			skipped++
		case promRuleStatusFailed:
			failed++
		}
	}
	payloadOut, _ := json.Marshal(map[string]interface{}{
		"group_id": groupId,
		"total":    len(items),
		"created":  created,
		"skipped":  skipped, // 重名规则跳过的数量，不是失败
		"failed":   failed,
		"items":    items,
	})
	logger.Debugf("import_prom_rule_yaml: group=%d total=%d created=%d skipped=%d failed=%d",
		groupId, len(items), created, skipped, failed)
	return string(payloadOut), nil
}

// parseCronEverySeconds turns "@every 60s" / "@every 5m" into seconds. Best
// effort: anything else returns 0 so the preview falls back to "unset" rather
// than misreporting.
func parseCronEverySeconds(pattern string) int {
	const prefix = "@every "
	if !strings.HasPrefix(pattern, prefix) {
		return 0
	}
	d, err := time.ParseDuration(strings.TrimPrefix(pattern, prefix))
	if err != nil {
		return 0
	}
	return int(d.Seconds())
}

// wrapIfComplex wraps a PromQL expression in parentheses if it contains
// operators or functions that could bind more loosely than the comparison
// we're about to append. For a bare metric selector like `cpu_usage_active`
// it returns the string unchanged; for `a / b` or `rate(x[5m])` it returns
// `(a / b)` / `(rate(x[5m]))`. This keeps the baked-in v1 threshold
// unambiguous, e.g. `(foo / bar) > 0.5` instead of `foo / bar > 0.5`.
func wrapIfComplex(promQL string) string {
	trimmed := strings.TrimSpace(promQL)
	// Already wrapped at the top level.
	if strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")") {
		return trimmed
	}
	// Cheap heuristic: any arithmetic operator, aggregation/function call,
	// or label selector with spaces suggests the expression is complex
	// enough that wrapping is safer than not.
	if strings.ContainsAny(trimmed, "+-*/%") ||
		strings.Contains(trimmed, " by ") ||
		strings.Contains(trimmed, " without ") ||
		strings.Contains(trimmed, "(") {
		return "(" + trimmed + ")"
	}
	return trimmed
}

// parseDisabledFlag 严格解析 import_prom_rule_yaml 的 disabled 入参：必须未传 /
// 0 / 1 三种之一。其他值（包括 -1、2、"abc"）报错。
//
// 历史实现用 getArgInt(args,"disabled",0)，但 getArgInt 把任何非正值（含负数）
// silently coerce 成默认 0，导致原本写的 disabled < 0 分支不可达，用户传 -1
// 不会得到任何反馈。这里直接读 raw 值做完整三态判断。
func parseDisabledFlag(raw interface{}) (int, error) {
	if raw == nil {
		return 0, nil
	}
	var n int64
	switch v := raw.(type) {
	case float64:
		// JSON 数字统一是 float64。要求整数 + 在 0/1 范围内。
		if v != float64(int64(v)) {
			return 0, fmt.Errorf("disabled must be 0 or 1 (got non-integer %v)", v)
		}
		n = int64(v)
	case int:
		n = int64(v)
	case int64:
		n = v
	case string:
		if v == "" {
			return 0, nil
		}
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("disabled must be 0 or 1 (got %q)", v)
		}
		n = parsed
	default:
		return 0, fmt.Errorf("disabled must be 0 or 1 (got %T)", raw)
	}
	if n != 0 && n != 1 {
		return 0, fmt.Errorf("disabled must be 0 or 1 (got %d)", n)
	}
	return int(n), nil
}
