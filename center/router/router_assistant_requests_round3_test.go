package router

import (
	"context"
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantMessageNew_RejectsMissingChatOrContent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request models.AssistantSendRequest
		message string
	}{
		{name: "chat", message: "chat_id is required"},
		{name: "content", request: models.AssistantSendRequest{ChatID: "chat"}, message: "query.content is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := assistantStateRouter(t)

			w := identityRequest(t, rt, rt.assistantMessageNew, tc.request, "0", &models.User{Id: 1})

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.AssistantMessageRow{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestAssistantMessageNew_BusyChatDoesNotAllocateMessage(t *testing.T) {
	rt := assistantStateRouter(t)
	require.NoError(t, models.AssistantChatSet(rt.Ctx, models.AssistantChat{ChatID: "chat", UserID: 1}))
	lock, err := models.AcquireChatLock(context.Background(), rt.Redis, "chat")
	require.NoError(t, err)
	require.NotNil(t, lock)
	t.Cleanup(func() { require.NoError(t, lock.Release(context.Background(), rt.Redis)) })
	request := models.AssistantSendRequest{ChatID: "chat", Query: models.AssistantMessageQuery{Content: "Check metrics"}}

	w := identityRequest(t, rt, rt.assistantMessageNew, request, "0", &models.User{Id: 1})

	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), "chat is busy")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.AssistantMessageRow{}).Count(&count).Error)
	require.Zero(t, count)
	held, err := models.ChatLockHeld(context.Background(), rt.Redis, "chat")
	require.NoError(t, err)
	require.True(t, held)
}

func TestAssistantMessageCancel_RequiresOwnerAndPersistsCancellation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		uid     int64
		allowed bool
	}{
		{name: "owner", uid: 1, allowed: true},
		{name: "outsider", uid: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := assistantStateRouter(t)
			require.NoError(t, models.AssistantChatSet(rt.Ctx, models.AssistantChat{ChatID: "chat", UserID: 1}))
			msg := models.AssistantMessage{ChatID: "chat", SeqID: 1, CurStep: "working"}
			require.NoError(t, models.AssistantMessageSet(rt.Ctx, msg))
			require.NoError(t, models.MsgStateSet(context.Background(), rt.Redis, &msg))

			w := identityRequest(t, rt, rt.assistantMessageCancel,
				map[string]interface{}{"chat_id": "chat", "seq_id": 1}, "0", &models.User{Id: tc.uid})

			stored, err := models.AssistantMessageGet(rt.Ctx, "chat", 1)
			require.NoError(t, err)
			if tc.allowed {
				require.Empty(t, identityReply(t, w, nil))
				require.True(t, stored.IsFinish)
				require.Equal(t, int(models.MessageStatusCancel), stored.ErrCode)
			} else {
				require.NotEmpty(t, identityReply(t, w, nil))
				require.False(t, stored.IsFinish)
				require.Equal(t, "working", stored.CurStep)
			}
		})
	}
}

func TestAssistantMessageCancel_FinishedMessageReturnsNotFound(t *testing.T) {
	rt := assistantStateRouter(t)
	require.NoError(t, models.AssistantChatSet(rt.Ctx, models.AssistantChat{ChatID: "chat", UserID: 1}))
	require.NoError(t, models.MsgStateSet(context.Background(), rt.Redis,
		&models.AssistantMessage{ChatID: "chat", SeqID: 1, IsFinish: true}))

	w := identityRequest(t, rt, rt.assistantMessageCancel,
		map[string]interface{}{"chat_id": "chat", "seq_id": 1}, "0", &models.User{Id: 1})

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), ErrMessageNotInflight.Error())
}
