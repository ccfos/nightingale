package storage

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 用 miniredis 启动一个伪 Redis；底层 client 是 *redis.Client，所以 NewPubsubBus
// 会走非 sharded 分支——sharded 路径只能在真实 Redis 7+ Cluster 上验证（PR5）。
func newMiniRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	s, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(s.Close)

	cli := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = cli.Close() })

	require.NoError(t, cli.Ping(context.Background()).Err())
	return cli
}

func TestPubsubBus_StandaloneNotSharded(t *testing.T) {
	cli := newMiniRedisClient(t)
	bus := NewPubsubBus(cli)

	// standalone 永远不应启用 sharded
	impl := bus.(*pubsubBus)
	assert.False(t, impl.sharded)
}

func TestPubsubBus_PublishSubscribeRoundtrip(t *testing.T) {
	cli := newMiniRedisClient(t)
	bus := NewPubsubBus(cli)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	const channel = "test:roundtrip"
	ps := bus.Subscribe(ctx, channel)
	require.NotNil(t, ps)
	defer ps.Close()

	// Subscribe 是异步的，先等订阅就位（拿到第一个确认消息再开始 Publish，
	// 否则可能在订阅完成前发，丢消息）。
	_, err := ps.Receive(ctx)
	require.NoError(t, err)

	go func() {
		// 给订阅者一点时间进入接收循环
		time.Sleep(50 * time.Millisecond)
		_ = bus.Publish(ctx, channel, "hello")
	}()

	select {
	case msg := <-ps.Channel():
		assert.Equal(t, channel, msg.Channel)
		assert.Equal(t, "hello", msg.Payload)
	case <-ctx.Done():
		t.Fatal("timeout waiting for published message")
	}
}

func TestPubsubBus_MultipleSubscribers(t *testing.T) {
	cli := newMiniRedisClient(t)
	bus := NewPubsubBus(cli)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	const channel = "test:fanout"

	const N = 3
	chans := make([]<-chan *redis.Message, N)
	subs := make([]*redis.PubSub, N)
	for i := 0; i < N; i++ {
		ps := bus.Subscribe(ctx, channel)
		require.NotNil(t, ps)
		_, err := ps.Receive(ctx)
		require.NoError(t, err)
		subs[i] = ps
		chans[i] = ps.Channel()
	}
	defer func() {
		for _, ps := range subs {
			_ = ps.Close()
		}
	}()

	time.Sleep(50 * time.Millisecond)
	require.NoError(t, bus.Publish(ctx, channel, "broadcast"))

	for i, ch := range chans {
		select {
		case msg := <-ch:
			assert.Equal(t, "broadcast", msg.Payload, "subscriber %d", i)
		case <-ctx.Done():
			t.Fatalf("subscriber %d timeout", i)
		}
	}
}
