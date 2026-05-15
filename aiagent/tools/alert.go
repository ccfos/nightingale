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

// =============================================================================
// Result types for LLM consumption (simplified from full models)
// =============================================================================

type alertCurEventResult struct {
	Id           int64             `json:"id"`
	Hash         string            `json:"hash,omitempty"`
	RuleId       int64             `json:"rule_id,omitempty"`
	RuleName     string            `json:"rule_name"`
	Severity     int               `json:"severity"`
	TargetIdent  string            `json:"target_ident,omitempty"`
	TriggerTime  string            `json:"trigger_time"`
	TriggerValue string            `json:"trigger_value,omitempty"`
	Tags         map[string]string `json:"tags,omitempty"`
	GroupName    string            `json:"group_name,omitempty"`
}

type alertHisEventResult struct {
	Id           int64             `json:"id"`
	Hash         string            `json:"hash,omitempty"`
	RuleId       int64             `json:"rule_id,omitempty"`
	RuleName     string            `json:"rule_name"`
	Severity     int               `json:"severity"`
	IsRecovered  int               `json:"is_recovered"`
	TargetIdent  string            `json:"target_ident,omitempty"`
	TriggerTime  string            `json:"trigger_time"`
	RecoverTime  string            `json:"recover_time,omitempty"`
	TriggerValue string            `json:"trigger_value,omitempty"`
	Tags         map[string]string `json:"tags,omitempty"`
	GroupName    string            `json:"group_name,omitempty"`
}

type alertEventDetailResult struct {
	Id           int64             `json:"id"`
	Hash         string            `json:"hash,omitempty"`
	RuleId       int64             `json:"rule_id,omitempty"`
	RuleName     string            `json:"rule_name"`
	RuleNote     string            `json:"rule_note,omitempty"`
	Severity     int               `json:"severity"`
	IsRecovered  int               `json:"is_recovered"`
	TargetIdent  string            `json:"target_ident,omitempty"`
	TargetNote   string            `json:"target_note,omitempty"`
	TriggerTime  string            `json:"trigger_time"`
	RecoverTime  string            `json:"recover_time,omitempty"`
	TriggerValue string            `json:"trigger_value,omitempty"`
	Tags         map[string]string `json:"tags,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	GroupName    string            `json:"group_name,omitempty"`
	PromQl       string            `json:"prom_ql,omitempty"`
	RunbookUrl   string            `json:"runbook_url,omitempty"`
	Callbacks    []string          `json:"callbacks,omitempty"`
}

// =============================================================================
// Tool registration
// =============================================================================

func init() {
	register(defs.SearchActiveAlerts, searchActiveAlerts)
	register(defs.SearchHistoryAlerts, searchHistoryAlerts)
	register(defs.GetAlertEventDetail, getAlertEventDetail)
	register(defs.GetAlertEvalLogs, getAlertEvalLogs)
	register(defs.GetEventProcessingLogs, getEventProcessingLogs)
	register(defs.ListAlertEngineInstances, listAlertEngineInstances)
	register(defs.GetEventPipelineExecutions, getEventPipelineExecutions)
}

// =============================================================================
// Tool implementations
// =============================================================================

func searchActiveAlerts(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	if deps == nil || deps.DBCtx == nil {
		return "", fmt.Errorf("alert context not configured")
	}
	dbCtx := deps.DBCtx

	query, _ := args["query"].(string)
	severity := -1
	if s, ok := args["severity"].(float64); ok {
		severity = int(s)
	}
	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 100 {
			limit = 100
		}
	}

	var stime, etime int64
	if tr, ok := args["time_range"].(string); ok && tr != "" {
		stime, etime = parseTimeRange(tr)
	}

	var severities []int64
	if severity >= 0 {
		severities = []int64{int64(severity)}
	}

	total, err := models.AlertCurEventTotal(dbCtx, nil, nil, stime, etime, severities, nil, nil, 0, query, nil)
	if err != nil {
		return "", fmt.Errorf("failed to count active alerts: %v", err)
	}

	events, err := models.AlertCurEventsGet(dbCtx, nil, nil, stime, etime, severities, nil, nil, 0, query, limit, 0, nil)
	if err != nil {
		return "", fmt.Errorf("failed to search active alerts: %v", err)
	}

	results := make([]alertCurEventResult, 0, len(events))
	for _, e := range events {
		results = append(results, alertCurEventResult{
			Id:           e.Id,
			Hash:         e.Hash,
			RuleId:       e.RuleId,
			RuleName:     e.RuleName,
			Severity:     e.Severity,
			TargetIdent:  e.TargetIdent,
			TriggerTime:  formatUnixTime(e.TriggerTime),
			TriggerValue: e.TriggerValue,
			Tags:         e.TagsMap,
			GroupName:    e.GroupName,
		})
	}

	logger.Debugf("search_active_alerts: query=%s, severity=%d, found %d/%d", query, severity, len(results), total)

	bytes, _ := json.Marshal(map[string]interface{}{
		"total": total, "count": len(results), "events": results,
	})
	return string(bytes), nil
}

func searchHistoryAlerts(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	if deps == nil || deps.DBCtx == nil {
		return "", fmt.Errorf("alert context not configured")
	}
	dbCtx := deps.DBCtx

	query, _ := args["query"].(string)
	severity := -1
	if s, ok := args["severity"].(float64); ok {
		severity = int(s)
	}
	recovered := -1
	if r, ok := args["is_recovered"].(float64); ok {
		recovered = int(r)
	}
	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 100 {
			limit = 100
		}
	}

	// Default to 24h for history alerts
	timeRange := "24h"
	if tr, ok := args["time_range"].(string); ok && tr != "" {
		timeRange = tr
	}
	stime, etime := parseTimeRange(timeRange)

	total, err := models.AlertHisEventTotal(dbCtx, nil, nil, stime, etime, severity, recovered, nil, nil, 0, query, nil)
	if err != nil {
		return "", fmt.Errorf("failed to count history alerts: %v", err)
	}

	events, err := models.AlertHisEventGets(dbCtx, nil, nil, stime, etime, severity, recovered, nil, nil, 0, query, limit, 0, nil)
	if err != nil {
		return "", fmt.Errorf("failed to search history alerts: %v", err)
	}

	results := make([]alertHisEventResult, 0, len(events))
	for _, e := range events {
		r := alertHisEventResult{
			Id:           e.Id,
			Hash:         e.Hash,
			RuleId:       e.RuleId,
			RuleName:     e.RuleName,
			Severity:     e.Severity,
			IsRecovered:  e.IsRecovered,
			TargetIdent:  e.TargetIdent,
			TriggerTime:  formatUnixTime(e.TriggerTime),
			TriggerValue: e.TriggerValue,
			Tags:         tagsJSONToMap(e.TagsJSON),
			GroupName:    e.GroupName,
		}
		if e.RecoverTime > 0 {
			r.RecoverTime = formatUnixTime(e.RecoverTime)
		}
		results = append(results, r)
	}

	logger.Debugf("search_history_alerts: query=%s, severity=%d, recovered=%d, found %d/%d", query, severity, recovered, len(results), total)

	bytes, _ := json.Marshal(map[string]interface{}{
		"total": total, "count": len(results), "events": results,
	})
	return string(bytes), nil
}

func getAlertEventDetail(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	if deps == nil || deps.DBCtx == nil {
		return "", fmt.Errorf("alert context not configured")
	}

	var eventId int64
	switch v := args["event_id"].(type) {
	case float64:
		eventId = int64(v)
	case string:
		eventId, _ = strconv.ParseInt(v, 10, 64)
	}
	if eventId == 0 {
		return "", fmt.Errorf("event_id is required")
	}

	eventType := "active"
	if t, ok := args["event_type"].(string); ok && t != "" {
		eventType = t
	}

	var detail *alertEventDetailResult
	var err error

	if eventType == "history" {
		detail, err = getHisEventDetail(deps, eventId)
	} else {
		detail, err = getCurEventDetail(deps, eventId)
	}

	if err != nil {
		return "", fmt.Errorf("failed to get alert event detail: %v", err)
	}
	if detail == nil {
		return fmt.Sprintf(`{"error": "alert event not found: id=%d, type=%s"}`, eventId, eventType), nil
	}

	logger.Debugf("get_alert_event_detail: id=%d, type=%s", eventId, eventType)

	bytes, _ := json.Marshal(detail)
	return string(bytes), nil
}

// authorizeBgid 校验非 admin 用户对某业务组 (rule 或 event 的 group_id) 是否可访问。
// admin 直通；groupId<=0 视作"无归属"，拒绝以防越权。
func authorizeBgid(deps *aiagent.ToolDeps, user *models.User, groupId int64) error {
	if user == nil {
		return fmt.Errorf("forbidden: no user context")
	}
	if user.IsAdmin() {
		return nil
	}
	if groupId <= 0 {
		return fmt.Errorf("forbidden: resource has no business group")
	}
	bgids, _, err := getUserBgids(deps, user)
	if err != nil {
		return err
	}
	if !int64SliceContains(bgids, groupId) {
		return fmt.Errorf("forbidden: no access to this resource")
	}
	return nil
}

// resolveEventGroupIdByHash 通过 event hash 解析对应事件的业务组归属。
// 先查 cur，没命中再查 his——hash 在两表都可能命中（活跃事件 + 历史事件）。
func resolveEventGroupIdByHash(deps *aiagent.ToolDeps, hash string) (int64, bool, error) {
	cur, err := models.AlertCurEventGet(deps.DBCtx, "hash=?", hash)
	if err != nil {
		return 0, false, err
	}
	if cur != nil {
		return cur.GroupId, true, nil
	}
	his, err := models.AlertHisEventGetByHash(deps.DBCtx, hash)
	if err != nil {
		return 0, false, err
	}
	if his != nil {
		return his.GroupId, true, nil
	}
	return 0, false, nil
}

// resolveEventGroupIdById 通过 event_id 解析对应事件的业务组归属。
func resolveEventGroupIdById(deps *aiagent.ToolDeps, eventId int64) (int64, bool, error) {
	cur, err := models.AlertCurEventGetById(deps.DBCtx, eventId)
	if err != nil {
		return 0, false, err
	}
	if cur != nil {
		return cur.GroupId, true, nil
	}
	his, err := models.AlertHisEventGetById(deps.DBCtx, eventId)
	if err != nil {
		return 0, false, err
	}
	if his != nil {
		return his.GroupId, true, nil
	}
	return 0, false, nil
}

func getAlertEvalLogs(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	if deps == nil || deps.GetAlertEvalLogs == nil {
		return "", fmt.Errorf("alert eval logs fetcher not configured")
	}
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}

	var ruleId int64
	switch v := args["rule_id"].(type) {
	case float64:
		ruleId = int64(v)
	case string:
		ruleId, _ = strconv.ParseInt(v, 10, 64)
	}
	if ruleId <= 0 {
		return "", fmt.Errorf("rule_id is required")
	}

	// 鉴权：按规则所在业务组校验。规则不存在直接拒绝，避免靠错误差异枚举 rule_id。
	rule, err := models.AlertRuleGetById(deps.DBCtx, ruleId)
	if err != nil {
		return "", fmt.Errorf("failed to get alert rule: %v", err)
	}
	if rule == nil {
		return "", fmt.Errorf("alert rule not found: %d", ruleId)
	}
	if err := authorizeBgid(deps, user, rule.GroupId); err != nil {
		return "", err
	}

	logs, instance, err := deps.GetAlertEvalLogs(strconv.FormatInt(ruleId, 10))
	if err != nil {
		return "", fmt.Errorf("failed to get alert eval logs: %v", err)
	}

	logger.Debugf("get_alert_eval_logs: rule_id=%d, instance=%s, lines=%d", ruleId, instance, len(logs))

	bytes, _ := json.Marshal(map[string]interface{}{
		"rule_id":  ruleId,
		"instance": instance,
		"count":    len(logs),
		"logs":     logs,
	})
	return string(bytes), nil
}

func getEventProcessingLogs(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	if deps == nil || deps.GetEventProcessingLogs == nil {
		return "", fmt.Errorf("event processing logs fetcher not configured")
	}
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}

	hash, _ := args["event_hash"].(string)
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return "", fmt.Errorf("event_hash is required")
	}

	groupId, found, err := resolveEventGroupIdByHash(deps, hash)
	if err != nil {
		return "", fmt.Errorf("failed to resolve event: %v", err)
	}
	if !found {
		return "", fmt.Errorf("event not found for hash")
	}
	if err := authorizeBgid(deps, user, groupId); err != nil {
		return "", err
	}

	logs, instance, err := deps.GetEventProcessingLogs(hash)
	if err != nil {
		return "", fmt.Errorf("failed to get event processing logs: %v", err)
	}

	logger.Debugf("get_event_processing_logs: hash=%s, instance=%s, lines=%d", hash, instance, len(logs))

	bytes, _ := json.Marshal(map[string]interface{}{
		"event_hash": hash,
		"instance":   instance,
		"count":      len(logs),
		"logs":       logs,
	})
	return string(bytes), nil
}

func listAlertEngineInstances(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	if deps == nil || deps.DBCtx == nil {
		return "", fmt.Errorf("alert context not configured")
	}
	// 引擎实例清单含 instance 地址 / cluster / 心跳——属于平台级运维信息，没有
	// 业务组归属。仅 admin 可见，避免普通用户用作部署拓扑侦察。
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if !user.IsAdmin() {
		return "", fmt.Errorf("forbidden: only admin can list alert engine instances")
	}

	var (
		whereParts []string
		whereArgs  []interface{}
	)
	if dsId, ok := args["datasource_id"].(float64); ok && dsId > 0 {
		whereParts = append(whereParts, "datasource_id = ?")
		whereArgs = append(whereArgs, int64(dsId))
	}
	if cluster, ok := args["engine_cluster"].(string); ok && cluster != "" {
		whereParts = append(whereParts, "engine_cluster = ?")
		whereArgs = append(whereArgs, cluster)
	}

	where := strings.Join(whereParts, " AND ")
	engines, err := models.AlertingEngineGets(deps.DBCtx, where, whereArgs...)
	if err != nil {
		return "", fmt.Errorf("failed to list alert engine instances: %v", err)
	}

	now := time.Now().Unix()
	type engineResult struct {
		Instance      string `json:"instance"`
		EngineCluster string `json:"engine_cluster"`
		DatasourceId  int64  `json:"datasource_id"`
		LastHeartbeat string `json:"last_heartbeat"`
		StaleSeconds  int64  `json:"stale_seconds"`
		Healthy       bool   `json:"healthy"`
	}
	results := make([]engineResult, 0, len(engines))
	for _, e := range engines {
		stale := now - e.Clock
		results = append(results, engineResult{
			Instance:      e.Instance,
			EngineCluster: e.EngineCluster,
			DatasourceId:  e.DatasourceId,
			LastHeartbeat: formatUnixTime(e.Clock),
			StaleSeconds:  stale,
			Healthy:       stale <= 30,
		})
	}

	logger.Debugf("list_alert_engine_instances: where=%q, found=%d", where, len(results))

	bytes, _ := json.Marshal(map[string]interface{}{
		"count":     len(results),
		"instances": results,
	})
	return string(bytes), nil
}

func getEventPipelineExecutions(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	if deps == nil || deps.DBCtx == nil {
		return "", fmt.Errorf("alert context not configured")
	}
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}

	var eventId int64
	switch v := args["event_id"].(type) {
	case float64:
		eventId = int64(v)
	case string:
		eventId, _ = strconv.ParseInt(v, 10, 64)
	}
	if eventId <= 0 {
		return "", fmt.Errorf("event_id is required")
	}

	groupId, found, err := resolveEventGroupIdById(deps, eventId)
	if err != nil {
		return "", fmt.Errorf("failed to resolve event: %v", err)
	}
	if !found {
		return "", fmt.Errorf("event not found: %d", eventId)
	}
	if err := authorizeBgid(deps, user, groupId); err != nil {
		return "", err
	}

	executions, err := models.ListEventPipelineExecutionsByEventID(deps.DBCtx, eventId)
	if err != nil {
		return "", fmt.Errorf("failed to list pipeline executions: %v", err)
	}

	type execResult struct {
		ID           string `json:"id"`
		PipelineID   int64  `json:"pipeline_id"`
		PipelineName string `json:"pipeline_name"`
		Mode         string `json:"mode"`
		Status       string `json:"status"`
		ErrorMessage string `json:"error_message,omitempty"`
		ErrorNode    string `json:"error_node,omitempty"`
		DurationMs   int64  `json:"duration_ms"`
		CreatedAt    string `json:"created_at"`
	}
	results := make([]execResult, 0, len(executions))
	for _, e := range executions {
		results = append(results, execResult{
			ID:           e.ID,
			PipelineID:   e.PipelineID,
			PipelineName: e.PipelineName,
			Mode:         e.Mode,
			Status:       e.Status,
			ErrorMessage: e.ErrorMessage,
			ErrorNode:    e.ErrorNode,
			DurationMs:   e.DurationMs,
			CreatedAt:    formatUnixTime(e.CreatedAt),
		})
	}

	logger.Debugf("get_event_pipeline_executions: event_id=%d, count=%d", eventId, len(results))

	bytes, _ := json.Marshal(map[string]interface{}{
		"event_id":   eventId,
		"count":      len(results),
		"executions": results,
	})
	return string(bytes), nil
}

// =============================================================================
// Internal query helpers
// =============================================================================

func getCurEventDetail(deps *aiagent.ToolDeps, eventId int64) (*alertEventDetailResult, error) {
	e, err := models.AlertCurEventGetById(deps.DBCtx, eventId)
	if err != nil || e == nil {
		return nil, err
	}
	return &alertEventDetailResult{
		Id:           e.Id,
		Hash:         e.Hash,
		RuleId:       e.RuleId,
		RuleName:     e.RuleName,
		RuleNote:     e.RuleNote,
		Severity:     e.Severity,
		TargetIdent:  e.TargetIdent,
		TargetNote:   e.TargetNote,
		TriggerTime:  formatUnixTime(e.TriggerTime),
		TriggerValue: e.TriggerValue,
		Tags:         e.TagsMap,
		Annotations:  e.AnnotationsJSON,
		GroupName:    e.GroupName,
		PromQl:       e.PromQl,
		RunbookUrl:   e.RunbookUrl,
		Callbacks:    e.CallbacksJSON,
	}, nil
}

func getHisEventDetail(deps *aiagent.ToolDeps, eventId int64) (*alertEventDetailResult, error) {
	e, err := models.AlertHisEventGetById(deps.DBCtx, eventId)
	if err != nil || e == nil {
		return nil, err
	}
	result := &alertEventDetailResult{
		Id:           e.Id,
		Hash:         e.Hash,
		RuleId:       e.RuleId,
		RuleName:     e.RuleName,
		RuleNote:     e.RuleNote,
		Severity:     e.Severity,
		IsRecovered:  e.IsRecovered,
		TargetIdent:  e.TargetIdent,
		TriggerTime:  formatUnixTime(e.TriggerTime),
		TriggerValue: e.TriggerValue,
		Tags:         tagsJSONToMap(e.TagsJSON),
		Annotations:  e.AnnotationsJSON,
		GroupName:    e.GroupName,
		PromQl:       e.PromQl,
		RunbookUrl:   e.RunbookUrl,
		Callbacks:    e.CallbacksJSON,
	}
	if e.RecoverTime > 0 {
		result.RecoverTime = formatUnixTime(e.RecoverTime)
	}
	return result, nil
}

// tagsJSONToMap converts TagsJSON ([]string of "key=value") to a map.
func tagsJSONToMap(tagsJSON []string) map[string]string {
	m := make(map[string]string, len(tagsJSON))
	for _, pair := range tagsJSON {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}
