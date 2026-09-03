package aiagent

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStreamBus(t *testing.T) (*streamBus, *redis.Client, *miniredis.Miniredis) {
	t.Helper()
	s, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(s.Close)

	cli := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = cli.Close() })

	// miniredis 不响应连接关闭，必须等 BLOCK 超时；测试里把 timeout 调到 200ms
	// 让 ctx 取消的测试能在合理时间收敛。函数返回时还原。
	prev := xreadBlockTimeout
	xreadBlockTimeout = 200 * time.Millisecond
	t.Cleanup(func() { xreadBlockTimeout = prev })

	return &streamBus{rds: cli}, cli, s
}

const (
	tChatID   = "chat-test"
	tStreamID = "chat-test:stream-1"
)

// 收集 channel 上所有消息直到关闭或超时。
func drain(t *testing.T, ch <-chan StreamMessage, deadline time.Duration) []StreamMessage {
	t.Helper()
	var got []StreamMessage
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	for {
		select {
		case m, ok := <-ch:
			if !ok {
				return got
			}
			got = append(got, m)
		case <-timer.C:
			t.Fatalf("drain timeout, got %d so far", len(got))
		}
	}
}

func TestStreamBus_AppendThenReadReplay(t *testing.T) {
	// 先 Append 几条再 Finish，再 Read：消费者应拿到全量历史并自然 close
	bus, _, _ := newTestStreamBus(t)
	ctx := context.Background()

	require.NoError(t, bus.Append(ctx, tChatID, tStreamID, StreamMessage{P: "content", V: "a"}))
	require.NoError(t, bus.Append(ctx, tChatID, tStreamID, StreamMessage{P: "content", V: "b"}))
	require.NoError(t, bus.Append(ctx, tChatID, tStreamID, StreamMessage{P: "reason", V: "c"}))
	require.NoError(t, bus.Finish(ctx, tChatID, tStreamID))

	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	got := drain(t, bus.Read(readCtx, tChatID, tStreamID), 3*time.Second)
	assert.Equal(t, []StreamMessage{
		{P: "content", V: "a"},
		{P: "content", V: "b"},
		{P: "reason", V: "c"},
	}, got)
	// finish 标记本身不应被推到 out
}

func TestStreamBus_LiveReadBeforeFinish(t *testing.T) {
	// Read 先开始（XREAD BLOCK），随后 Append → 消费者立即收到
	bus, _, _ := newTestStreamBus(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := bus.Read(ctx, tChatID, tStreamID)

	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = bus.Append(ctx, tChatID, tStreamID, StreamMessage{P: "content", V: "live"})
		time.Sleep(50 * time.Millisecond)
		_ = bus.Finish(ctx, tChatID, tStreamID)
	}()

	got := drain(t, ch, 3*time.Second)
	assert.Equal(t, []StreamMessage{{P: "content", V: "live"}}, got)
}

func TestStreamBus_MultipleConsumers(t *testing.T) {
	// 同一 stream 多个消费者，每个都收到全量 (Streams 是 fan-out 读，不是消费组)
	bus, _, _ := newTestStreamBus(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, bus.Append(ctx, tChatID, tStreamID, StreamMessage{P: "content", V: "x"}))
	require.NoError(t, bus.Finish(ctx, tChatID, tStreamID))

	const N = 3
	results := make([][]StreamMessage, N)
	done := make(chan int, N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			results[i] = drain(t, bus.Read(ctx, tChatID, tStreamID), 3*time.Second)
			done <- i
		}()
	}
	for i := 0; i < N; i++ {
		<-done
	}
	for i := 0; i < N; i++ {
		assert.Equal(t, []StreamMessage{{P: "content", V: "x"}}, results[i],
			"consumer %d should see full history", i)
	}
}

func TestStreamBus_ContextCancelClosesChannel(t *testing.T) {
	// ctx 取消 → goroutine 退出 → out chan 关闭，不必等到 finish/timeout
	bus, _, _ := newTestStreamBus(t)
	ctx, cancel := context.WithCancel(context.Background())

	ch := bus.Read(ctx, tChatID, tStreamID)
	// 给 goroutine 时间进入 XREAD BLOCK
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case _, ok := <-ch:
		assert.False(t, ok, "channel should be closed after ctx cancel")
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close after ctx cancel")
	}
}

func TestStreamBus_FinishMarkerNotForwarded(t *testing.T) {
	// finish entry 不能被推到 out chan——消费者只看到正常 chunk
	bus, _, _ := newTestStreamBus(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	require.NoError(t, bus.Append(ctx, tChatID, tStreamID, StreamMessage{P: "content", V: "only"}))
	require.NoError(t, bus.Finish(ctx, tChatID, tStreamID))

	got := drain(t, bus.Read(ctx, tChatID, tStreamID), 2*time.Second)
	require.Len(t, got, 1)
	assert.Equal(t, "content", got[0].P, "finish marker leaked into output")
	assert.NotEqual(t, finishMarker, got[0].P)
}

func TestStreamBus_InitMarkerNotForwarded(t *testing.T) {
	// Init 写入的 marker 仅占位让 Exists 通过，不应被推到 SSE 消费者
	bus, _, _ := newTestStreamBus(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	require.NoError(t, bus.Init(ctx, tChatID, tStreamID))
	require.NoError(t, bus.Append(ctx, tChatID, tStreamID, StreamMessage{P: "content", V: "x"}))
	require.NoError(t, bus.Finish(ctx, tChatID, tStreamID))

	got := drain(t, bus.Read(ctx, tChatID, tStreamID), 2*time.Second)
	require.Len(t, got, 1)
	assert.Equal(t, StreamMessage{P: "content", V: "x"}, got[0])
}

func TestStreamBus_Exists(t *testing.T) {
	// Init 后 Exists=true；未 Init 的 streamID Exists=false
	bus, _, _ := newTestStreamBus(t)
	ctx := context.Background()

	exists, err := bus.Exists(ctx, tChatID, tStreamID)
	require.NoError(t, err)
	assert.False(t, exists, "stream should not exist before any write")

	require.NoError(t, bus.Init(ctx, tChatID, tStreamID))

	exists, err = bus.Exists(ctx, tChatID, tStreamID)
	require.NoError(t, err)
	assert.True(t, exists, "stream should exist after Init")
}

func TestStreamBus_InitTTLApplied(t *testing.T) {
	// Init 也得设 TTL，否则 owner 崩溃后空 stream 会无限驻留
	bus, _, srv := newTestStreamBus(t)
	ctx := context.Background()

	require.NoError(t, bus.Init(ctx, tChatID, tStreamID))

	ttl := srv.TTL("aichat:stream:{" + tChatID + "}:" + tStreamID)
	assert.True(t, ttl > 0, "expected positive TTL after Init, got %s", ttl)
	assert.True(t, ttl <= streamTTL, "TTL %s should not exceed configured streamTTL", ttl)
}

func TestStreamBus_StreamTTLApplied(t *testing.T) {
	// Append 应当让 stream key 拥有 TTL（防止 owner 崩溃后无限驻留）
	bus, _, srv := newTestStreamBus(t)
	ctx := context.Background()

	require.NoError(t, bus.Append(ctx, tChatID, tStreamID, StreamMessage{P: "content", V: "x"}))

	ttl := srv.TTL("aichat:stream:{" + tChatID + "}:" + tStreamID)
	assert.True(t, ttl > 0, "expected positive TTL after Append, got %s", ttl)
	assert.True(t, ttl <= streamTTL, "TTL %s should not exceed configured streamTTL", ttl)
}
