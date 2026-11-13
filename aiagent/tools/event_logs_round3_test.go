package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestGetAlertEvalLogs_ReturnsPartialResultNotice(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{})
	resourceMembership(t, deps, "ro")
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.AlertRule{Id: 7, GroupId: 1}).Error)
	deps.GetAlertEvalLogs = func(id string) ([]string, string, string, error) {
		require.Equal(t, "7", id)
		return []string{"query failed"}, "engine-a", "limit", nil
	}

	out, err := getAlertEvalLogs(context.Background(), deps, map[string]interface{}{"rule_id": "7"}, map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		RuleID    int64    `json:"rule_id"`
		Count     int      `json:"count"`
		Logs      []string `json:"logs"`
		Instance  string   `json:"instance"`
		Truncated bool     `json:"truncated"`
		Reason    string   `json:"truncated_reason"`
		Note      string   `json:"note"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, int64(7), result.RuleID)
	require.Equal(t, 1, result.Count)
	require.Equal(t, []string{"query failed"}, result.Logs)
	require.Equal(t, "engine-a", result.Instance)
	require.True(t, result.Truncated)
	require.Equal(t, "limit", result.Reason)
	require.Contains(t, result.Note, "only the newest")
}

func TestGetAlertEvalLogs_DeniesUnownedRulesBeforeFetching(t *testing.T) {
	cases := []struct {
		name  string
		group int64
		want  string
	}{
		{name: "other group", group: 2, want: "no access to this resource"},
		{name: "no group", group: 0, want: "resource has no business group"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.AlertRule{})
			resourceMembership(t, deps, "ro")
			require.NoError(t, models.DB(deps.DBCtx).Create(&models.AlertRule{Id: 7, GroupId: tc.group}).Error)
			called := false
			deps.GetAlertEvalLogs = func(string) ([]string, string, string, error) { called = true; return nil, "", "", nil }

			out, err := getAlertEvalLogs(context.Background(), deps, map[string]interface{}{"rule_id": float64(7)},
				map[string]string{"user_id": "2"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
			require.False(t, called)
		})
	}
}

func TestGetAlertEvalLogs_PropagatesFetcherError(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.AlertRule{Id: 7, GroupId: 1}).Error)
	deps.GetAlertEvalLogs = func(string) ([]string, string, string, error) { return nil, "", "", errors.New("engine unavailable") }

	out, err := getAlertEvalLogs(context.Background(), deps, map[string]interface{}{"rule_id": "7"}, map[string]string{"user_id": "1"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "failed to get alert eval logs: engine unavailable")
}

func TestGetEventProcessingLogs_ResolvesActiveAndHistoricalEvents(t *testing.T) {
	cases := []struct {
		name    string
		history bool
		reason  string
	}{
		{name: "active complete"},
		{name: "history partial", history: true, reason: "timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.AlertCurEvent{}, &models.AlertHisEvent{})
			resourceMembership(t, deps, "ro")
			db := models.DB(deps.DBCtx)
			if tc.history {
				require.NoError(t, db.Create(&models.AlertHisEvent{Id: 7, Hash: "event-hash", GroupId: 1}).Error)
			} else {
				require.NoError(t, db.Create(&models.AlertCurEvent{Id: 7, Hash: "event-hash", GroupId: 1}).Error)
			}
			deps.GetEventProcessingLogs = func(hash string) ([]string, string, string, error) {
				require.Equal(t, "event-hash", hash)
				return []string{"muted by maintenance"}, "engine-b", tc.reason, nil
			}

			out, err := getEventProcessingLogs(context.Background(), deps, map[string]interface{}{"event_hash": " event-hash "},
				map[string]string{"user_id": "2"})

			require.NoError(t, err)
			var result map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(out), &result))
			require.Equal(t, "event-hash", result["event_hash"])
			require.Equal(t, "engine-b", result["instance"])
			require.Equal(t, float64(1), result["count"])
			require.Equal(t, []interface{}{"muted by maintenance"}, result["logs"])
			if tc.reason == "" {
				require.NotContains(t, result, "truncated")
			} else {
				require.Equal(t, true, result["truncated"])
				require.Equal(t, "timeout", result["truncated_reason"])
				require.Contains(t, result["note"], "time budget")
			}
		})
	}
}

func TestGetEventProcessingLogs_DeniesMissingAndInaccessibleEvents(t *testing.T) {
	cases := []struct {
		name string
		hash string
		want string
	}{
		{name: "missing argument", hash: " ", want: "event_hash is required"},
		{name: "missing row", hash: "absent", want: "event not found for hash"},
		{name: "inaccessible row", hash: "private", want: "no access to this resource"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.AlertCurEvent{}, &models.AlertHisEvent{})
			resourceMembership(t, deps, "ro")
			require.NoError(t, models.DB(deps.DBCtx).Create(&models.AlertCurEvent{Id: 7, Hash: "private", GroupId: 2}).Error)
			called := false
			deps.GetEventProcessingLogs = func(string) ([]string, string, string, error) { called = true; return nil, "", "", nil }

			out, err := getEventProcessingLogs(context.Background(), deps, map[string]interface{}{"event_hash": tc.hash},
				map[string]string{"user_id": "2"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
			require.False(t, called)
		})
	}
}

func TestGetEventPipelineExecutions_ReturnsOnlyRequestedEvent(t *testing.T) {
	cases := []struct {
		name    string
		history bool
	}{
		{name: "active"},
		{name: "history", history: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.AlertCurEvent{}, &models.AlertHisEvent{}, &models.EventPipelineExecution{})
			resourceMembership(t, deps, "ro")
			db := models.DB(deps.DBCtx)
			if tc.history {
				require.NoError(t, db.Create(&models.AlertHisEvent{Id: 7, GroupId: 1}).Error)
			} else {
				require.NoError(t, db.Create(&models.AlertCurEvent{Id: 7, GroupId: 1}).Error)
			}
			require.NoError(t, db.Create(&[]models.EventPipelineExecution{
				{
					ID: "exec-1", EventID: 7, PipelineID: 3, PipelineName: "Enrich", Mode: "event", Status: "failed",
					ErrorMessage: "lookup failed", ErrorNode: "lookup", DurationMs: 25, CreatedAt: 1000,
				},
				{ID: "private", EventID: 8, PipelineID: 4, Status: "success"},
			}).Error)

			out, err := getEventPipelineExecutions(context.Background(), deps, map[string]interface{}{"event_id": "7"},
				map[string]string{"user_id": "2"})

			require.NoError(t, err)
			var result struct {
				EventID    int64                    `json:"event_id"`
				Count      int                      `json:"count"`
				Executions []map[string]interface{} `json:"executions"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &result))
			require.Equal(t, int64(7), result.EventID)
			require.Equal(t, 1, result.Count)
			require.Equal(t, []map[string]interface{}{{
				"id": "exec-1", "pipeline_id": float64(3), "pipeline_name": "Enrich", "mode": "event", "status": "failed",
				"error_message": "lookup failed", "error_node": "lookup", "duration_ms": float64(25), "created_at": formatUnixTime(1000),
			}}, result.Executions)
		})
	}
}

func TestGetEventPipelineExecutions_DeniesOtherGroups(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertCurEvent{}, &models.AlertHisEvent{})
	resourceMembership(t, deps, "ro")
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.AlertCurEvent{Id: 7, GroupId: 2}).Error)

	out, err := getEventPipelineExecutions(context.Background(), deps, map[string]interface{}{"event_id": float64(7)},
		map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "no access to this resource")
}

func TestListAlertEngineInstances_RequiresAdmin(t *testing.T) {
	deps := newResourceTestDeps(t)

	out, err := listAlertEngineInstances(context.Background(), deps, nil, map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "only admin can list alert engine instances")
}

func TestEventTools_RejectMissingContext(t *testing.T) {
	cases := []struct {
		name    string
		handler aiagent.BuiltinToolFunc
		want    string
	}{
		{name: "evaluation logs", handler: getAlertEvalLogs, want: "fetcher not configured"},
		{name: "processing logs", handler: getEventProcessingLogs, want: "fetcher not configured"},
		{name: "pipeline executions", handler: getEventPipelineExecutions, want: "alert context not configured"},
		{name: "engine instances", handler: listAlertEngineInstances, want: "alert context not configured"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.handler(context.Background(), nil, nil, nil)

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
		})
	}
}
