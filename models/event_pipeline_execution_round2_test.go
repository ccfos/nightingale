package models

import (
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
)

func seedExecutionsRound2(t *testing.T, c *ctx.Context) {
	t.Helper()
	require.NoError(t, c.DB.Create(&[]EventPipelineExecution{
		{
			ID: "one", PipelineID: 7, PipelineName: "CPU", EventID: 11, Mode: "event",
			Status: "success", CreatedAt: 100, DurationMs: 20,
		},
		{
			ID: "two", PipelineID: 7, PipelineName: "CPU", EventID: 11, Mode: "event",
			Status: "success", CreatedAt: 200, DurationMs: 40,
		},
		{
			ID: "three", PipelineID: 7, PipelineName: "CPU", EventID: 12, Mode: "api",
			Status: "failed", CreatedAt: 300, DurationMs: 90,
		},
		{ID: "four", PipelineID: 7, PipelineName: "CPU", EventID: 12, Mode: "event", Status: "running", CreatedAt: 400},
		{
			ID: "other", PipelineID: 8, PipelineName: "Disk", EventID: 13, Mode: "api",
			Status: "success", CreatedAt: 500, DurationMs: 1000,
		},
	}).Error)
}

func TestEventPipelineExecutionDetail_RoundTripsNodeResultsAndInputs(t *testing.T) {
	c := pipelineContext(t, &EventPipelineExecution{})
	execution := EventPipelineExecution{ID: "execution", PipelineID: 7, Status: ExecutionStatusRunning}
	results := []*NodeExecutionResult{{NodeID: "node-1", Status: "success", Message: "processed", DurationMs: 25}}
	inputs := map[string]string{"env": "prod", "redacted_token": "***"}
	require.NoError(t, execution.SetNodeResults(results))
	require.NoError(t, execution.SetInputsSnapshot(inputs))
	require.NoError(t, CreateEventPipelineExecution(c, &execution))
	execution.Status = ExecutionStatusSuccess
	execution.DurationMs = 25
	require.NoError(t, UpdateEventPipelineExecution(c, &execution))

	detail, err := GetEventPipelineExecutionDetail(c, execution.ID)

	require.NoError(t, err)
	require.Equal(t, execution, detail.EventPipelineExecution)
	require.Equal(t, results, detail.NodeResultsParsed)
	require.Equal(t, inputs, detail.InputsSnapshotParsed)
}

func TestEventPipelineExecutionDetail_HandlesMissingAndMalformedPayloads(t *testing.T) {
	c := pipelineContext(t, &EventPipelineExecution{})
	missing, err := GetEventPipelineExecutionDetail(c, "missing")
	require.NoError(t, err)
	require.Equal(t, &EventPipelineExecutionDetail{}, missing)
	cases := []struct {
		name      string
		execution EventPipelineExecution
		want      string
	}{
		{name: "empty", execution: EventPipelineExecution{ID: "empty"}},
		{name: "nodes", execution: EventPipelineExecution{ID: "nodes", NodeResults: "{"}, want: "parse node results error"},
		{
			name: "inputs", execution: EventPipelineExecution{ID: "inputs", InputsSnapshot: "{"},
			want: "parse inputs snapshot error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, CreateEventPipelineExecution(c, &tc.execution))

			detail, err := GetEventPipelineExecutionDetail(c, tc.execution.ID)

			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
				require.Nil(t, detail)
			} else {
				require.NoError(t, err)
				require.Nil(t, detail.NodeResultsParsed)
				require.Nil(t, detail.InputsSnapshotParsed)
			}
		})
	}
}

func TestListEventPipelineExecutions_FiltersBeforeCountingAndPagination(t *testing.T) {
	c := pipelineContext(t, &EventPipelineExecution{})
	seedExecutionsRound2(t, c)

	executions, total, err := ListEventPipelineExecutions(c, 7, "event", "success", 1, 1)

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, executions, 1)
	require.Equal(t, "one", executions[0].ID)
	all, total, err := ListEventPipelineExecutions(c, 7, "", "", 10, 0)
	require.NoError(t, err)
	require.Equal(t, int64(4), total)
	require.Len(t, all, 4)
	require.Equal(t, "four", all[0].ID)
}

func TestListAllEventPipelineExecutions_AppliesInclusiveTimeAndMetadataFilters(t *testing.T) {
	c := pipelineContext(t, &EventPipelineExecution{})
	seedExecutionsRound2(t, c)

	executions, total, err := ListAllEventPipelineExecutions(c, 7, "CP", "event", "success", 100, 200, 1, 0)

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, executions, 1)
	require.Equal(t, "two", executions[0].ID)
	all, total, err := ListAllEventPipelineExecutions(c, 0, "", "", "", 0, 0, 10, 0)
	require.NoError(t, err)
	require.Equal(t, int64(5), total)
	require.Len(t, all, 5)
	require.Equal(t, "other", all[0].ID)
}

func TestListEventPipelineExecutionsByEventID_OrdersOnlyMatchingEvent(t *testing.T) {
	c := pipelineContext(t, &EventPipelineExecution{})
	seedExecutionsRound2(t, c)

	executions, err := ListEventPipelineExecutionsByEventID(c, 11)

	require.NoError(t, err)
	require.Len(t, executions, 2)
	require.Equal(t, "two", executions[0].ID)
	require.Equal(t, "one", executions[1].ID)
}

func TestEventPipelineExecutionStatistics_AveragesOnlySuccessfulRuns(t *testing.T) {
	c := pipelineContext(t, &EventPipelineExecution{})
	seedExecutionsRound2(t, c)

	stats, err := GetEventPipelineExecutionStatistics(c, 7)

	require.NoError(t, err)
	require.Equal(t, &EventPipelineExecutionStatistics{
		Total: 4, Success: 2, Failed: 1, Running: 1, AvgDurMs: 30, LastRunAt: 400,
	}, stats)
	empty, err := GetEventPipelineExecutionStatistics(c, 99)
	require.NoError(t, err)
	require.Equal(t, &EventPipelineExecutionStatistics{}, empty)
}

func TestDeleteEventPipelineExecutionsInBatches_RespectsLimitAndCutoff(t *testing.T) {
	c := pipelineContext(t, &EventPipelineExecution{})
	seedExecutionsRound2(t, c)

	deleted, err := DeleteEventPipelineExecutionsInBatches(c, 300, 1)

	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	// Batch selection has no ordering guarantee; assert the remaining count and cutoff instead of a chosen ID.
	remaining, total, err := ListAllEventPipelineExecutions(c, 0, "", "", "", 0, 0, 10, 0)
	require.NoError(t, err)
	require.Equal(t, int64(4), total)
	remainingIDs := make([]string, 0, len(remaining))
	for _, execution := range remaining {
		remainingIDs = append(remainingIDs, execution.ID)
	}
	require.Contains(t, remainingIDs, "three")
	require.Contains(t, remainingIDs, "four")
	require.Contains(t, remainingIDs, "other")
	deleted, err = DeleteEventPipelineExecutionsInBatches(c, 300, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	deleted, err = DeleteEventPipelineExecutionsInBatches(c, 300, 10)
	require.NoError(t, err)
	require.Zero(t, deleted)
	remaining, total, err = ListAllEventPipelineExecutions(c, 0, "", "", "", 0, 0, 10, 0)
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	for _, execution := range remaining {
		require.GreaterOrEqual(t, execution.CreatedAt, int64(300))
	}
}

func TestDeleteEventPipelineExecutions_UsesStrictTimeCutoff(t *testing.T) {
	c := pipelineContext(t, &EventPipelineExecution{})
	seedExecutionsRound2(t, c)

	deleted, err := DeleteEventPipelineExecutions(c, 300)

	require.NoError(t, err)
	require.Equal(t, int64(2), deleted)
	execution, err := GetEventPipelineExecution(c, "three")
	require.NoError(t, err)
	require.NotNil(t, execution)
	require.Equal(t, int64(300), execution.CreatedAt)
}

func TestDeleteEventPipelineExecutionsByPipelineID_PreservesOtherPipelines(t *testing.T) {
	c := pipelineContext(t, &EventPipelineExecution{})
	seedExecutionsRound2(t, c)

	require.NoError(t, DeleteEventPipelineExecutionsByPipelineID(c, 7))

	executions, total, err := ListAllEventPipelineExecutions(c, 0, "", "", "", 0, 0, 10, 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, executions, 1)
	require.Equal(t, "other", executions[0].ID)
}
