package doris

import (
	"context"
	"database/sql"
	"sync"
)

// StreamOrder orders connection acquisition per pool, independently of SQL execution.
type StreamOrder struct {
	tails map[*sql.DB]<-chan struct{}
}
type streamOrderKey struct{}
type streamTurn struct {
	pool     *sql.DB
	previous <-chan struct{}
	acquired chan struct{}
	once     sync.Once
}

// Context registers readers in output order before they start. A zero StreamOrder
// is usable. Call done when the operation ends, including before any reads.
// Only ExecQueryStream participates; collected ExecQuery probes are independent.
func (o *StreamOrder) Context(ctx context.Context, pool *sql.DB) (ordered context.Context, done func()) {
	if o.tails == nil {
		o.tails = make(map[*sql.DB]<-chan struct{})
	}
	t := &streamTurn{pool: pool, previous: o.tails[pool], acquired: make(chan struct{})}
	o.tails[pool] = t.acquired
	return context.WithValue(ctx, streamOrderKey{}, t), t.release
}
func (t *streamTurn) wait(ctx context.Context) error {
	if t.previous == nil {
		return ctx.Err()
	}
	select {
	case <-t.previous:
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (t *streamTurn) release() { t.once.Do(func() { close(t.acquired) }) }
