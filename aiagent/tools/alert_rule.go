package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
