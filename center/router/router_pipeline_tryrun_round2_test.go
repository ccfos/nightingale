package router

import (
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestTryRunEventPipeline_UsesMockEventWithoutPersistingIt(t *testing.T) {
	rt := identityRouter(t, &models.AlertHisEvent{}, &models.AlertCurEvent{}, &models.EventPipelineExecution{})
	body := map[string]interface{}{
		"use_mock_event": true, "mock_severity": 2, "pipeline_config": models.EventPipeline{Name: "empty workflow"},
	}

	w := pipelinePermissionRequest(t, rt.tryRunEventPipeline, body, "", &models.User{Username: "alice"})

	var got struct {
		Event  models.AlertCurEvent `json:"event"`
		Status string               `json:"status"`
		Result string               `json:"result"`
	}
	require.Empty(t, pipelinePermissionReply(t, w, &got))
	require.Equal(t, models.ExecutionStatusSuccess, got.Status)
	require.Equal(t, "no nodes to execute", got.Result)
	require.Equal(t, "event-pipeline-test-mock-event", got.Event.Hash)
	require.Equal(t, 2, got.Event.Severity)
	for _, table := range []interface{}{&models.AlertHisEvent{}, &models.AlertCurEvent{}} {
		var count int64
		require.NoError(t, rt.Ctx.DB.Model(table).Count(&count).Error)
		require.Zero(t, count)
	}
}

func TestTryRunEventPipeline_ConvertsSelectedHistoricalEvent(t *testing.T) {
	rt := identityRouter(t, &models.AlertHisEvent{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertHisEvent{
		Id: 7, RuleName: "CPU high", TriggerTime: 123, RuleConfig: "{}", Annotations: "{}", Tags: "env=prod",
	}).Error)
	body := map[string]interface{}{"event_id": 7, "pipeline_config": models.EventPipeline{Name: "empty workflow"}}

	w := pipelinePermissionRequest(t, rt.tryRunEventPipeline, body, "", &models.User{Username: "alice"})

	var got struct {
		Event  models.AlertCurEvent `json:"event"`
		Status string               `json:"status"`
	}
	require.Empty(t, pipelinePermissionReply(t, w, &got))
	require.Equal(t, models.ExecutionStatusSuccess, got.Status)
	require.Equal(t, int64(7), got.Event.Id)
	require.Equal(t, "CPU high", got.Event.RuleName)
	require.Equal(t, int64(123), got.Event.TriggerTime)
	require.Equal(t, "prod", got.Event.TagsMap["env"])
}

func TestTryRunEventProcessor_RejectsUnknownProcessorAndMissingEvent(t *testing.T) {
	rt := identityRouter(t, &models.AlertHisEvent{})
	unknown := map[string]interface{}{
		"use_mock_event": true, "processor_config": models.ProcessorConfig{Typ: "round2-unknown"},
	}

	w := pipelinePermissionRequest(t, rt.tryRunEventProcessor, unknown, "", nil)

	require.Contains(t, pipelinePermissionReply(t, w, nil), "processor type round2-unknown not found")

	missing := pipelinePermissionRequest(t, rt.tryRunEventPipeline, map[string]interface{}{"event_id": 404}, "", nil)

	require.Equal(t, http.StatusBadRequest, missing.Code)
	require.Contains(t, missing.Body.String(), "event not found")
}

func TestTryRunEventProcessorByNotifyRule_SkipsDisabledPipelineConfigurations(t *testing.T) {
	rt := identityRouter(t, &models.AlertHisEvent{}, &models.EventPipeline{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertHisEvent{
		Id: 7, RuleName: "original", RuleConfig: "{}", Annotations: "{}",
	}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.EventPipeline{
		ID: 9, Name: "disabled in notify rule", ProcessorConfigs: []models.ProcessorConfig{{Typ: "round2-unknown"}},
	}).Error)
	body := map[string]interface{}{
		"event_id": 7, "pipeline_configs": []models.PipelineConfig{{PipelineId: 9, Enable: false}},
	}

	w := pipelinePermissionRequest(t, rt.tryRunEventProcessorByNotifyRule, body, "", nil)

	var got models.AlertCurEvent
	require.Empty(t, pipelinePermissionReply(t, w, &got))
	require.Equal(t, int64(7), got.Id)
	require.Equal(t, "original", got.RuleName)
}

func TestTryRunEventProcessor_DropsOnlyEventsMatchingTemplate(t *testing.T) {
	cases := []struct {
		name      string
		condition string
		dropped   bool
	}{
		{name: "matches", condition: `{{ eq $event.Severity 2 }}`, dropped: true},
		{name: "does not match", condition: `{{ eq $event.Severity 1 }}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}
			body := map[string]interface{}{
				"use_mock_event": true, "mock_severity": 2,
				"processor_config": models.ProcessorConfig{
					Typ:    "event_drop",
					Config: map[string]interface{}{"content": tc.condition},
				},
			}

			w := pipelinePermissionRequest(t, rt.tryRunEventProcessor, body, "", nil)

			var got struct {
				Event  *models.AlertCurEvent `json:"event"`
				Result string                `json:"result"`
			}
			require.Empty(t, pipelinePermissionReply(t, w, &got))
			if tc.dropped {
				require.Nil(t, got.Event)
				require.Equal(t, "drop event success", got.Result)
			} else {
				require.NotNil(t, got.Event)
				require.Equal(t, 2, got.Event.Severity)
				require.Equal(t, "drop event failed", got.Result)
			}
		})
	}
}

func TestTryRunEventProcessorByNotifyRule_StopsAfterEventIsDropped(t *testing.T) {
	rt := identityRouter(t, &models.AlertHisEvent{}, &models.EventPipeline{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertHisEvent{
		Id: 7, RuleName: "original", RuleConfig: "{}", Annotations: "{}",
	}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.EventPipeline{
		ID: 9, Name: "drop event", ProcessorConfigs: []models.ProcessorConfig{
			{Typ: "event_drop", Config: map[string]interface{}{"content": "true"}},
			{Typ: "round2-unknown"},
		},
	}).Error)
	body := map[string]interface{}{
		"event_id": 7, "pipeline_configs": []models.PipelineConfig{{PipelineId: 9, Enable: true}},
	}

	w := pipelinePermissionRequest(t, rt.tryRunEventProcessorByNotifyRule, body, "", nil)

	var got struct {
		Event  *models.AlertCurEvent `json:"event"`
		Result string                `json:"result"`
	}
	require.Empty(t, pipelinePermissionReply(t, w, &got))
	require.Nil(t, got.Event)
	require.Equal(t, "event is dropped", got.Result)
	var original models.AlertHisEvent
	require.NoError(t, rt.Ctx.DB.First(&original, 7).Error)
	require.Equal(t, "original", original.RuleName)
}

func TestTryRunEventProcessor_InvalidTemplateReturnsProcessorError(t *testing.T) {
	rt := &Router{}
	body := map[string]interface{}{
		"use_mock_event":   true,
		"processor_config": models.ProcessorConfig{Typ: "event_drop", Config: map[string]interface{}{"content": "{{"}},
	}

	w := pipelinePermissionRequest(t, rt.tryRunEventProcessor, body, "", nil)

	require.Contains(t, pipelinePermissionReply(t, w, nil), "processor failed to parse template")
}
