package router

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestAssistantChatNew_AssignsOwnerAndStoresPageContext(t *testing.T) {
	rt := identityRouter(t, &models.AssistantChatRow{})
	body := json.RawMessage(`{"page":"dashboards","param":{"dashboard_id":7}}`)

	w := identityRequest(t, rt, rt.assistantChatNew, body, "1", &models.User{Id: 42})

	var got models.AssistantChat
	require.Empty(t, identityReply(t, w, &got))
	require.NotEmpty(t, got.ChatID)
	require.Equal(t, int64(42), got.UserID)
	require.Equal(t, "New Chat", got.Title)
	require.True(t, got.IsNew)
	require.Equal(t, models.PageTypeDashboards, got.PageFrom.Page)
	require.JSONEq(t, `{"dashboard_id":7}`, string(got.PageFrom.Param))
	stored, err := models.AssistantChatCheckOwner(rt.Ctx, got.ChatID, 42)
	require.NoError(t, err)
	require.Equal(t, got, *stored)
}

func TestAssistantChatHistory_OnlyReturnsCurrentUsersChatsInRecentOrder(t *testing.T) {
	rt := identityRouter(t, &models.AssistantChatRow{})
	for _, chat := range []models.AssistantChat{
		{ChatID: "old", UserID: 1, Title: "Old", LastUpdate: 100},
		{ChatID: "new", UserID: 1, Title: "New", LastUpdate: 200},
		{ChatID: "other", UserID: 2, Title: "Private", LastUpdate: 300},
	} {
		require.NoError(t, models.AssistantChatSet(rt.Ctx, chat))
	}

	w := identityRequest(t, rt, rt.assistantChatHistory, nil, "1", &models.User{Id: 1})

	var got []models.AssistantChat
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	require.Equal(t, "new", got[0].ChatID)
	require.Equal(t, "old", got[1].ChatID)
	empty := identityRequest(t, rt, rt.assistantChatHistory, nil, "1", &models.User{Id: 99})
	require.Empty(t, identityReply(t, empty, &got))
	require.NotNil(t, got)
	require.Empty(t, got)
}

func TestAssistantChatDel_ChecksOwnershipAndRemovesMessages(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		name := "outsider"
		userID := int64(2)
		if allowed {
			name = "owner"
			userID = 1
		}
		t.Run(name, func(t *testing.T) {
			rt := identityRouter(t, &models.AssistantChatRow{}, &models.AssistantMessageRow{})
			require.NoError(t,
				models.AssistantChatSet(rt.Ctx, models.AssistantChat{ChatID: "owned", UserID: 1, Title: "Original"}))
			require.NoError(t,
				models.AssistantMessageSet(rt.Ctx, models.AssistantMessage{ChatID: "owned", SeqID: 1, IsFinish: true}))
			handler := func(c *gin.Context) {
				c.Params = append(c.Params, gin.Param{Key: "chatId", Value: "owned"})
				rt.assistantChatDel(c)
			}

			w := identityRequest(t, rt, handler, nil, "1", &models.User{Id: userID})

			var chatCount, messageCount int64
			require.NoError(t, rt.Ctx.DB.Model(&models.AssistantChatRow{}).Count(&chatCount).Error)
			require.NoError(t, rt.Ctx.DB.Model(&models.AssistantMessageRow{}).Count(&messageCount).Error)
			if allowed {
				require.Empty(t, identityReply(t, w, nil))
				require.Zero(t, chatCount)
				require.Zero(t, messageCount)
			} else {
				require.NotEmpty(t, identityReply(t, w, nil))
				require.Equal(t, int64(1), chatCount)
				require.Equal(t, int64(1), messageCount)
			}
		})
	}
}

func TestAssistantMessageHistory_RequiresOwnerAndOrdersMessages(t *testing.T) {
	rt := identityRouter(t, &models.AssistantChatRow{}, &models.AssistantMessageRow{})
	require.NoError(t, models.AssistantChatSet(rt.Ctx, models.AssistantChat{ChatID: "owned", UserID: 1}))
	for _, seq := range []int64{2, 1} {
		require.NoError(t, models.AssistantMessageSet(rt.Ctx, models.AssistantMessage{
			ChatID: "owned", SeqID: seq, IsFinish: true,
			Query: models.AssistantMessageQuery{Content: "question"},
		}))
	}

	w := identityRequest(t, rt, rt.assistantMessageHistory,
		map[string]string{"chat_id": "owned"}, "1", &models.User{Id: 1})

	var got []models.AssistantMessage
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	require.Equal(t, int64(1), got[0].SeqID)
	require.Equal(t, int64(2), got[1].SeqID)
	denied := identityRequest(t, rt, rt.assistantMessageHistory,
		map[string]string{"chat_id": "owned"}, "1", &models.User{Id: 2})
	require.NotEmpty(t, identityReply(t, denied, nil))
	require.NotContains(t, denied.Body.String(), "question")
}

func TestAssistantMessageHistory_RejectsMissingChatAndRendersEmptyHistory(t *testing.T) {
	rt := identityRouter(t, &models.AssistantChatRow{}, &models.AssistantMessageRow{})
	require.NoError(t, models.AssistantChatSet(rt.Ctx, models.AssistantChat{ChatID: "empty", UserID: 1}))

	missing := identityRequest(t, rt, rt.assistantMessageHistory, map[string]string{}, "1", &models.User{Id: 1})

	require.Equal(t, http.StatusBadRequest, missing.Code)
	require.Contains(t, missing.Body.String(), "chat_id is required")
	empty := identityRequest(t, rt, rt.assistantMessageHistory,
		map[string]string{"chat_id": "empty"}, "1", &models.User{Id: 1})
	var got []models.AssistantMessage
	require.Empty(t, identityReply(t, empty, &got))
	require.NotNil(t, got)
	require.Empty(t, got)
}

func TestAssistantMessageDetail_PrefersFinishedRedisSnapshotOverDatabase(t *testing.T) {
	rt := identityRouter(t, &models.AssistantChatRow{}, &models.AssistantMessageRow{})
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	rt.Redis = client
	require.NoError(t, models.AssistantChatSet(rt.Ctx, models.AssistantChat{ChatID: "owned", UserID: 1}))
	require.NoError(t, models.AssistantMessageSet(rt.Ctx, models.AssistantMessage{
		ChatID: "owned", SeqID: 1, IsFinish: true, ErrMsg: "database copy",
	}))
	snapshot := models.AssistantMessage{ChatID: "owned", SeqID: 1, IsFinish: true, ErrMsg: "latest snapshot"}
	require.NoError(t, models.MsgStateSet(context.Background(), client, &snapshot))

	w := identityRequest(t, rt, rt.assistantMessageDetail, map[string]interface{}{"chat_id": "owned", "seq_id": 1},
		"1", &models.User{Id: 1})

	var got models.AssistantMessage
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "latest snapshot", got.ErrMsg)
	require.True(t, got.IsFinish)
	stored, err := models.AssistantMessageGet(rt.Ctx, "owned", 1)
	require.NoError(t, err)
	require.Equal(t, "database copy", stored.ErrMsg)
}

func TestAssistantMessageDetail_FallsBackToDatabaseAndRejectsMissingMessage(t *testing.T) {
	rt := identityRouter(t, &models.AssistantChatRow{}, &models.AssistantMessageRow{})
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	rt.Redis = client
	require.NoError(t, models.AssistantChatSet(rt.Ctx, models.AssistantChat{ChatID: "owned", UserID: 1}))
	require.NoError(t, models.AssistantMessageSet(rt.Ctx, models.AssistantMessage{
		ChatID: "owned", SeqID: 1, IsFinish: true, ErrMsg: "persisted",
	}))

	w := identityRequest(t, rt, rt.assistantMessageDetail, map[string]interface{}{"chat_id": "owned", "seq_id": 1},
		"1", &models.User{Id: 1})

	var got models.AssistantMessage
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "persisted", got.ErrMsg)

	missing := identityRequest(t, rt, rt.assistantMessageDetail, map[string]interface{}{"chat_id": "owned", "seq_id": 404},
		"1", &models.User{Id: 1})

	require.Equal(t, http.StatusNotFound, missing.Code)
	require.Contains(t, missing.Body.String(), "message not found")
}
