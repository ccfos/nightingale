package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/storage"
	"github.com/redis/go-redis/v9"
)

// 多实例化改造：进行中的 AssistantMessage 快照、token 流、取消信号都外置到 Redis，
// 任何实例都能观察。详见 doc/design/aichat-multi-instance.md。
//
// 所有 key 用 {chatID} 作为 hash tag——Cluster 模式下这保证同 chat 的全部 key
// 落同一 slot（多 key Lua / Pipeline 不会触发 CROSSSLOT 错误）；Standalone /
// Sentinel 下 {} 是普通字符无副作用。

const msgStateTTL = 24 * time.Hour

// MsgStateKey returns the Redis key holding the JSON-serialized AssistantMessage
// snapshot for an in-flight or recently-finished message.
func MsgStateKey(chatID string, seq int64) string {
	return fmt.Sprintf("aichat:msg:{%s}:%d", chatID, seq)
}

// MsgCancelKey returns the Redis key whose existence signals "this message has
// been cancelled". Set by /cancel handler with a TTL; checked by owner as a
// fallback if the cancel pubsub notification was missed.
func MsgCancelKey(chatID string, seq int64) string {
	return fmt.Sprintf("aichat:cancel:{%s}:%d", chatID, seq)
}

// MsgCancelChannel returns the Pub/Sub channel that owners subscribe to for
// immediate cancel notifications.
func MsgCancelChannel(chatID string, seq int64) string {
	return fmt.Sprintf("aichat:cancelch:{%s}:%d", chatID, seq)
}

// StreamKey returns the Redis Stream key holding the token chunk sequence.
// Consumers use XREAD BLOCK on this stream — Redis itself wakes blocked readers
// on every XADD, so no additional pub/sub wakeup is needed.
func StreamKey(chatID, streamID string) string {
	return fmt.Sprintf("aichat:stream:{%s}:%s", chatID, streamID)
}

// MsgStateGet 读取进行中的消息快照。返回 nil 表示 key 不存在（消息已 TTL 过期
// 或从未被该机制写入），调用方应 fallback 到 DB 读历史记录。
func MsgStateGet(ctx context.Context, rds storage.Redis, chatID string, seq int64) (*AssistantMessage, error) {
	raw, err := rds.Get(ctx, MsgStateKey(chatID, seq)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var msg AssistantMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, fmt.Errorf("MsgStateGet unmarshal: %w", err)
	}
	return &msg, nil
}

// MsgStateSet 整体覆盖快照，TTL 24h。只有 owner 实例会写，没有并发写竞态。
func MsgStateSet(ctx context.Context, rds storage.Redis, msg *AssistantMessage) error {
	if msg == nil {
		return errors.New("MsgStateSet: nil message")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("MsgStateSet marshal: %w", err)
	}
	return rds.Set(ctx, MsgStateKey(msg.ChatID, msg.SeqID), data, msgStateTTL).Err()
}

// MsgStateDelete 一般不需要主动调用——TTL 会自然清理。预留给清理工具使用。
func MsgStateDelete(ctx context.Context, rds storage.Redis, chatID string, seq int64) error {
	return rds.Del(ctx, MsgStateKey(chatID, seq)).Err()
}

// MsgCancelExists 兜底检查：owner 在 ReAct 循环每轮迭代时可调用此函数确认
// 是否有迟到的 cancel 标志（pubsub 偶发漏发时使用）。
func MsgCancelExists(ctx context.Context, rds storage.Redis, chatID string, seq int64) (bool, error) {
	n, err := rds.Exists(ctx, MsgCancelKey(chatID, seq)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// MsgCancelMark 由 /cancel handler 调用：设取消标志（TTL 1h，远长于 LLM 最长响应）
func MsgCancelMark(ctx context.Context, rds storage.Redis, chatID string, seq int64) error {
	return rds.Set(ctx, MsgCancelKey(chatID, seq), "1", time.Hour).Err()
}
