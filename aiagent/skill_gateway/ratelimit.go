package skillgateway

import (
	"sync"
	"time"
)

// tokenBucket is a tiny per-execution rate limiter (§12.4): it bounds how fast one
// skill run can hammer the gateway, so even a read-only grant like alert:read
// cannot be used to enumerate / DoS the internal API. Hand-rolled to avoid adding
// a dependency for ~20 lines.
type tokenBucket struct {
	mu     sync.Mutex
	tokens float64
	max    float64
	perSec float64
	last   time.Time
}

func newTokenBucket(perSec float64, burst int) *tokenBucket {
	if perSec <= 0 {
		perSec = 5
	}
	if burst <= 0 {
		burst = 10
	}
	return &tokenBucket{tokens: float64(burst), max: float64(burst), perSec: perSec, last: time.Now()}
}

// allow consumes one token, refilling by elapsed time first. Returns false when
// the bucket is empty (caller rejects the request).
func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.tokens += now.Sub(b.last).Seconds() * b.perSec
	if b.tokens > b.max {
		b.tokens = b.max
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}
