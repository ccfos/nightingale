package aiagent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/redis/go-redis/v9"
	"github.com/toolkits/pkg/logger"
)

// StreamMessage 是 SSE 推送给前端的 payload，也是 Redis Stream entry 的 schema。
// 字段名与原 stream_cache.StreamMessage 完全一致，前端 wire format 不变。
type StreamMessage struct {
	V string `json:"v"`
	P string `json:"p"` // "content" | "reason" | "step" | "finish"
}

const (
	// streamMaxLen 限制 Stream 长度，防止长对话内存膨胀。XADD MAXLEN ~ 5000
	// 是近似裁剪——Redis 在底层链表节点边界处裁，常驻略超 5000 不要紧。
	streamMaxLen = 5000

	// streamTTL 控制完结后多久清理。设为 24h 给跨日的迟到 detail/重连留够窗口。
	streamTTL = 24 * time.Hour

	// finishMarker 标记一个 stream 的最后一个 entry，消费者读到即退出。
	// 不转发给 SSE 客户端——defer close(out) 会让上层 SSE handler 命中 !ok 分支
	// 写出 event:finish wire 标记，与旧 StreamCache.Finish 行为完全一致。
	finishMarker = "finish"

	// initMarker 由 assistantMessageNew 在返回 streamID 给客户端之前同步写入，
	// 让 stream key 立即存在。SSE handler 用 Exists 校验时即可区分"合法但 owner
	// 还没首包"和"非法 / 已 TTL 过期"。Read 跳过此 marker，不向 SSE 客户端转发。
	initMarker = "init"
)

// xreadBlockTimeout 是 idle 兜底。Redis 在 XADD 时会立即唤醒所有阻塞读者，
// 正常路径远不会触发到 30s。设这么长是为了把心跳频率压到极低，省 CPU。
//
// 之所以做成 var：miniredis 的 XREAD BLOCK 不响应 TCP 连接关闭，只能等到 timeout
// 才检测 ctx 取消，所以单测里需要把这个值改小（真 Redis 没这个问题）。
var xreadBlockTimeout = 30 * time.Second

// StreamBus 提供分布式可见的 token 流。任意实例都能 Append 写入和 Read 订阅；
// 单一对话只有 owner 实例 Append，但消费者可以是任意实例（包括非 owner）。
type StreamBus interface {
	Init(ctx context.Context, chatID, streamID string) error
	Append(ctx context.Context, chatID, streamID string, msg StreamMessage) error
	Finish(ctx context.Context, chatID, streamID string) error
	Exists(ctx context.Context, chatID, streamID string) (bool, error)
	Read(ctx context.Context, chatID, streamID string) <-chan StreamMessage
}

// NewStreamBus returns a Redis-backed StreamBus.
func NewStreamBus(rds storage.Redis) StreamBus {
	return &streamBus{rds: rds}
}

type streamBus struct {
	rds storage.Redis
}

// Init 在客户端拿到 streamID 之前同步写入一个隐藏 init marker，让 stream key
// 立即存在。这样 SSE handler 的 Exists 校验能区分"合法、owner 即将开写"和"非法
// 或已过期"，避免对不存在的 key 做 XREAD BLOCK 而无限挂起。
func (b *streamBus) Init(ctx context.Context, chatID, streamID string) error {
	key := models.StreamKey(chatID, streamID)
	pipe := b.rds.Pipeline()
	pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		MaxLen: streamMaxLen,
		Approx: true,
		Values: map[string]interface{}{"p": initMarker, "v": ""},
	})
	pipe.Expire(ctx, key, streamTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("StreamBus.Init: %w", err)
	}
	return nil
}

// Exists 报告 stream key 是否存在。供 SSE handler 在进入 Read 之前做合法性校验。
func (b *streamBus) Exists(ctx context.Context, chatID, streamID string) (bool, error) {
	key := models.StreamKey(chatID, streamID)
	n, err := b.rds.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("StreamBus.Exists: %w", err)
	}
	return n > 0, nil
}

// Append 写入一个 chunk。XADD + EXPIRE 走 Pipeline 一次 round trip。
// 每次都刷 TTL 是有意为之：长对话(>24h)期间 stream 不会被提前过期。
func (b *streamBus) Append(ctx context.Context, chatID, streamID string, msg StreamMessage) error {
	key := models.StreamKey(chatID, streamID)
	pipe := b.rds.Pipeline()
	pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		MaxLen: streamMaxLen,
		Approx: true,
		Values: map[string]interface{}{"p": msg.P, "v": msg.V},
	})
	pipe.Expire(ctx, key, streamTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("StreamBus.Append: %w", err)
	}
	return nil
}

// Finish 写入终止标记。所有阻塞在 XREAD 的消费者会立即被 Redis 唤醒，
// 读到 finishMarker 后退出循环并 close out chan。
func (b *streamBus) Finish(ctx context.Context, chatID, streamID string) error {
	key := models.StreamKey(chatID, streamID)
	pipe := b.rds.Pipeline()
	pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		Values: map[string]interface{}{"p": finishMarker, "v": ""},
	})
	pipe.Expire(ctx, key, streamTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("StreamBus.Finish: %w", err)
	}
	return nil
}

// Read 返回一个 channel：先重放 stream 中已有的全部 entries，再持续读新 entries，
// 直到读到 finish 标记 / ctx 取消 / Redis 错误持续。channel 关闭即代表流结束。
//
// 调用方必须确保通过 ctx 取消（如 SSE 客户端断开）能传播到这里，否则 goroutine
// 会泄漏直到下一次 Redis 唤醒或 30s 超时。
func (b *streamBus) Read(ctx context.Context, chatID, streamID string) <-chan StreamMessage {
	out := make(chan StreamMessage, 256)
	key := models.StreamKey(chatID, streamID)

	go func() {
		defer close(out)

		// cursor "0" 表示从头读，包含历史 entries——保证迟到的消费者也能拿到全量。
		cursor := "0"
		for {
			if ctx.Err() != nil {
				return
			}

			res, err := b.rds.XRead(ctx, &redis.XReadArgs{
				Streams: []string{key, cursor},
				Block:   xreadBlockTimeout,
				Count:   100,
			}).Result()

			if errors.Is(err, redis.Nil) {
				// BLOCK 超时无新数据，再来一次。
				continue
			}
			if err != nil {
				// ctx 取消时 go-redis 会让 XRead 立即返回 err；优先识别 ctx。
				if ctx.Err() != nil {
					return
				}
				logger.Warningf("[StreamBus] XREAD %s: %v", key, err)
				// 避免 hot loop 打爆 Redis；同时尊重 ctx。
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
				continue
			}

			// 同一次 XREAD 可以返回多个 stream 的结果，我们只订阅一个 key。
			for _, stream := range res {
				for _, entry := range stream.Messages {
					cursor = entry.ID
					p, _ := entry.Values["p"].(string)
					v, _ := entry.Values["v"].(string)

					if p == finishMarker {
						return
					}
					if p == initMarker {
						// Init 仅用于 stream key 存在性占位，不向消费者转发。
						continue
					}

					select {
					case out <- StreamMessage{V: v, P: p}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return out
}
