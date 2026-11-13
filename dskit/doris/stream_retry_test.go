package doris

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/pool"
)

type stalePoolConnector struct {
	mu        sync.Mutex
	opened    int
	stale     bool
	check     bool
	queries   []int
	freshErr  error
	freshWait chan struct{}
}

func (c *stalePoolConnector) Driver() driver.Driver { return rawTestDriver{c} }
func (p *stalePoolConnector) Connect(ctx context.Context) (driver.Conn, error) {
	p.mu.Lock()
	p.opened++
	id, stale, err, wait := p.opened, p.stale, p.freshErr, p.freshWait
	p.mu.Unlock()
	if stale && id > 5 {
		if err != nil {
			return nil, err
		}
		if wait != nil {
			close(wait)
			<-ctx.Done()
			return nil, ctx.Err()
		}
	}
	return &stalePoolConn{parent: p, id: id}, nil
}

type stalePoolConn struct {
	driver.Conn
	parent *stalePoolConnector
	id     int
}

func (*stalePoolConn) Close() error { return nil }
func (c *stalePoolConn) ResetSession(context.Context) error {
	p := c.parent
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.check && p.stale && c.id <= 5 {
		return driver.ErrBadConn
	}
	return nil
}
func (c *stalePoolConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	p := c.parent
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queries = append(p.queries, c.id)
	if p.stale && c.id <= 5 {
		return nil, driver.ErrBadConn
	}
	return &retryTestRows{}, nil
}

type retryTestRows struct{ done bool }

func (*retryTestRows) Columns() []string { return []string{"v"} }
func (*retryTestRows) Close() error      { return nil }
func (r *retryTestRows) Next(v []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	v[0] = int64(1)
	return nil
}
func staleIdlePool(t *testing.T, check bool) (*sql.DB, *stalePoolConnector) {
	t.Helper()
	p := &stalePoolConnector{check: check}
	db := sql.OpenDB(p)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	t.Cleanup(func() { db.Close() })
	var held []*sql.Conn
	for range 5 {
		c, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, c)
	}
	for _, c := range held {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if db.Stats().Idle != 5 {
		t.Fatal(db.Stats())
	}
	p.stale = true
	return db, p
}
func TestExecQueryRawStalePoolRecovery(t *testing.T) {
	for _, mode := range []string{"sql_QueryContext", "current_Raw", "current_Raw_ResetSession_detects_stale"} {
		t.Run(mode, func(t *testing.T) {
			db, p := staleIdlePool(t, mode == "current_Raw_ResetSession_detects_stale")
			var err error
			var n int64
			if mode == "sql_QueryContext" {
				var rows *sql.Rows
				rows, err = db.QueryContext(context.Background(), "select")
				if err == nil {
					for rows.Next() {
						n++
					}
					err = rows.Err()
					rows.Close()
				}
			} else {
				client := &Doris{Addr: t.Name(), Timeout: 1000}
				key := client.Addr + "::"
				pool.PoolClient.Store(key, db)
				t.Cleanup(func() { pool.PoolClient.Delete(key) })
				n, err = client.ExecQueryStream(context.Background(), "", "select", math.MaxInt64, func(map[string]any) error { return nil })
			}
			t.Logf("mode=%s rows=%d err=%v queried_connection_ids=%v idle_after=%d opened=%d", mode, n, err, p.queries, db.Stats().Idle, p.opened)
			if n != 1 || err != nil {
				t.Fatalf("rows=%d err=%v", n, err)
			}
			if mode == "current_Raw" && (len(p.queries) != 2 || p.queries[0] != 5 || p.queries[1] <= 5) {
				t.Fatalf("did not retry on a fresh connection: %v", p.queries)
			}

		})
	}
}
func TestExecQueryRawFreshPoolRetainsThreeAttempts(t *testing.T) {
	client, c, db := rawTestClient(t, rawPlan{openErr: driver.ErrBadConn}, rawPlan{openErr: driver.ErrBadConn}, rawPlan{rows: 2})
	t.Logf("initial idle=%d suggested attempt budget=%d", db.Stats().Idle, db.Stats().Idle+2)
	n, err := client.ExecQueryStream(context.Background(), "", "SELECT", math.MaxInt64, func(map[string]any) error { return nil })
	t.Logf("rows=%d err=%v queries=%d", n, err, c.queries)
	if n != 2 || err != nil || c.queries != 3 {
		t.Fatalf("third fresh attempt lost: rows=%d err=%v queries=%d", n, err, c.queries)
	}
}

func TestExecQueryRawOrderedRetrySingleConnection(t *testing.T) {
	client, c, db := rawTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c.plans = []rawPlan{{openErr: driver.ErrBadConn, onOpen: func() {
		for db.Stats().WaitCount == 0 && ctx.Err() == nil {
			time.Sleep(time.Millisecond)
		}
	}}, {rows: 2}, {rows: 2}}
	var order StreamOrder
	first, doneFirst := order.Context(ctx, db)
	defer doneFirst()
	second, doneSecond := order.Context(ctx, db)
	defer doneSecond()
	errors := make(chan error, 2)
	for _, q := range []context.Context{second, first} {
		go func(q context.Context) {
			n, e := client.ExecQueryStream(q, "", "SELECT", math.MaxInt64, func(map[string]any) error { return nil })
			if n != 2 && e == nil {
				e = io.ErrUnexpectedEOF
			}
			errors <- e
		}(q)
	}
	for range 2 {
		if e := <-errors; e != nil {
			t.Fatal(e)
		}
	}
	if c.queries != 3 || db.Stats().InUse != 0 {
		t.Fatalf("queries=%d stats=%+v", c.queries, db.Stats())
	}
}

func TestExecQueryRawFreshConnectionFailure(t *testing.T) {
	for _, kind := range []string{"error", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			db, p := staleIdlePool(t, false)
			failure := errors.New("fresh connect failed")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := error(failure)
			if kind == "error" {
				p.freshErr = failure
			} else {
				p.freshWait = make(chan struct{})
				want = context.Canceled
			}
			client := &Doris{Addr: t.Name(), Timeout: 1000}
			key := client.Addr + "::"
			pool.PoolClient.Store(key, db)
			defer pool.PoolClient.Delete(key)
			finished := make(chan error, 1)
			go func() {
				_, err := client.ExecQueryStream(ctx, "", "SELECT", math.MaxInt64, func(map[string]any) error { t.Error("unexpected row"); return nil })
				finished <- err
			}()
			if kind == "cancel" {
				select {
				case <-p.freshWait:
					cancel()
				case <-time.After(time.Second):
					t.Fatal("fresh connection not attempted")
				}
			}
			select {
			case err := <-finished:
				if !errors.Is(err, want) {
					t.Fatalf("err=%v want=%v", err, want)
				}
			case <-time.After(time.Second):
				t.Fatal("connection error did not return")
			}
			if db.Stats().InUse != 0 || len(p.queries) != 1 {
				t.Fatalf("queries=%v stats=%+v", p.queries, db.Stats())
			}
		})
	}
}
func TestExecQueryRawStreamOrderSkippedAndCanceled(t *testing.T) {
	client, _, db := rawTestClient(t)
	var order StreamOrder
	_, skip := order.Context(context.Background(), db)
	ctx, done := order.Context(context.Background(), db)
	defer done()
	skip()
	skip()
	if n, err := client.ExecQueryStream(ctx, "", "SELECT", math.MaxInt64, func(map[string]any) error { return nil }); n != 2 || err != nil {
		t.Fatalf("n=%d err=%v", n, err)
	}
	_, block := order.Context(context.Background(), db)
	defer block()
	canceled, cancel := context.WithCancel(context.Background())
	last, finish := order.Context(canceled, db)
	defer finish()
	cancel()
	if n, err := client.ExecQueryStream(last, "", "SELECT", math.MaxInt64, func(map[string]any) error { t.Fatal("read after cancellation"); return nil }); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestExecQueryRawUsesScheduledPool(t *testing.T) {
	client, original, db := rawTestClient(t)
	var order StreamOrder
	ctx, done := order.Context(context.Background(), db)
	defer done()
	other := &rawConnector{}
	replacement := sql.OpenDB(other)
	defer replacement.Close()
	pool.PoolClient.Store(client.Addr+"::", replacement)
	n, err := client.ExecQueryStream(ctx, "", "SELECT", math.MaxInt64, func(map[string]any) error { return nil })
	if n != 2 || err != nil || original.queries != 1 || other.queries != 0 {
		t.Fatalf("n=%d err=%v original=%d replacement=%d", n, err, original.queries, other.queries)
	}
}
