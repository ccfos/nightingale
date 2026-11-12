package router

import (
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetEventPipelineExecution_ReturnsParsedResultsAndInputs(t *testing.T) {
	rt := identityRouter(t, &models.EventPipelineExecution{})
	record := models.EventPipelineExecution{
		ID: "execution-1", PipelineID: 7, PipelineName: "repair", Status: models.ExecutionStatusSuccess,
		NodeResults:    `[{"node_id":"step-1","node_type":"filter","status":"success"}]`,
		InputsSnapshot: `{"host":"node-a"}`, DurationMs: 123,
	}
	require.NoError(t, rt.Ctx.DB.Create(&record).Error)
	handler := func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "exec_id", Value: "execution-1"})
		rt.getEventPipelineExecution(c)
	}

	w := identityRequest(t, rt, handler, nil, "7", nil)

	var got models.EventPipelineExecutionDetail
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, record.ID, got.ID)
	require.Equal(t, record.Status, got.Status)
	require.Equal(t, record.DurationMs, got.DurationMs)
	require.Equal(t, map[string]string{"host": "node-a"}, got.InputsSnapshotParsed)
	require.Len(t, got.NodeResultsParsed, 1)
	require.Equal(t, "step-1", got.NodeResultsParsed[0].NodeID)
}

func TestGetEventPipelineExecutionStats_CountsStatusesAndAveragesSuccesses(t *testing.T) {
	rt := identityRouter(t, &models.EventPipelineExecution{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.EventPipelineExecution{
		{ID: "one", PipelineID: 7, Status: models.ExecutionStatusSuccess, DurationMs: 100, CreatedAt: 100},
		{ID: "two", PipelineID: 7, Status: models.ExecutionStatusSuccess, DurationMs: 300, CreatedAt: 200},
		{ID: "three", PipelineID: 7, Status: models.ExecutionStatusFailed, DurationMs: 900, CreatedAt: 300},
		{ID: "four", PipelineID: 7, Status: models.ExecutionStatusRunning, CreatedAt: 400},
		{ID: "other", PipelineID: 8, Status: models.ExecutionStatusSuccess, DurationMs: 1000, CreatedAt: 500},
	}).Error)

	w := identityRequest(t, rt, rt.getEventPipelineExecutionStats, nil, "7", nil)

	var got models.EventPipelineExecutionStatistics
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, models.EventPipelineExecutionStatistics{
		Total: 4, Success: 2, Failed: 1, Running: 1, AvgDurMs: 200, LastRunAt: 400,
	}, got)
}

func TestCleanEventPipelineExecutions_DeletesRecordsOlderThanRetention(t *testing.T) {
	cases := []struct {
		name string
		days int
	}{
		{name: "explicit retention", days: 7},
		{name: "default retention", days: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.EventPipelineExecution{})
			require.NoError(t, rt.Ctx.DB.Create(&[]models.EventPipelineExecution{
				{ID: "old", CreatedAt: 1},
				{ID: "retained", CreatedAt: time.Now().Unix()},
			}).Error)

			w := identityRequest(t, rt, rt.cleanEventPipelineExecutions, map[string]int{"before_days": tc.days}, "7", nil)

			var got struct {
				Deleted int64 `json:"deleted"`
			}
			require.Empty(t, identityReply(t, w, &got))
			require.Equal(t, int64(1), got.Deleted)
			var saved []models.EventPipelineExecution
			require.NoError(t, rt.Ctx.DB.Find(&saved).Error)
			require.Len(t, saved, 1)
			require.Equal(t, "retained", saved[0].ID)
		})
	}
}

func TestEventPipelineExecutionAdd_PersistsServiceRecord(t *testing.T) {
	rt := identityRouter(t, &models.EventPipelineExecution{})
	record := models.EventPipelineExecution{
		ID:             "execution-1",
		PipelineID:     7,
		CreatedAt:      123,
		Mode:           "api",
		Status:         "success",
		TriggerBy:      "service-user",
		InputsSnapshot: `{"host":"node-a"}`,
	}

	w := identityRequest(t, rt, rt.eventPipelineExecutionAdd, record, "7", nil)

	require.Empty(t, identityReply(t, w, nil))
	var saved models.EventPipelineExecution
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, record, saved)
}
