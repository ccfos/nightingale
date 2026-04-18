package aiagent

import (
	"sync"
	"time"

	"github.com/toolkits/pkg/logger"
)

// streamTTL 和 cleanupTick 迁移至 defaults.go（StreamTTL / StreamCleanupTick）。

// StreamMessage is the SSE payload sent to the client.
type StreamMessage struct {
	V string `json:"v"` // value/delta
	P string `json:"p"` // "content" or "reason"
}

// StreamData holds the state of a single stream.
type StreamData struct {
	mu        sync.RWMutex
	messages  []StreamMessage
	consumers []chan StreamMessage
	isFinish  bool
	expire    time.Time
}

// StreamCache is a global in-memory store for SSE streams.
type StreamCache struct {
	mu      sync.RWMutex
	streams map[string]*StreamData
}

var globalStreamCache *StreamCache
var streamCacheOnce sync.Once

// GetStreamCache returns the global StreamCache singleton.
func GetStreamCache() *StreamCache {
	streamCacheOnce.Do(func() {
		globalStreamCache = &StreamCache{
			streams: make(map[string]*StreamData),
		}
		go globalStreamCache.cleanupLoop()
	})
	return globalStreamCache
}

// Create initializes a new stream.
func (sc *StreamCache) Create(streamID string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.streams[streamID] = &StreamData{
		expire: time.Now().Add(StreamTTL),
	}
}

// AddContent appends a content message to the stream.
func (sc *StreamCache) AddContent(streamID, delta string) {
	sc.add(streamID, StreamMessage{V: delta, P: "content"})
}

// AddReason appends a reason/thinking message to the stream.
func (sc *StreamCache) AddReason(streamID, delta string) {
	sc.add(streamID, StreamMessage{V: delta, P: "reason"})
}

func (sc *StreamCache) add(streamID string, msg StreamMessage) {
	sc.mu.RLock()
	sd, ok := sc.streams[streamID]
	sc.mu.RUnlock()
	if !ok {
		return
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.messages = append(sd.messages, msg)
	// Notify all consumers
	for _, ch := range sd.consumers {
		select {
		case ch <- msg:
		default:
			// Consumer too slow, drop
		}
	}
}

// Finish marks the stream as finished, closes all consumer channels,
// and shortens the TTL to 5 minutes (aligned with fc-model).
func (sc *StreamCache) Finish(streamID string) {
	sc.mu.RLock()
	sd, ok := sc.streams[streamID]
	sc.mu.RUnlock()
	if !ok {
		return
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.isFinish = true
	sd.expire = time.Now().Add(5 * time.Minute)
	for _, ch := range sd.consumers {
		close(ch)
	}
	sd.consumers = nil
}

// Read returns a channel that replays existing messages then receives new ones in real-time.
// The returned channel is closed when the stream finishes.
func (sc *StreamCache) Read(streamID string) <-chan StreamMessage {
	sc.mu.RLock()
	sd, ok := sc.streams[streamID]
	sc.mu.RUnlock()

	out := make(chan StreamMessage, 256)
	if !ok {
		close(out)
		return out
	}

	sd.mu.Lock()

	if sd.isFinish {
		// Stream already done: copy messages, close, return.
		cached := make([]StreamMessage, len(sd.messages))
		copy(cached, sd.messages)
		sd.mu.Unlock()

		go func() {
			for _, msg := range cached {
				out <- msg
			}
			close(out)
		}()
		return out
	}

	// Stream is still live. Take a snapshot of existing messages, then
	// register a live consumer channel for future ones.
	cached := make([]StreamMessage, len(sd.messages))
	copy(cached, sd.messages)

	live := make(chan StreamMessage, 256)
	sd.consumers = append(sd.consumers, live)
	sd.mu.Unlock()

	// Goroutine: replay cached messages into out, then forward live messages.
	go func() {
		for _, msg := range cached {
			out <- msg
		}
		for msg := range live {
			out <- msg
		}
		close(out)
	}()

	return out
}

func (sc *StreamCache) cleanupLoop() {
	ticker := time.NewTicker(StreamCleanupTick)
	defer ticker.Stop()
	for range ticker.C {
		sc.cleanup()
	}
}

func (sc *StreamCache) cleanup() {
	now := time.Now()
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for id, sd := range sc.streams {
		if now.After(sd.expire) {
			sd.mu.Lock()
			for _, ch := range sd.consumers {
				close(ch)
			}
			sd.consumers = nil
			sd.mu.Unlock()
			delete(sc.streams, id)
			logger.Debugf("[StreamCache] cleaned up expired stream: %s", id)
		}
	}
}
