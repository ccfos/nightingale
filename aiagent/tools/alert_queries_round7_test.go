package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSearchActiveAlerts_FiltersAndLimitsResults(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertCurEvent{})
	events := []models.AlertCurEvent{
		{
			Id: 1, Hash: "cpu-old", RuleId: 7, RuleName: "CPU high", Severity: 2, GroupId: 1,
			TriggerTime: 1700000000, Annotations: `{}`, RuleConfig: `{}`,
		},
		{
			Id: 2, Hash: "cpu-new", RuleId: 7, RuleName: "CPU high", Severity: 2, GroupId: 1,
			GroupName: "Operations", TargetIdent: "web-1", TriggerTime: 1700000100, TriggerValue: "91",
			Tags: "env=prod,,host=web-1", Annotations: `{}`, RuleConfig: `{}`,
		},
		{Id: 3, RuleName: "CPU warning", Severity: 3, GroupId: 1},
		{Id: 4, RuleName: "Disk high", Severity: 2, GroupId: 1},
	}
	require.NoError(t, models.DB(deps.DBCtx).Create(&events).Error)

	out, err := searchActiveAlerts(context.Background(), deps, map[string]interface{}{
		"query": "CPU", "severity": float64(2), "limit": float64(1),
	}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var result struct {
		Total  int64                 `json:"total"`
		Count  int                   `json:"count"`
		Events []alertCurEventResult `json:"events"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, int64(2), result.Total)
	require.Equal(t, 1, result.Count)
	require.Equal(t, []alertCurEventResult{{
		Id: 2, Hash: "cpu-new", RuleId: 7, RuleName: "CPU high", Severity: 2,
		TargetIdent: "web-1", TriggerTime: time.Unix(1700000100, 0).Format("2006-01-02 15:04:05"),
		TriggerValue: "91", Tags: map[string]string{"env": "prod", "host": "web-1"}, GroupName: "Operations",
	}}, result.Events)
}

func TestSearchActiveAlerts_AppliesTimeRange(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertCurEvent{})
	recent := time.Now().Add(-time.Hour).Unix()
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.AlertCurEvent{
		{Id: 1, RuleName: "recent", TriggerTime: recent, Annotations: `{}`, RuleConfig: `{}`},
		{Id: 2, RuleName: "old", TriggerTime: 1, Annotations: `{}`, RuleConfig: `{}`},
	}).Error)

	out, err := searchActiveAlerts(context.Background(), deps,
		map[string]interface{}{"time_range": "24h"}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var result struct {
		Total  int64                 `json:"total"`
		Events []alertCurEventResult `json:"events"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, int64(1), result.Total)
	require.Len(t, result.Events, 1)
	require.Equal(t, int64(1), result.Events[0].Id)
}

func TestSearchHistoryAlerts_FiltersRecoveredEventsAndRendersTags(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertHisEvent{})
	recent := time.Now().Add(-time.Hour).Unix()
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.AlertHisEvent{
		{
			Id: 1, Hash: "cpu", RuleId: 7, RuleName: "CPU high", Severity: 2, IsRecovered: 1, GroupId: 1,
			GroupName: "Operations", TargetIdent: "web-1", TriggerTime: 1700000000, TriggerValue: "91",
			RecoverTime: 1700000100, LastEvalTime: recent, Tags: "env=prod,,filter=a=b,,invalid",
		},
		{Id: 2, RuleName: "CPU active", Severity: 2, IsRecovered: 0, LastEvalTime: recent},
		{Id: 3, RuleName: "CPU warning", Severity: 3, IsRecovered: 1, LastEvalTime: recent},
		{Id: 4, RuleName: "Disk high", Severity: 2, IsRecovered: 1, LastEvalTime: recent},
		{Id: 5, RuleName: "CPU old", Severity: 2, IsRecovered: 1, LastEvalTime: 1},
	}).Error)

	out, err := searchHistoryAlerts(context.Background(), deps, map[string]interface{}{
		"query": "CPU", "severity": float64(2), "is_recovered": float64(1), "limit": float64(1),
	}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var result struct {
		Total  int64                 `json:"total"`
		Count  int                   `json:"count"`
		Events []alertHisEventResult `json:"events"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, int64(1), result.Total)
	require.Equal(t, 1, result.Count)
	require.Equal(t, []alertHisEventResult{{
		Id: 1, Hash: "cpu", RuleId: 7, RuleName: "CPU high", Severity: 2, IsRecovered: 1,
		TargetIdent: "web-1", TriggerTime: time.Unix(1700000000, 0).Format("2006-01-02 15:04:05"),
		RecoverTime: time.Unix(1700000100, 0).Format("2006-01-02 15:04:05"), TriggerValue: "91",
		Tags: map[string]string{"env": "prod", "filter": "a=b"}, GroupName: "Operations",
	}}, result.Events)
}

func TestSearchAlerts_CapsRequestedLimit(t *testing.T) {
	cases := []struct {
		name    string
		handler aiagent.BuiltinToolFunc
		model   interface{}
	}{
		{name: "active", handler: searchActiveAlerts, model: &models.AlertCurEvent{}},
		{name: "history", handler: searchHistoryAlerts, model: &models.AlertHisEvent{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, tc.model)
			recent := time.Now().Add(-time.Hour).Unix()
			for i := 1; i <= 101; i++ {
				var event interface{} = &models.AlertCurEvent{Id: int64(i), TriggerTime: recent + int64(i)}
				if tc.name == "history" {
					event = &models.AlertHisEvent{Id: int64(i), LastEvalTime: recent + int64(i)}
				}
				require.NoError(t, models.DB(deps.DBCtx).Create(event).Error)
			}

			out, err := tc.handler(context.Background(), deps,
				map[string]interface{}{"limit": float64(1000)}, map[string]string{"user_id": "1"})

			require.NoError(t, err)
			var result struct {
				Total  int64             `json:"total"`
				Count  int               `json:"count"`
				Events []json.RawMessage `json:"events"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &result))
			require.Equal(t, int64(101), result.Total)
			require.Equal(t, 100, result.Count)
			require.Len(t, result.Events, 100)
		})
	}
}

func TestSearchAlerts_RejectsInvalidTimeRange(t *testing.T) {
	deps := newResourceTestDeps(t)
	for _, handler := range []aiagent.BuiltinToolFunc{searchActiveAlerts, searchHistoryAlerts} {
		out, err := handler(context.Background(), deps,
			map[string]interface{}{"time_range": "1fortnight"}, map[string]string{"user_id": "1"})

		require.Empty(t, out)
		require.ErrorContains(t, err, `invalid time_range "1fortnight"`)
	}
}

func TestSearchAlerts_ReportsCountFailure(t *testing.T) {
	deps := newResourceTestDeps(t)
	cases := []struct {
		name    string
		handler aiagent.BuiltinToolFunc
	}{
		{name: "active", handler: searchActiveAlerts},
		{name: "history", handler: searchHistoryAlerts},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.handler(context.Background(), deps, nil, map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, "failed to count "+tc.name+" alerts")
		})
	}
}

func TestSearchAlerts_ReportsListFailureAfterSuccessfulCount(t *testing.T) {
	cases := []struct {
		name    string
		handler aiagent.BuiltinToolFunc
		model   interface{}
	}{
		{name: "active", handler: searchActiveAlerts, model: &models.AlertCurEvent{}},
		{name: "history", handler: searchHistoryAlerts, model: &models.AlertHisEvent{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, tc.model)
			db := models.DB(deps.DBCtx)
			queries := 0
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("round7:list_failure", func(tx *gorm.DB) {
				queries++
				if queries == 2 {
					tx.AddError(fmt.Errorf("list unavailable"))
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove("round7:list_failure")) })

			out, err := tc.handler(context.Background(), deps, nil, map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.EqualError(t, err, "failed to search "+tc.name+" alerts: list unavailable")
			require.Equal(t, 2, queries)
		})
	}
}

func TestGetAlertEventDetail_RendersActiveEvent(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertCurEvent{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.AlertCurEvent{
		Id: 7, Hash: "cpu", RuleId: 9, RuleName: "CPU high", RuleNote: "inspect", Severity: 2,
		GroupName: "Operations", TargetIdent: "web-1", TargetNote: "API host", TriggerTime: 1700000000,
		TriggerValue: "91", Tags: "host=web-1", Annotations: `{"summary":"busy"}`, RuleConfig: `{}`,
		PromQl: "cpu > 80", RunbookUrl: "/runbooks/cpu", Callbacks: "/callbacks/one /callbacks/two",
	}).Error)

	out, err := getAlertEventDetail(context.Background(), deps,
		map[string]interface{}{"event_id": "7"}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var result alertEventDetailResult
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, alertEventDetailResult{
		Id: 7, Hash: "cpu", RuleId: 9, RuleName: "CPU high", RuleNote: "inspect", Severity: 2,
		GroupName: "Operations", TargetIdent: "web-1", TargetNote: "API host",
		TriggerTime: time.Unix(1700000000, 0).Format("2006-01-02 15:04:05"), TriggerValue: "91",
		Tags: map[string]string{"host": "web-1"}, Annotations: map[string]string{"summary": "busy"},
		PromQl: "cpu > 80", RunbookUrl: "/runbooks/cpu", Callbacks: []string{"/callbacks/one", "/callbacks/two"},
	}, result)
}

func TestGetAlertEventDetail_SelectsHistoryEvent(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertCurEvent{}, &models.AlertHisEvent{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.AlertCurEvent{Id: 7, RuleName: "active"}).Error)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.AlertHisEvent{
		Id: 7, RuleName: "recovered", IsRecovered: 1, RecoverTime: 1700000100,
		Tags: "env=prod", Annotations: `{"summary":"resolved"}`, Callbacks: "/callback",
	}).Error)

	out, err := getAlertEventDetail(context.Background(), deps, map[string]interface{}{
		"event_id": float64(7), "event_type": "history",
	}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var result alertEventDetailResult
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, "recovered", result.RuleName)
	require.Equal(t, 1, result.IsRecovered)
	require.Equal(t, time.Unix(1700000100, 0).Format("2006-01-02 15:04:05"), result.RecoverTime)
	require.Equal(t, map[string]string{"env": "prod"}, result.Tags)
	require.Equal(t, map[string]string{"summary": "resolved"}, result.Annotations)
	require.Equal(t, []string{"/callback"}, result.Callbacks)
}

func TestGetAlertEventDetail_ReturnsNotFoundPayload(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertCurEvent{}, &models.AlertHisEvent{})
	for _, eventType := range []string{"active", "history"} {
		t.Run(eventType, func(t *testing.T) {
			out, err := getAlertEventDetail(context.Background(), deps, map[string]interface{}{
				"event_id": "7", "event_type": eventType,
			}, map[string]string{"user_id": "1"})

			require.NoError(t, err)
			require.JSONEq(t, fmt.Sprintf(`{"error":"alert event not found: id=7, type=%s"}`, eventType), out)
		})
	}
}

func TestGetAlertEventDetail_RejectsMissingID(t *testing.T) {
	deps := newResourceTestDeps(t)
	out, err := getAlertEventDetail(context.Background(), deps,
		map[string]interface{}{"event_id": "invalid"}, map[string]string{"user_id": "1"})

	require.Empty(t, out)
	require.EqualError(t, err, "event_id is required")
}

func TestGetAlertEventDetail_ReportsReadFailure(t *testing.T) {
	deps := newResourceTestDeps(t)
	out, err := getAlertEventDetail(context.Background(), deps,
		map[string]interface{}{"event_id": "7"}, map[string]string{"user_id": "1"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "failed to get alert event detail:")
}

func TestAlertQueries_RequireDatabaseContext(t *testing.T) {
	for _, handler := range []aiagent.BuiltinToolFunc{searchActiveAlerts, searchHistoryAlerts, getAlertEventDetail} {
		out, err := handler(context.Background(), nil, nil, nil)

		require.Empty(t, out)
		require.EqualError(t, err, "alert context not configured")
	}
}
