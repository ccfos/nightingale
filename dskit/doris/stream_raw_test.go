package doris

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/pool"
)

type rawPlan struct {
	onOpen                     func()
	openErr, nextErr, closeErr error
	rows                       int
	block                      bool
	more                       bool
}
type rawConnector struct {
	mu          sync.Mutex
	plans       []rawPlan
	connections []*rawConn
	cursors     []*rawRows
	queries     int
	entered     chan struct{}
}

type rawTestDriver struct{ connector driver.Connector }

func (d rawTestDriver) Open(string) (driver.Conn, error) {
	return d.connector.Connect(context.Background())
}
func (d rawTestDriver) OpenConnector(string) (driver.Connector, error) { return d.connector, nil }
func (c *rawConnector) Driver() driver.Driver                          { return rawTestDriver{c} }
func (c *rawConnector) Connect(context.Context) (driver.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	conn := &rawConn{parent: c}
	c.connections = append(c.connections, conn)
	return conn, nil
}

type rawConn struct {
	driver.Conn
	parent *rawConnector
	closed bool
}

func (c *rawConn) Close() error { c.closed = true; return nil }
func (c *rawConn) QueryContext(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	p := c.parent
	p.mu.Lock()
	defer p.mu.Unlock()
	plan := rawPlan{rows: 2}
	if p.queries < len(p.plans) {
		plan = p.plans[p.queries]
	}
	p.queries++
	if plan.onOpen != nil {
		plan.onOpen()
	}
	if plan.openErr != nil {
		return nil, plan.openErr
	}
	r := &rawRows{conn: c, plan: plan, ctx: ctx, body: []byte("owned")}
	p.cursors = append(p.cursors, r)
	return r, nil
}

type rawRows struct {
	conn         *rawConn
	plan         rawPlan
	ctx          context.Context
	body         []byte
	values       []driver.Value
	next, closed int
	drained      bool
	eof          bool
}

func (*rawRows) Columns() []string { return []string{"body", "seq", "nil", "float", "bool", "time"} }
func (r *rawRows) Close() error {
	r.closed++
	if !r.eof && !r.conn.closed {
		r.drained = true
	}
	return r.plan.closeErr
}
func (r *rawRows) HasNextResultSet() bool { return r.plan.more }
func (*rawRows) NextResultSet() error     { panic("must not drain another result set") }
func (r *rawRows) Next(dest []driver.Value) error {
	for _, v := range r.values {
		if v != nil {
			panic("driver values retained until next Next")
		}
	}
	r.values = dest
	r.next++
	if r.plan.block {
		if r.conn.parent.entered != nil {
			select {
			case r.conn.parent.entered <- struct{}{}:
			default:
			}
		}
		<-r.ctx.Done()
		return r.ctx.Err()
	}
	if r.next > r.plan.rows {
		if r.plan.nextErr != nil {
			return r.plan.nextErr
		}
		r.eof = true
		return io.EOF
	}
	r.body[0] = byte('a' + r.next - 1)
	copy(dest, []driver.Value{r.body, int64(r.next), nil, 1.25, true, time.Unix(1, 0).UTC()})
	return nil
}
func rawTestClient(t *testing.T, plans ...rawPlan) (*Doris, *rawConnector, *sql.DB) {
	t.Helper()
	connector := &rawConnector{plans: plans}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	client := &Doris{Addr: t.Name(), Timeout: 5000, MaxIdleConns: 1, MaxOpenConns: 1, MaxQueryRows: 500, ConnMaxLifetime: 60}
	key := client.Addr + "::"
	pool.PoolClient.Store(key, db)
	t.Cleanup(func() { pool.PoolClient.Delete(key); db.Close() })
	return client, connector, db
}
func rawFollowup(t *testing.T, client *Doris) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	n, err := client.ExecQueryStream(ctx, "", "followup", 10, func(map[string]any) error { return nil })
	if n != 2 || err != nil {
		t.Fatalf("pool unusable: n=%d err=%v", n, err)
	}
}
func TestExecQueryRawOwnershipAndPoolReuse(t *testing.T) {
	client, c, db := rawTestClient(t)
	var saved []map[string]any
	n, err := client.ExecQueryStream(context.Background(), "", "select", 10, func(row map[string]any) error {
		r := c.cursors[0]
		for _, v := range r.values {
			if v != nil {
				t.Fatal("driver packet still referenced during consumer")
			}
		}
		saved = append(saved, row)
		return nil
	})
	if n != 2 || err != nil {
		t.Fatalf("n=%d err=%v", n, err)
	}
	expected := []map[string]any{
		{"body": "awned", "seq": int64(1), "nil": nil, "float": 1.25, "bool": true, "time": time.Unix(1, 0).UTC()},
		{"body": "bwned", "seq": int64(2), "nil": nil, "float": 1.25, "bool": true, "time": time.Unix(1, 0).UTC()},
	}
	if !reflect.DeepEqual(saved, expected) {
		t.Fatalf("rows=%#v", saved)
	}
	rawFollowup(t, client)
	if len(c.connections) != 1 || c.connections[0].closed || db.Stats().InUse != 0 {
		t.Fatalf("connections=%d stats=%+v", len(c.connections), db.Stats())
	}
	for _, r := range c.cursors {
		if r.closed != 1 || r.drained {
			t.Fatalf("closed=%d drained=%t", r.closed, r.drained)
		}
	}
}
func TestExecQueryRawStopsDiscardWithoutDrain(t *testing.T) {
	failure := errors.New("consumer failed")
	for _, tc := range []struct {
		name                  string
		plan                  rawPlan
		limit                 int64
		consumeErr, errorWant error
		count                 int64
		next                  int
	}{
		{name: "limit", plan: rawPlan{rows: 5}, limit: 1, count: 1, next: 1},
		{name: "zero_limit", plan: rawPlan{rows: 5}, limit: 0, next: 0},
		{name: "negative_limit", plan: rawPlan{rows: 5}, limit: -1, next: 0},
		{name: "exact_limit", plan: rawPlan{rows: 1}, limit: 1, count: 1, next: 1},
		{name: "consumer_error", plan: rawPlan{rows: 5}, limit: 10, consumeErr: failure, errorWant: failure, next: 1},
		{name: "consumer_badconn_not_retried", plan: rawPlan{rows: 5}, limit: 10, consumeErr: driver.ErrBadConn, errorWant: driver.ErrBadConn, next: 1},
		{name: "read_error", plan: rawPlan{rows: 1, nextErr: failure}, limit: 10, errorWant: failure, count: 1, next: 2},
		{name: "read_badconn_not_retried", plan: rawPlan{nextErr: driver.ErrBadConn}, limit: 10, errorWant: driver.ErrBadConn, next: 1},
		{name: "additional_result", plan: rawPlan{rows: 1, more: true}, limit: 10, count: 1, next: 2},
		{name: "close_error", plan: rawPlan{rows: 1, closeErr: failure}, limit: 10, count: 1, next: 2, errorWant: failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, c, db := rawTestClient(t, tc.plan)
			n, err := client.ExecQueryStream(context.Background(), "", "select", tc.limit, func(map[string]any) error { return tc.consumeErr })
			if n != tc.count || !errors.Is(err, tc.errorWant) {
				t.Fatalf("n=%d err=%v want=%v", n, err, tc.errorWant)
			}
			if c.queries != 1 || len(c.connections) != 1 || !c.connections[0].closed || db.Stats().InUse != 0 {
				t.Fatalf("queries=%d connections=%d stats=%+v", c.queries, len(c.connections), db.Stats())
			}
			r := c.cursors[0]
			if r.next != tc.next || r.closed != 1 || r.drained {
				t.Fatalf("next=%d closed=%d drained=%t", r.next, r.closed, r.drained)
			}
			rawFollowup(t, client)
			if len(c.connections) != 2 {
				t.Fatalf("reused incomplete connection: %d", len(c.connections))
			}
		})
	}
}
func TestExecQueryRawOpenErrors(t *testing.T) {
	failure := errors.New("SQL rejected")
	for _, tc := range []struct {
		name                 string
		plans                []rawPlan
		want                 error
		queries, connections int
	}{
		{"SQL_error_reuses", []rawPlan{{openErr: failure}}, failure, 1, 1},
		{"bad_connection_retries", []rawPlan{{openErr: driver.ErrBadConn}, {rows: 2}}, nil, 2, 3},
		{"bounded_retry", []rawPlan{{openErr: driver.ErrBadConn}, {openErr: driver.ErrBadConn}, {openErr: driver.ErrBadConn}}, driver.ErrBadConn, 3, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, c, _ := rawTestClient(t, tc.plans...)
			n, err := client.ExecQueryStream(context.Background(), "", "select", 10, func(map[string]any) error { return nil })
			if !errors.Is(err, tc.want) || c.queries != tc.queries {
				t.Fatalf("n=%d err=%v queries=%d", n, err, c.queries)
			}
			rawFollowup(t, client)
			if len(c.connections) != tc.connections {
				t.Fatalf("connections=%d want=%d", len(c.connections), tc.connections)
			}
		})
	}
}
func TestExecQueryRawCancellationAndTimeout(t *testing.T) {
	for _, kind := range []string{"before_acquire", "during_read", "timeout"} {
		t.Run(kind, func(t *testing.T) {
			client, c, db := rawTestClient(t, rawPlan{block: true})
			c.entered = make(chan struct{}, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			expected := error(context.Canceled)
			if kind == "before_acquire" {
				cancel()
			}
			if kind == "timeout" {
				client.Timeout = 25
				expected = context.DeadlineExceeded
			}
			done := make(chan error, 1)
			go func() {
				n, err := client.ExecQueryStream(ctx, "", "select", 10, func(map[string]any) error { t.Error("unexpected row"); return nil })
				if n != 0 {
					t.Errorf("n=%d", n)
				}
				done <- err
			}()
			if kind == "during_read" {
				select {
				case <-c.entered:
					cancel()
				case <-time.After(time.Second):
					t.Fatal("read not entered")
				}
			}
			select {
			case err := <-done:
				if !errors.Is(err, expected) {
					t.Fatalf("err=%v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancellation stuck")
			}
			if db.Stats().InUse != 0 {
				t.Fatal("connection lease leaked")
			}
			if kind == "before_acquire" {
				if c.queries != 0 {
					t.Fatal("query ran after cancellation")
				}
				c.plans = nil
			} else {
				r := c.cursors[0]
				if !r.conn.closed || r.closed != 1 || r.drained {
					t.Fatalf("closed conn=%t rows=%d drained=%t", r.conn.closed, r.closed, r.drained)
				}
			}
			rawFollowup(t, client)
		})
	}
}
func TestExecQueryRawConsumerPanicReleasesPool(t *testing.T) {
	client, c, db := rawTestClient(t)
	func() {
		defer func() {
			if p := recover(); p != "consumer panic" {
				t.Fatalf("panic=%v", p)
			}
		}()
		_, _ = client.ExecQueryStream(context.Background(), "", "select", 10, func(map[string]any) error { panic("consumer panic") })
	}()
	r := c.cursors[0]
	if r.closed != 1 || r.drained || !r.conn.closed || db.Stats().InUse != 0 {
		t.Fatalf("closed=%d drained=%t stats=%+v", r.closed, r.drained, db.Stats())
	}
	rawFollowup(t, client)
	if len(c.connections) != 2 {
		t.Fatal("panicked connection reused")
	}
}

func TestExecQueryRawCancellationAtLimit(t *testing.T) {
	client, c, _ := rawTestClient(t, rawPlan{rows: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n, err := client.ExecQueryStream(ctx, "", "select", 1, func(map[string]any) error { cancel(); return nil })
	if n != 1 || !errors.Is(err, context.Canceled) {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if !c.connections[0].closed || c.cursors[0].drained {
		t.Fatal("unread response reused or drained")
	}
	rawFollowup(t, client)
}
func TestExecQueryRawCancellationDuringOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, c, _ := rawTestClient(t, rawPlan{openErr: driver.ErrBadConn, onOpen: cancel})
	n, err := client.ExecQueryStream(ctx, "", "select", 10, func(map[string]any) error { t.Fatal("unexpected callback"); return nil })
	if n != 0 || !errors.Is(err, context.Canceled) || c.queries != 1 {
		t.Fatalf("n=%d err=%v queries=%d", n, err, c.queries)
	}
	rawFollowup(t, client)
}
