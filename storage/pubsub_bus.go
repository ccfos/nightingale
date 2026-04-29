package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/toolkits/pkg/logger"
)

// PubsubBus 抽象 Redis Pub/Sub，对调用方屏蔽 standalone / sentinel / cluster
// 之间的差异。Cluster 模式下若服务端支持 (Redis 7.0+) 自动启用 Sharded Pub/Sub
// (SPUBLISH/SSUBSCRIBE)，避免 cluster bus 全广播带来的 N 倍放大；其他场景退化
// 为普通 PUBLISH/SUBSCRIBE。
type PubsubBus interface {
	// Publish 发布消息。channel 在 Cluster Sharded 模式下参与 slot 哈希，调用方
	// 应将业务上"必须同节点处理"的 key 与 channel 用相同的 hash tag。
	Publish(ctx context.Context, channel, payload string) error

	// Subscribe 订阅一个或多个 channel。返回的 *redis.PubSub 由调用方负责 Close。
	Subscribe(ctx context.Context, channels ...string) *redis.PubSub
}

const pubsubProbeChannel = "n9e:pubsub:probe"

// NewPubsubBus 探测一次后缓存结果，运行期不再变。Standalone / Sentinel 永远走
// 普通 Pub/Sub；Cluster 优先尝试 SPUBLISH，失败回退普通 Pub/Sub。
func NewPubsubBus(rds Redis) PubsubBus {
	b := &pubsubBus{rds: rds}
	if cc, ok := rds.(*redis.ClusterClient); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		// Redis < 7.0 的 cluster 收到 SPUBLISH 会返回 "ERR unknown command"；
		// 任何错误都视作不支持，回退到普通模式。
		if err := cc.SPublish(ctx, pubsubProbeChannel, "x").Err(); err == nil {
			b.sharded = true
			logger.Info("[pubsub] sharded pub/sub enabled (cluster + Redis 7.0+)")
		} else {
			logger.Warningf("[pubsub] sharded pub/sub disabled: %v", err)
		}
	}
	return b
}

type pubsubBus struct {
	rds     Redis
	sharded bool
}

func (b *pubsubBus) Publish(ctx context.Context, channel, payload string) error {
	if b.sharded {
		return b.rds.SPublish(ctx, channel, payload).Err()
	}
	return b.rds.Publish(ctx, channel, payload).Err()
}

func (b *pubsubBus) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	if b.sharded {
		// b.sharded 为 true 时底层一定是 *ClusterClient，前面 NewPubsubBus 已保证。
		return b.rds.(*redis.ClusterClient).SSubscribe(ctx, channels...)
	}
	switch c := b.rds.(type) {
	case *redis.ClusterClient:
		return c.Subscribe(ctx, channels...)
	case *redis.Client:
		// *redis.Client 同时覆盖 standalone / sentinel / miniredis 三种创建路径。
		return c.Subscribe(ctx, channels...)
	}
	// 调用方往往 `defer ps.Close()`，返回 nil 会让上游 nil-deref 把整个进程带崩。
	// 配置错误应该在启动期就暴露，所以这里 fail-fast。
	panic(fmt.Sprintf("pubsubBus: unsupported redis client type %T", b.rds))
}
