package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAssistantChatSet_UpdatesExistingChatWithoutDuplicatingIt(t *testing.T) {
	c := pipelineContext(t, &AssistantChatRow{})
	chat := AssistantChat{ChatID: "chat-a", UserID: 7, Title: "old", LastUpdate: 100, IsNew: true}
	require.NoError(t, AssistantChatSet(c, chat))
	chat.Title = "renamed"
	chat.LastUpdate = 200
	chat.IsNew = false
	chat.IsRenamed = true

	require.NoError(t, AssistantChatSet(c, chat))
	got, err := AssistantChatGet(c, "chat-a")

	require.NoError(t, err)
	require.Equal(t, chat, *got)
	var count int64
	require.NoError(t, c.DB.Model(&AssistantChatRow{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
	missing, err := AssistantChatGet(c, "missing")
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestAssistantChatGetsByUserID_OrdersUsableChatsAndSkipsDraftsAndCorruptRows(t *testing.T) {
	c := pipelineContext(t, &AssistantChatRow{})
	for _, chat := range []AssistantChat{
		{ChatID: "older", UserID: 7, LastUpdate: 100},
		{ChatID: "newer", UserID: 7, LastUpdate: 200},
		{ChatID: "draft", UserID: 7, LastUpdate: 300, IsNew: true},
		{ChatID: "other-user", UserID: 8, LastUpdate: 400},
	} {
		require.NoError(t, AssistantChatSet(c, chat))
	}
	require.NoError(t, c.DB.Create(&[]AssistantChatRow{
		{ChatID: "bad", UserID: 7, UpdatedAt: 500, Data: "not base64"},
		{ChatID: "empty", UserID: 7, UpdatedAt: 600},
	}).Error)

	got, err := AssistantChatGetsByUserID(c, 7)

	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "newer", got[0].ChatID)
	require.Equal(t, "older", got[1].ChatID)
}

func TestAssistantChatCheckOwner_RejectsMissingAndForeignChats(t *testing.T) {
	c := pipelineContext(t, &AssistantChatRow{})
	chat := AssistantChat{ChatID: "chat-a", UserID: 7, Title: "owned"}
	require.NoError(t, AssistantChatSet(c, chat))

	got, err := AssistantChatCheckOwner(c, "chat-a", 7)

	require.NoError(t, err)
	require.Equal(t, chat, *got)
	got, err = AssistantChatCheckOwner(c, "chat-a", 8)
	require.EqualError(t, err, "forbidden")
	require.Nil(t, got)
	got, err = AssistantChatCheckOwner(c, "missing", 7)
	require.EqualError(t, err, "chat not found")
	require.Nil(t, got)
}

func TestAssistantChatDelete_DeletesOnlyTheChatAndItsMessages(t *testing.T) {
	c := pipelineContext(t, &AssistantChatRow{}, &AssistantMessageRow{})
	for _, id := range []string{"chat-a", "chat-b"} {
		require.NoError(t, AssistantChatSet(c, AssistantChat{ChatID: id, UserID: 7}))
		require.NoError(t, AssistantMessageSet(c, AssistantMessage{ChatID: id, SeqID: 1}))
	}

	require.NoError(t, AssistantChatDelete(c, "chat-a"))

	chat, err := AssistantChatGet(c, "chat-a")
	require.NoError(t, err)
	require.Nil(t, chat)
	msg, err := AssistantMessageGet(c, "chat-a", 1)
	require.NoError(t, err)
	require.Nil(t, msg)
	chat, err = AssistantChatGet(c, "chat-b")
	require.NoError(t, err)
	require.NotNil(t, chat)
	msg, err = AssistantMessageGet(c, "chat-b", 1)
	require.NoError(t, err)
	require.NotNil(t, msg)
}

func TestAssistantMessageSet_RoundTripsExtraAndUpdatesOnlyMatchingSequence(t *testing.T) {
	c := pipelineContext(t, &AssistantMessageRow{})
	first := AssistantMessage{ChatID: "chat-a", SeqID: 1, Query: AssistantMessageQuery{Content: "first"}}
	second := AssistantMessage{ChatID: "chat-a", SeqID: 2, Query: AssistantMessageQuery{Content: "second"}}
	require.NoError(t, AssistantMessageSet(c, first))
	require.NoError(t, AssistantMessageSet(c, second))
	second.IsFinish = true
	second.Extra = AssistantMessageExtra{HistoryMessages: []byte(`{"messages":[]}`)}

	require.NoError(t, AssistantMessageSet(c, second))
	got, err := AssistantMessageGet(c, "chat-a", 2)

	require.NoError(t, err)
	require.Equal(t, second, *got)
	got, err = AssistantMessageGet(c, "chat-a", 1)
	require.NoError(t, err)
	require.Equal(t, first, *got)
	var count int64
	require.NoError(t, c.DB.Model(&AssistantMessageRow{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
	missing, err := AssistantMessageGet(c, "chat-a", 99)
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestAssistantMessageGetsByChat_OrdersMessagesAndOmitsCancelledOrCorruptRows(t *testing.T) {
	c := pipelineContext(t, &AssistantMessageRow{})
	for _, msg := range []AssistantMessage{
		{ChatID: "chat-a", SeqID: 3},
		{ChatID: "chat-a", SeqID: 1},
		{ChatID: "chat-a", SeqID: 2},
		{ChatID: "chat-b", SeqID: 1},
	} {
		require.NoError(t, AssistantMessageSet(c, msg))
	}
	require.NoError(t, AssistantMessageSetStatus(c, "chat-a", 2, MessageStatusCancel))
	require.NoError(t, c.DB.Create(&[]AssistantMessageRow{
		{ChatID: "chat-a", SeqID: 4, Data: "invalid"},
		{ChatID: "chat-a", SeqID: 5},
	}).Error)

	got, err := AssistantMessageGetsByChat(c, "chat-a")

	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, int64(1), got[0].SeqID)
	require.Equal(t, int64(3), got[1].SeqID)
	var other AssistantMessageRow
	require.NoError(t, c.DB.Where("chat_id = ? AND seq_id = ?", "chat-b", 1).First(&other).Error)
	require.Equal(t, int(MessageStatusNone), other.Status)
}

func TestAssistantMessageMaxSeqID_IsScopedToChat(t *testing.T) {
	c := pipelineContext(t, &AssistantMessageRow{})
	require.NoError(t, c.DB.Create(&[]AssistantMessageRow{
		{ChatID: "chat-a", SeqID: 2},
		{ChatID: "chat-a", SeqID: 5},
		{ChatID: "chat-b", SeqID: 99},
	}).Error)

	got, err := AssistantMessageMaxSeqID(c, "chat-a")

	require.NoError(t, err)
	require.Equal(t, int64(5), got)
	got, err = AssistantMessageMaxSeqID(c, "missing")
	require.NoError(t, err)
	require.Zero(t, got)
}

func TestDecodeChat_RejectsInvalidPayloads(t *testing.T) {
	invalidJSON, err := gzipBase64Encode([]byte("{"))
	require.NoError(t, err)
	cases := []struct {
		name      string
		row       *AssistantChatRow
		wantError bool
	}{
		{name: "nil"},
		{name: "empty", row: &AssistantChatRow{}},
		{name: "invalid compression", row: &AssistantChatRow{Data: "bad"}, wantError: true},
		{name: "invalid json", row: &AssistantChatRow{Data: invalidJSON}, wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chat, err := decodeChat(tc.row)

			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Nil(t, chat)
		})
	}
}
