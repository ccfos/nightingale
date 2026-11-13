package router

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestFinishMessage_PersistsFailureAndFinishesStream(t *testing.T) {
	rt := assistantStateRouter(t)
	msg := &models.AssistantMessage{ChatID: "chat", SeqID: 1, CurStep: "querying"}
	state := NewMessageState(rt.Redis, msg)

	rt.finishMessage(state, "stream", 502, "query failed")

	stored, err := models.AssistantMessageGet(rt.Ctx, "chat", 1)
	require.NoError(t, err)
	require.True(t, stored.IsFinish)
	require.Empty(t, stored.CurStep)
	require.Equal(t, 502, stored.ErrCode)
	require.Equal(t, "query failed", stored.ErrMsg)
	snapshot, err := models.MsgStateGet(context.Background(), rt.Redis, "chat", 1)
	require.NoError(t, err)
	require.Equal(t, stored.ErrCode, snapshot.ErrCode)
	require.Equal(t, stored.ErrMsg, snapshot.ErrMsg)
	require.True(t, snapshot.IsFinish)
	entries, err := rt.Redis.XRange(context.Background(), models.StreamKey("chat", "stream"), "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "finish", entries[0].Values["p"])
}

func TestFinishMessage_PreservesAuthoritativeCancellation(t *testing.T) {
	rt := assistantStateRouter(t)
	cancelled := models.AssistantMessage{
		ChatID: "chat", SeqID: 1, IsFinish: true, ErrCode: int(models.MessageStatusCancel), ErrMsg: "cancelled by user",
	}
	require.NoError(t, models.AssistantMessageSet(rt.Ctx, cancelled))
	require.NoError(t, models.MsgStateSet(context.Background(), rt.Redis, &cancelled))
	require.NoError(t, models.MsgCancelMark(context.Background(), rt.Redis, "chat", 1))
	state := NewMessageState(rt.Redis, &models.AssistantMessage{ChatID: "chat", SeqID: 1, CurStep: "late work"})

	rt.finishMessage(state, "stream", 500, "late failure")

	stored, err := models.AssistantMessageGet(rt.Ctx, "chat", 1)
	require.NoError(t, err)
	require.Equal(t, cancelled, *stored)
	snapshot, err := models.MsgStateGet(context.Background(), rt.Redis, "chat", 1)
	require.NoError(t, err)
	require.Equal(t, cancelled, *snapshot)
}

func TestFinishHaltedMessage_PublishesResponsesBeforeStreamCompletion(t *testing.T) {
	rt := assistantStateRouter(t)
	msg := &models.AssistantMessage{
		ChatID: "chat", SeqID: 1, CurStep: "selecting",
		Query: models.AssistantMessageQuery{Content: "create an alert"},
	}
	state := NewMessageState(rt.Redis, msg)
	responses := []models.AssistantMessageResponse{
		{ContentType: models.ContentTypeFormSelect, Content: `{"field":"group_id"}`},
		{ContentType: models.ContentTypeMarkdown, Content: "Choose a business group"},
	}
	route := &models.ConversationRoute{ActionKey: "creation", AwaitingForm: true}

	rt.finishHaltedMessage(state, "stream", nil, responses, route, "")

	stored, err := models.AssistantMessageGet(rt.Ctx, "chat", 1)
	require.NoError(t, err)
	require.True(t, stored.IsFinish)
	require.Empty(t, stored.CurStep)
	require.Equal(t, route, stored.Extra.Route)
	require.Len(t, stored.Response, 2)
	require.Equal(t, "stream", stored.Response[0].StreamID)
	require.Empty(t, stored.Response[1].StreamID)
	for i, response := range stored.Response {
		require.True(t, response.IsFinish)
		require.True(t, response.IsFromAI)
		require.Equal(t, responses[i].Content, response.Content)
	}
	var transcript aiagent.TranscriptEnvelope
	require.NoError(t, json.Unmarshal(stored.Extra.HistoryMessages, &transcript))
	require.Equal(t, []aiagent.ChatMessage{{Role: "user", Content: "create an alert"}}, transcript.Messages)
	entries, err := rt.Redis.XRange(context.Background(), models.StreamKey("chat", "stream"), "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, entries, 3)
	for i := range responses {
		require.Equal(t, aiagent.PhaseResponse, entries[i].Values["p"])
		var frame aiagent.ResponseFrame
		require.NoError(t, json.Unmarshal([]byte(entries[i].Values["v"].(string)), &frame))
		require.Equal(t, responses[i].ContentType, frame.ContentType)
		require.Equal(t, responses[i].Content, frame.Content)
	}
	// Consumers must see the form before the terminal stream marker.
	require.Equal(t, "finish", entries[2].Values["p"])
}

func TestFinishHaltedMessage_AppendsDeterministicResultToTranscript(t *testing.T) {
	rt := assistantStateRouter(t)
	msg := &models.AssistantMessage{ChatID: "chat", SeqID: 2, Query: models.AssistantMessageQuery{Content: "confirm"}}
	state := NewMessageState(rt.Redis, msg)
	history := []aiagent.ChatMessage{{Role: "user", Content: "propose an alert"}}

	rt.finishHaltedMessage(state, "stream", history, nil, nil, "Alert created")

	stored, err := models.AssistantMessageGet(rt.Ctx, "chat", 2)
	require.NoError(t, err)
	require.True(t, stored.IsFinish)
	require.Empty(t, stored.Response)
	var transcript aiagent.TranscriptEnvelope
	require.NoError(t, json.Unmarshal(stored.Extra.HistoryMessages, &transcript))
	require.Equal(t, aiagent.TranscriptSchemaVersion, transcript.SchemaVersion)
	require.Equal(t, []aiagent.ChatMessage{
		{Role: "user", Content: "propose an alert"},
		{Role: "user", Content: "confirm"},
		{Role: "assistant", Content: "Alert created"},
	}, transcript.Messages)
}

func TestExtractImportedAlertRuleCards_PreservesEachRuleCard(t *testing.T) {
	got := extractImportedAlertRuleCards(`{"cards":[{"id":7,"name":"CPU"},{"id":8,"name":"Memory"}]}`)

	require.Len(t, got, 2)
	require.JSONEq(t, `{"id":7,"name":"CPU"}`, got[0])
	require.JSONEq(t, `{"id":8,"name":"Memory"}`, got[1])
}

func TestExtractImportedAlertRuleCards_RejectsMalformedSummary(t *testing.T) {
	got := extractImportedAlertRuleCards(`{"cards":`)

	require.Nil(t, got)
}

func TestHasFormSelect_DetectsFormAmongOtherResponses(t *testing.T) {
	cases := []struct {
		name      string
		responses []models.AssistantMessageResponse
		want      bool
	}{
		{name: "empty"},
		{name: "markdown", responses: []models.AssistantMessageResponse{{ContentType: models.ContentTypeMarkdown}}},
		{
			name: "form after markdown", want: true,
			responses: []models.AssistantMessageResponse{
				{ContentType: models.ContentTypeMarkdown}, {ContentType: models.ContentTypeFormSelect},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasFormSelect(tc.responses)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestResolveChatLLMConfig_PrefersEnabledDefaultOverAgentBinding(t *testing.T) {
	rt := identityRouter(t, &models.AILLMConfig{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AILLMConfig{
		{Id: 1, Name: "default", IsDefault: true, Enabled: true},
		{Id: 2, Name: "agent", Enabled: true},
	}).Error)

	got := rt.resolveChatLLMConfig(&models.AIAgent{LLMConfigId: 2})

	require.NotNil(t, got)
	require.Equal(t, int64(1), got.Id)
}

func TestResolveChatLLMConfig_UsesOnlyEnabledAgentFallback(t *testing.T) {
	cases := []struct {
		name    string
		enabled bool
		boundID int64
		wantID  int64
	}{
		{name: "enabled fallback", enabled: true, boundID: 2, wantID: 2},
		{name: "disabled fallback", boundID: 2},
		{name: "missing fallback", boundID: 404},
		{name: "unbound agent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.AILLMConfig{})
			require.NoError(t, rt.Ctx.DB.Create(&models.AILLMConfig{Id: 2, Name: "agent", Enabled: tc.enabled}).Error)

			got := rt.resolveChatLLMConfig(&models.AIAgent{LLMConfigId: tc.boundID})

			if tc.wantID == 0 {
				require.Nil(t, got)
			} else {
				require.NotNil(t, got)
				require.Equal(t, tc.wantID, got.Id)
			}
		})
	}
}
