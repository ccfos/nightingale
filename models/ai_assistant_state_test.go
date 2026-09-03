package models

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	s, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(s.Close)
	cli := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

func TestMsgKeys_HashTagConsistency(t *testing.T) {
	// 同一 chatID 的所有 key 必须包含 {chatID} hash tag，且 tag 一致
	chatID := "abc-123"
	seq := int64(7)
	streamID := "abc-123:stream-uuid"

	keys := []string{
		MsgStateKey(chatID, seq),
		MsgCancelKey(chatID, seq),
		MsgCancelChannel(chatID, seq),
		StreamKey(chatID, streamID),
	}

	tag := "{" + chatID + "}"
	for _, k := range keys {
		assert.Truef(t, strings.Contains(k, tag),
			"key %q must contain hash tag %q for cluster slot affinity", k, tag)
	}
}

func TestMsgStateGet_MissingReturnsNil(t *testing.T) {
	rds := newTestRedis(t)
	got, err := MsgStateGet(context.Background(), rds, "no-such-chat", 1)
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestMsgStateSetGet_RoundTrip(t *testing.T) {
	rds := newTestRedis(t)
	ctx := context.Background()

	msg := &AssistantMessage{
		ChatID:        "chat-1",
		SeqID:         3,
		CurStep:       "thinking",
		IsFinish:      false,
		ExecutedTools: true,
		Response: []AssistantMessageResponse{
			{ContentType: ContentTypeMarkdown, Content: "hello", IsFromAI: true},
		},
		Query: AssistantMessageQuery{Content: "what is 1+1?"},
	}
	require.NoError(t, MsgStateSet(ctx, rds, msg))

	got, err := MsgStateGet(ctx, rds, "chat-1", 3)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, msg.ChatID, got.ChatID)
	assert.Equal(t, msg.SeqID, got.SeqID)
	assert.Equal(t, msg.CurStep, got.CurStep)
	assert.Equal(t, msg.ExecutedTools, got.ExecutedTools)
	assert.Equal(t, msg.Query.Content, got.Query.Content)
	assert.Len(t, got.Response, 1)
	assert.Equal(t, "hello", got.Response[0].Content)
}

func TestMsgStateDelete(t *testing.T) {
	rds := newTestRedis(t)
	ctx := context.Background()

	msg := &AssistantMessage{ChatID: "chat-x", SeqID: 1}
	require.NoError(t, MsgStateSet(ctx, rds, msg))
	got, _ := MsgStateGet(ctx, rds, "chat-x", 1)
	require.NotNil(t, got)

	require.NoError(t, MsgStateDelete(ctx, rds, "chat-x", 1))
	got, err := MsgStateGet(ctx, rds, "chat-x", 1)
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestMsgCancelMarkExists(t *testing.T) {
	rds := newTestRedis(t)
	ctx := context.Background()

	exists, err := MsgCancelExists(ctx, rds, "chat-c", 1)
	require.NoError(t, err)
	assert.False(t, exists)

	require.NoError(t, MsgCancelMark(ctx, rds, "chat-c", 1))

	exists, err = MsgCancelExists(ctx, rds, "chat-c", 1)
	require.NoError(t, err)
	assert.True(t, exists)
}
