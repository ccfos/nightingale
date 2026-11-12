package router

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/stretchr/testify/require"
)

func assistantStateRouter(t *testing.T) *Router {
	t.Helper()
	rt := identityRouter(t, &models.AssistantChatRow{}, &models.AssistantMessageRow{})
	targetReadRedis(t, rt)
	rt.pubsubBus = storage.NewPubsubBus(rt.Redis)
	rt.streamBus = aiagent.NewStreamBus(rt.Redis)
	return rt
}

func TestEnsureAssistantChat_AdoptsUnusedIDAndPersistsOwner(t *testing.T) {
	rt := assistantStateRouter(t)
	page := models.AssistantPageInfo{Page: models.PageTypeDashboards}

	chat, err := rt.EnsureAssistantChat(7, "sdk-context-id", page)

	require.NoError(t, err)
	require.Equal(t, "sdk-context-id", chat.ChatID)
	require.Equal(t, int64(7), chat.UserID)
	require.True(t, chat.IsNew)
	stored, err := models.AssistantChatCheckOwner(rt.Ctx, chat.ChatID, 7)
	require.NoError(t, err)
	require.Equal(t, page, stored.PageFrom)
	require.Equal(t, "New Chat", stored.Title)
}

func TestEnsureAssistantChat_ReusesOwnedChatWithoutReplacingContext(t *testing.T) {
	rt := assistantStateRouter(t)
	original := models.AssistantChat{ChatID: "existing", UserID: 7, Title: "Original", LastUpdate: 123}
	require.NoError(t, models.AssistantChatSet(rt.Ctx, original))

	chat, err := rt.EnsureAssistantChat(7, "existing", models.AssistantPageInfo{Page: models.PageTypeDashboards})

	require.NoError(t, err)
	require.Equal(t, original, *chat)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.AssistantChatRow{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestEnsureAssistantChat_ForeignIDAllocatesSeparateChat(t *testing.T) {
	rt := assistantStateRouter(t)
	original := models.AssistantChat{ChatID: "foreign", UserID: 8, Title: "Private"}
	require.NoError(t, models.AssistantChatSet(rt.Ctx, original))

	chat, err := rt.EnsureAssistantChat(7, "foreign", models.AssistantPageInfo{})

	require.NoError(t, err)
	require.NotEqual(t, "foreign", chat.ChatID)
	require.Equal(t, int64(7), chat.UserID)
	stored, err := models.AssistantChatCheckOwner(rt.Ctx, "foreign", 8)
	require.NoError(t, err)
	require.Equal(t, original, *stored)
}

func TestMessageStateUpdate_PersistsChangesToRedis(t *testing.T) {
	rt := assistantStateRouter(t)
	state := NewMessageState(rt.Redis, &models.AssistantMessage{ChatID: "chat", SeqID: 1})

	state.Update(context.Background(), func(msg *models.AssistantMessage) { msg.CurStep = "checking metrics" })

	saved, err := models.MsgStateGet(context.Background(), rt.Redis, "chat", 1)
	require.NoError(t, err)
	require.Equal(t, "checking metrics", saved.CurStep)
	require.Equal(t, "checking metrics", state.Msg().CurStep)
}

func TestMessageStatePersist_DoesNotOverwriteCancelledSnapshot(t *testing.T) {
	rt := assistantStateRouter(t)
	cancelled := models.AssistantMessage{ChatID: "chat", SeqID: 1, IsFinish: true, ErrMsg: "cancelled by user"}
	require.NoError(t, models.MsgStateSet(context.Background(), rt.Redis, &cancelled))
	require.NoError(t, models.MsgCancelMark(context.Background(), rt.Redis, "chat", 1))
	state := NewMessageState(rt.Redis, &models.AssistantMessage{ChatID: "chat", SeqID: 1})

	state.Update(context.Background(), func(msg *models.AssistantMessage) { msg.CurStep = "late update" })
	state.Persist(context.Background())

	require.Equal(t, "late update", state.Msg().CurStep)
	saved, err := models.MsgStateGet(context.Background(), rt.Redis, "chat", 1)
	require.NoError(t, err)
	require.Equal(t, cancelled, *saved)
}

func TestMessageStatePersist_WritesExplicitLocalChanges(t *testing.T) {
	rt := assistantStateRouter(t)
	msg := &models.AssistantMessage{ChatID: "chat", SeqID: 1, IsFinish: true, ErrMsg: "finished"}
	state := NewMessageState(rt.Redis, msg)

	state.Persist(context.Background())

	saved, err := models.MsgStateGet(context.Background(), rt.Redis, "chat", 1)
	require.NoError(t, err)
	require.Equal(t, *msg, *saved)
}

func TestCancelAssistantMessageInternal_PersistsCancellationAndFinishesStream(t *testing.T) {
	rt := assistantStateRouter(t)
	msg := models.AssistantMessage{
		ChatID: "chat", SeqID: 1, CurStep: "querying", Response: []models.AssistantMessageResponse{{StreamID: "1-1"}},
	}
	require.NoError(t, models.MsgStateSet(context.Background(), rt.Redis, &msg))
	require.NoError(t, models.AssistantMessageSet(rt.Ctx, msg))

	err := rt.CancelAssistantMessageInternal(context.Background(), "chat", 1)

	require.NoError(t, err)
	saved, err := models.MsgStateGet(context.Background(), rt.Redis, "chat", 1)
	require.NoError(t, err)
	require.True(t, saved.IsFinish)
	require.Empty(t, saved.CurStep)
	require.Equal(t, int(models.MessageStatusCancel), saved.ErrCode)
	require.Equal(t, "cancelled by user", saved.ErrMsg)
	stored, err := models.AssistantMessageGet(rt.Ctx, "chat", 1)
	require.NoError(t, err)
	require.Equal(t, saved.ErrCode, stored.ErrCode)
	require.True(t, stored.IsFinish)
	marked, err := models.MsgCancelExists(context.Background(), rt.Redis, "chat", 1)
	require.NoError(t, err)
	require.True(t, marked)
	entries, err := rt.Redis.XRange(context.Background(), models.StreamKey("chat", "1-1"), "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	// The terminal marker lets readers stop without waiting for another chunk.
	require.Equal(t, "finish", entries[0].Values["p"])
}

func TestCancelAssistantMessageInternal_RejectsMissingOrFinishedMessages(t *testing.T) {
	rt := assistantStateRouter(t)
	require.NoError(t, models.MsgStateSet(context.Background(), rt.Redis,
		&models.AssistantMessage{ChatID: "chat", SeqID: 1, IsFinish: true}))

	missing := rt.CancelAssistantMessageInternal(context.Background(), "chat", 404)
	finished := rt.CancelAssistantMessageInternal(context.Background(), "chat", 1)

	require.ErrorIs(t, missing, ErrMessageNotInflight)
	require.ErrorIs(t, finished, ErrMessageNotInflight)
	marked, err := models.MsgCancelExists(context.Background(), rt.Redis, "chat", 1)
	require.NoError(t, err)
	require.False(t, marked)
}

func TestAssistantMessageDetail_OrphanBecomesPersistedFailure(t *testing.T) {
	rt := assistantStateRouter(t)
	require.NoError(t, models.AssistantChatSet(rt.Ctx, models.AssistantChat{ChatID: "chat", UserID: 1}))
	msg := models.AssistantMessage{ChatID: "chat", SeqID: 1, CurStep: "querying"}
	require.NoError(t, models.MsgStateSet(context.Background(), rt.Redis, &msg))

	w := identityRequest(t, rt, rt.assistantMessageDetail,
		map[string]interface{}{"chat_id": "chat", "seq_id": 1}, "0", &models.User{Id: 1})

	var got models.AssistantMessage
	require.Empty(t, identityReply(t, w, &got))
	require.True(t, got.IsFinish)
	require.Equal(t, 500, got.ErrCode)
	require.Equal(t, "owner instance lost, message aborted", got.ErrMsg)
	require.Empty(t, got.CurStep)
	saved, err := models.AssistantMessageGet(rt.Ctx, "chat", 1)
	require.NoError(t, err)
	require.True(t, saved.IsFinish)
	require.Equal(t, got.ErrMsg, saved.ErrMsg)
	snapshot, err := models.MsgStateGet(context.Background(), rt.Redis, "chat", 1)
	require.NoError(t, err)
	require.Nil(t, snapshot)
}

func TestAssistantMessageDetail_CancelMarkerPreservesAuthoritativeDatabaseState(t *testing.T) {
	rt := assistantStateRouter(t)
	require.NoError(t, models.AssistantChatSet(rt.Ctx, models.AssistantChat{ChatID: "chat", UserID: 1}))
	stored := models.AssistantMessage{ChatID: "chat", SeqID: 1, IsFinish: true, ErrMsg: "authoritative final state"}
	require.NoError(t, models.AssistantMessageSet(rt.Ctx, stored))
	require.NoError(t, models.MsgStateSet(context.Background(), rt.Redis,
		&models.AssistantMessage{ChatID: "chat", SeqID: 1, CurStep: "stale"}))
	require.NoError(t, models.MsgCancelMark(context.Background(), rt.Redis, "chat", 1))

	w := identityRequest(t, rt, rt.assistantMessageDetail,
		map[string]interface{}{"chat_id": "chat", "seq_id": 1}, "0", &models.User{Id: 1})

	var got models.AssistantMessage
	require.Empty(t, identityReply(t, w, &got))
	require.True(t, got.IsFinish)
	require.Equal(t, int(models.MessageStatusCancel), got.ErrCode)
	saved, err := models.AssistantMessageGet(rt.Ctx, "chat", 1)
	require.NoError(t, err)
	require.Equal(t, "authoritative final state", saved.ErrMsg)
	snapshot, err := models.MsgStateGet(context.Background(), rt.Redis, "chat", 1)
	require.NoError(t, err)
	require.False(t, snapshot.IsFinish)
}
