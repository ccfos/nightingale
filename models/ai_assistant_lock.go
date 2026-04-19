package models

import (
	"context"
	"time"

	"github.com/ccfos/nightingale/v6/storage"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/toolkits/pkg/logger"
)

// Per-chat lock implements the Redisson "Watchdog" pattern: short TTL plus
// background renewal so a long agent run cannot outlive its lease, while a
// crashed holder's lock still expires within ChatLockTTL.
//
// Why not just bump the TTL to cover max execution time: a fixed long TTL
// holds the chat hostage for that full window if the process dies, so a
// retry on the user side keeps failing with "chat is busy". Short TTL +
// renewal gets both safety (no stale holders) and liveness (auto-recover).
const (
	ChatLockTTL           = 30 * time.Second
	ChatLockRenewInterval = 10 * time.Second
)

// Token-based CAS scripts: only the holder (matching token) may release or
// extend. Plain DEL would risk wiping another goroutine's lock if our TTL
// ever expired between operations.
var (
	chatLockReleaseScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
end
return 0
`)
	chatLockRenewScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("pexpire", KEYS[1], ARGV[2])
end
return 0
`)
)

type ChatLock struct {
	Key   string
	Token string
}

// AcquireChatLock attempts SETNX on the per-chat lock key. Returns nil
// without error when another holder exists (caller treats as "busy").
func AcquireChatLock(ctx context.Context, rds storage.Redis, chatID string) (*ChatLock, error) {
	lock := &ChatLock{
		Key:   AssistantChatLockKey(chatID),
		Token: uuid.NewString(),
	}
	ok, err := rds.SetNX(ctx, lock.Key, lock.Token, ChatLockTTL).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return lock, nil
}

// Release deletes the lock only if we still hold it (token match).
// Pass a fresh background context — release must run even when the caller's
// context has already been canceled.
func (l *ChatLock) Release(ctx context.Context, rds storage.Redis) error {
	return chatLockReleaseScript.Run(ctx, rds, []string{l.Key}, l.Token).Err()
}

// renew extends the TTL only if we still hold it. Returns false when the
// lock has been taken over by someone else (we should stop renewing).
func (l *ChatLock) renew(ctx context.Context, rds storage.Redis) (bool, error) {
	res, err := chatLockRenewScript.Run(ctx, rds, []string{l.Key}, l.Token, ChatLockTTL.Milliseconds()).Result()
	if err != nil {
		return false, err
	}
	n, _ := res.(int64)
	return n == 1, nil
}

// KeepAlive renews the lease until ctx is done or ownership is lost.
// Run as `go lock.KeepAlive(ctx, rds)`; cancel ctx to stop it. Release
// remains the caller's responsibility (deferred in the task goroutine).
func (l *ChatLock) KeepAlive(ctx context.Context, rds storage.Redis) {
	t := time.NewTicker(ChatLockRenewInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ok, err := l.renew(ctx, rds)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				logger.Warningf("[ChatLock] renew error key=%s: %v", l.Key, err)
				continue
			}
			if !ok {
				logger.Warningf("[ChatLock] lost ownership, stop renewing key=%s", l.Key)
				return
			}
		}
	}
}
