package models

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestFetchBeatTimesFromRedis_BatchesKeysAndSkipsMissingOrMalformedValues(t *testing.T) {
	rds := newTestRedis(t)
	c := context.Background()
	idents := make([]string, 102)
	expected := make(map[string]int64)
	for i := range idents {
		idents[i] = fmt.Sprintf("node-%03d", i)
		if i < 100 {
			expected[idents[i]] = int64(i + 10)
			require.NoError(t, rds.Set(c, WrapIdentUpdateTime(idents[i]), HostUpdateTime{
				Ident: idents[i], UpdateTime: int64(i + 10),
			}, 0).Err())
		}
	}
	require.NoError(t, rds.Set(c, WrapIdentUpdateTime(idents[101]), "invalid JSON", 0).Err())

	got := FetchBeatTimesFromRedis(rds, idents)

	require.Equal(t, expected, got)
	require.Empty(t, FetchBeatTimesFromRedis(nil, idents))
	require.Empty(t, FetchBeatTimesFromRedis(rds, nil))
}

func TestFillTargetsBeatTime_PreservesExistingValueWhenRedisHasNoHeartbeat(t *testing.T) {
	rds := newTestRedis(t)
	require.NoError(t, rds.Set(context.Background(), WrapIdentUpdateTime("present"), HostUpdateTime{
		Ident: "present", UpdateTime: 42,
	}, 0).Err())
	targets := []*Target{{Ident: "present", BeatTime: 1}, {Ident: "missing", BeatTime: 9}}

	FillTargetsBeatTime(rds, targets)

	require.Equal(t, int64(42), targets[0].BeatTime)
	require.Equal(t, int64(9), targets[1].BeatTime)
}

func TestAcquireChatLock_ExcludesCompetingHoldersButAllowsOtherChats(t *testing.T) {
	rds := newTestRedis(t)
	c := context.Background()

	lock, err := AcquireChatLock(c, rds, "chat")

	require.NoError(t, err)
	require.NotNil(t, lock)
	require.Equal(t, AssistantChatLockKey("chat"), lock.Key)
	require.NotEmpty(t, lock.Token)
	require.Equal(t, lock.Token, rds.Get(c, lock.Key).Val())
	held, err := ChatLockHeld(c, rds, "chat")
	require.NoError(t, err)
	require.True(t, held)
	competitor, err := AcquireChatLock(c, rds, "chat")
	require.NoError(t, err)
	require.Nil(t, competitor)
	other, err := AcquireChatLock(c, rds, "other")
	require.NoError(t, err)
	require.NotNil(t, other)
	require.NoError(t, lock.Release(c, rds))
	held, err = ChatLockHeld(c, rds, "chat")
	require.NoError(t, err)
	require.False(t, held)
	held, err = ChatLockHeld(c, rds, "other")
	require.NoError(t, err)
	require.True(t, held)
}

func TestChatLockRelease_DoesNotDeleteSuccessorLock(t *testing.T) {
	server := miniredis.RunT(t)
	rds := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, rds.Close()) })
	c := context.Background()
	original, err := AcquireChatLock(c, rds, "chat")
	require.NoError(t, err)
	server.FastForward(ChatLockTTL + time.Second)
	successor, err := AcquireChatLock(c, rds, "chat")
	require.NoError(t, err)
	require.NotNil(t, successor)

	require.NoError(t, original.Release(c, rds))

	require.Equal(t, successor.Token, rds.Get(c, successor.Key).Val())
}

func TestChatLockRenew_ExtendsOnlyCurrentOwnersLease(t *testing.T) {
	server := miniredis.RunT(t)
	rds := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, rds.Close()) })
	c := context.Background()
	lock, err := AcquireChatLock(c, rds, "chat")
	require.NoError(t, err)
	server.FastForward(10 * time.Second)

	renewed, err := lock.renew(c, rds)

	require.NoError(t, err)
	require.True(t, renewed)
	require.Equal(t, ChatLockTTL, server.TTL(lock.Key))
	require.NoError(t, rds.Set(c, lock.Key, "successor", 5*time.Second).Err())
	renewed, err = lock.renew(c, rds)
	require.NoError(t, err)
	require.False(t, renewed)
	require.Equal(t, 5*time.Second, server.TTL(lock.Key))
	require.Equal(t, "successor", rds.Get(c, lock.Key).Val())
}
