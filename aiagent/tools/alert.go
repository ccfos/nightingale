package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

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
