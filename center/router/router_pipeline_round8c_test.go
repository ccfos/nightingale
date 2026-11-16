package router

import (
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestR8cPipelineStreamFrames(t *testing.T) {
	for _, mode := range []string{"done", "closed", "cancelled", "unserializable"} {
		t.Run(mode, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/stream", nil)
			ch := make(chan *models.StreamChunk, 3)
			switch mode {
			case "done":
				ch <- &models.StreamChunk{Type: "text", Delta: "hello"}
				ch <- &models.StreamChunk{Type: "done", Done: true}
			case "closed":
				close(ch)
			case "cancelled":
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			case "unserializable":
				ch <- &models.StreamChunk{Metadata: make(chan int)}
				ch <- &models.StreamChunk{Type: "done", Done: true}
			}
			(&Router{}).handleStreamResponse(c, &models.WorkflowResult{StreamChan: ch}, "request-8c")
			require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
			require.Equal(t, "no", w.Header().Get("X-Accel-Buffering"))
			require.Equal(t, "request-8c", w.Header().Get("X-Request-ID"))
			frames := strings.Split(strings.TrimSpace(w.Body.String()), "\n\n")
			var first map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[0], "data: ")), &first))
			require.Equal(t, "connected", first["type"])
			if mode == "done" {
				require.Len(t, frames, 3)
				require.Contains(t, frames[1], `"delta":"hello"`)
			}
			if mode == "unserializable" {
				require.Len(t, frames, 2)
				require.Contains(t, frames[1], `"done":true`)
			}
		})
	}
}
func TestR8cPipelineMissingResources(t *testing.T) {
	rt := r8cRouter(t, &models.EventPipeline{}, &models.EventPipelineExecution{})
	for _, h := range []gin.HandlerFunc{rt.triggerEventPipelineByAPI, rt.triggerEventPipelineByService, rt.streamEventPipeline, rt.streamEventPipelineByService} {
		w := r8cRequest(t, h, EventPipelineRequest{}, "/404")
		require.Equal(t, 404, w.Code)
		require.Contains(t, w.Body.String(), "pipeline not found")
	}
	for _, body := range []map[string]interface{}{{}, {"id": "exec"}} {
		w := r8cRequest(t, rt.eventPipelineExecutionAdd, body, "/1")
		require.Equal(t, 400, w.Code)
		require.Contains(t, w.Body.String(), "required")
	}
}

func TestR8cPipelineTriggersPersistExecutionIdentity(t *testing.T) {
	for _, service := range []bool{false, true} {
		t.Run(fmt.Sprint(service), func(t *testing.T) {
			rt := r8cRouter(t, &models.EventPipeline{}, &models.EventPipelineExecution{})
			pipeline := models.EventPipeline{ID: 1, Name: "disabled node", Nodes: []models.WorkflowNode{{ID: "skip", Type: "event_drop", Disabled: true}}}
			require.NoError(t, rt.Ctx.DB.Create(&pipeline).Error)
			completed := make(chan struct{}, 1)
			require.NoError(t, rt.Ctx.DB.Callback().Create().After("gorm:commit_or_rollback_transaction").Register("r8c_execution_saved", func(db *gorm.DB) {
				if db.Statement.Table == "event_pipeline_execution" {
					completed <- struct{}{}
				}
			}))
			handler := rt.triggerEventPipelineByAPI
			request := EventPipelineRequest{InputsOverrides: map[string]string{"region": "test"}}
			if service {
				handler = rt.triggerEventPipelineByService
				request.Username = "service-user"
				request.Event = &models.AlertCurEvent{Id: 42}
			}
			var reply struct {
				ID      string `json:"execution_id"`
				Message string `json:"message"`
			}
			r8cData(t, r8cRequest(t, handler, request, "/1"), &reply)
			require.NotEmpty(t, reply.ID)
			require.Equal(t, "workflow execution started", reply.Message)
			select {
			case <-completed:
			case <-time.After(5 * time.Second):
				t.Fatal("execution was not persisted")
			}
			var execution models.EventPipelineExecution
			require.NoError(t, rt.Ctx.DB.First(&execution, "id = ?", reply.ID).Error)
			require.Equal(t, int64(1), execution.PipelineID)
			require.Equal(t, models.TriggerModeAPI, execution.Mode)
			require.Equal(t, models.ExecutionStatusSuccess, execution.Status)
			if service {
				require.Equal(t, "service-user", execution.TriggerBy)
				require.Equal(t, int64(42), execution.EventID)
			} else {
				require.Equal(t, "editor", execution.TriggerBy)
			}
			require.Contains(t, execution.InputsSnapshot, "test")
		})
	}
}
func TestR8cPipelineStreamEndpointReturnsSynchronousResult(t *testing.T) {
	rt := r8cRouter(t, &models.EventPipeline{})
	require.NoError(t, rt.Ctx.DB.Create(&models.EventPipeline{ID: 1, Name: "empty"}).Error)
	for _, service := range []bool{false, true} {
		for _, hasEvent := range []bool{false, true} {
			t.Run(fmt.Sprintf("service=%t/event=%t", service, hasEvent), func(t *testing.T) {
				handler := rt.streamEventPipeline
				if service {
					handler = rt.streamEventPipelineByService
				}
				request := EventPipelineRequest{Username: "service-user"}
				if hasEvent {
					request.Event = &models.AlertCurEvent{RuleName: "preserved"}
				}
				var result models.WorkflowResult
				r8cData(t, r8cRequest(t, handler, request, "/1"), &result)
				require.Equal(t, models.ExecutionStatusSuccess, result.Status)
				require.Equal(t, "no nodes to execute", result.Message)
				require.NotNil(t, result.Event)
				if hasEvent {
					require.Equal(t, "preserved", result.Event.RuleName)
				} else {
					require.Positive(t, result.Event.TriggerTime)
				}
			})
		}
	}
}
