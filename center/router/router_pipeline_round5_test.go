package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStreamEventPipeline_ReturnsLocalWorkflowResult(t *testing.T) {
	rt := identityRouter(t, &models.EventPipeline{}, &models.EventPipelineExecution{}, &models.UserGroupMember{})
	require.NoError(t, rt.Ctx.DB.Create(&models.EventPipeline{ID: 1, Name: "empty"}).Error)
	event := &models.AlertCurEvent{Id: 7, RuleName: "CPU", TriggerTime: 123, Severity: 2}
	user := &models.User{Username: "admin", RolesLst: []string{models.AdminRole}}

	w := identityRequest(t, rt, rt.streamEventPipeline, EventPipelineRequest{Event: event}, "1", user)

	var got models.WorkflowResult
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, models.ExecutionStatusSuccess, got.Status)
	require.Equal(t, "no nodes to execute", got.Message)
	require.Equal(t, event, got.Event)
}

func TestStreamEventPipeline_RejectsForeignGroupBeforeExecution(t *testing.T) {
	rt := identityRouter(t, &models.EventPipeline{}, &models.EventPipelineExecution{},
		&models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserGroupMember{}, &models.UserGroup{})
	seedPipelinePermissionMembership(t, rt)
	original := models.EventPipeline{ID: 1, Name: "restricted", GroupId: 2}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)

	w := identityRequest(t, rt, rt.streamEventPipeline, EventPipelineRequest{}, "1", &models.User{Id: 1})

	require.Equal(t, http.StatusForbidden, w.Code)
	require.NotContains(t, w.Body.String(), "no nodes to execute")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.EventPipelineExecution{}).Count(&count).Error)
	require.Zero(t, count)
	var saved models.EventPipeline
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, original.Name, saved.Name)
	require.Equal(t, original.GroupId, saved.GroupId)
}

func TestStreamEventPipelineByService_SuppliesEmptyEventWhenOmitted(t *testing.T) {
	rt := identityRouter(t, &models.EventPipeline{})
	require.NoError(t, rt.Ctx.DB.Create(&models.EventPipeline{ID: 1, Name: "empty"}).Error)

	w := identityRequest(t, rt, rt.streamEventPipelineByService, EventPipelineRequest{Username: "service"}, "1", nil)

	var got models.WorkflowResult
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, models.ExecutionStatusSuccess, got.Status)
	require.NotNil(t, got.Event)
	require.Positive(t, got.Event.TriggerTime)
	require.Empty(t, got.Event.RuleName)
}

func TestStreamEventPipelineByService_PersistsTriggerIdentityAndInputOverrides(t *testing.T) {
	rt := identityRouter(t, &models.EventPipeline{}, &models.EventPipelineExecution{})
	pipeline := models.EventPipeline{
		ID: 1, Name: "local drop check",
		ProcessorConfigs: []models.ProcessorConfig{{Typ: "event_drop", Config: map[string]interface{}{"content": "false"}}},
	}
	require.NoError(t, rt.Ctx.DB.Create(&pipeline).Error)
	form := EventPipelineRequest{
		Username: "service", Event: &models.AlertCurEvent{Id: 17, RuleName: "CPU", Severity: 2},
		InputsOverrides: map[string]string{"environment": "prod"},
	}

	w := identityRequest(t, rt, rt.streamEventPipelineByService, form, "1", nil)

	var got models.WorkflowResult
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, models.ExecutionStatusSuccess, got.Status)
	require.Equal(t, "CPU", got.Event.RuleName)
	var saved []models.EventPipelineExecution
	require.NoError(t, rt.Ctx.DB.Find(&saved).Error)
	require.Len(t, saved, 1)
	require.NotEmpty(t, saved[0].ID)
	require.Equal(t, int64(1), saved[0].PipelineID)
	require.Equal(t, int64(17), saved[0].EventID)
	require.Equal(t, "service", saved[0].TriggerBy)
	require.Equal(t, models.TriggerModeAPI, saved[0].Mode)
	require.Equal(t, models.ExecutionStatusSuccess, saved[0].Status)
	inputs, err := saved[0].GetInputsSnapshot()
	require.NoError(t, err)
	require.Equal(t, form.InputsOverrides, inputs)
}

func TestPipelineExecutionEndpoints_RejectMissingPipeline(t *testing.T) {
	rt := identityRouter(t, &models.EventPipeline{}, &models.EventPipelineExecution{})
	cases := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "API trigger", handler: rt.triggerEventPipelineByAPI},
		{name: "service trigger", handler: rt.triggerEventPipelineByService},
		{name: "API stream", handler: rt.streamEventPipeline},
		{name: "service stream", handler: rt.streamEventPipelineByService},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := identityRequest(t, rt, tc.handler, EventPipelineRequest{}, "404", &models.User{Roles: models.AdminRole})

			require.Equal(t, http.StatusNotFound, w.Code)
			require.Contains(t, w.Body.String(), "pipeline not found")
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.EventPipelineExecution{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestEventPipelinesListByService_ReturnsStoredProcessorConfiguration(t *testing.T) {
	rt := identityRouter(t, &models.EventPipeline{})
	pipeline := models.EventPipeline{
		ID: 1, Name: "drop critical", GroupId: 9,
		ProcessorConfigs: []models.ProcessorConfig{{Typ: "event_drop", Config: map[string]interface{}{"content": "true"}}},
	}
	require.NoError(t, rt.Ctx.DB.Create(&pipeline).Error)

	w := identityRequest(t, rt, rt.eventPipelinesListByService, nil, "0", nil)

	var got []models.EventPipeline
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, pipeline.ID, got[0].ID)
	require.Equal(t, pipeline.GroupId, got[0].GroupId)
	require.Equal(t, pipeline.ProcessorConfigs, got[0].ProcessorConfigs)
}

func TestHandleStreamResponse_EmitsOrderedChunksAndStopsAtDone(t *testing.T) {
	chunks := make(chan *models.StreamChunk, 3)
	chunks <- &models.StreamChunk{Type: "text", Content: "hello", Delta: "hello"}
	chunks <- &models.StreamChunk{Type: "done", Content: "hello", Done: true}
	chunks <- &models.StreamChunk{Type: "text", Content: "must not be emitted"}
	close(chunks)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/stream", nil)

	(&Router{}).handleStreamResponse(c, &models.WorkflowResult{StreamChan: chunks}, "request-1")

	require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	require.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	require.Equal(t, "no", w.Header().Get("X-Accel-Buffering"))
	require.Equal(t, "request-1", w.Header().Get("X-Request-ID"))
	require.True(t, w.Flushed)
	frames := strings.Split(strings.TrimSpace(w.Body.String()), "\n\n")
	require.Len(t, frames, 3)
	var connected map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[0], "data: ")), &connected))
	require.Equal(t, "connected", connected["type"])
	require.Equal(t, "request-1", connected["request_id"])
	var text, done models.StreamChunk
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[1], "data: ")), &text))
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[2], "data: ")), &done))
	require.Equal(t, "hello", text.Delta)
	require.False(t, text.Done)
	require.True(t, done.Done)
	require.NotContains(t, w.Body.String(), "must not be emitted")
}

func TestHandleStreamResponse_ClosedChannelEndsAfterConnectionFrame(t *testing.T) {
	chunks := make(chan *models.StreamChunk)
	close(chunks)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/stream", nil)

	(&Router{}).handleStreamResponse(c, &models.WorkflowResult{StreamChan: chunks}, "closed-request")

	require.Equal(t, 1, strings.Count(w.Body.String(), "data: "))
	require.Contains(t, w.Body.String(), `"request_id":"closed-request"`)
	require.True(t, w.Flushed)
}
