package doris

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/ccfos/nightingale/v6/dskit/pool"
)

type streamConnector struct{ rows *streamRows }

func (c streamConnector) Connect(context.Context) (driver.Conn, error) {
	return streamConn{rows: c.rows}, nil
}
func (streamConnector) Driver() driver.Driver { return nil }

type streamConn struct {
	driver.Conn
	rows *streamRows
}

func (streamConn) Close() error { return nil }
func (c streamConn) QueryContext(ctx context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.rows.query = query
	c.rows.ctx = ctx
	return c.rows, nil
}

type streamRows struct {
	index, closed int
	body          []byte
	readErr       error
	query         string
	ctx           context.Context
	closeErr      error
}

func (*streamRows) Columns() []string { return []string{"message", "sequence", "missing"} }
func (r *streamRows) Close() error    { r.closed++; r.closeErr = r.ctx.Err(); return nil }
func (r *streamRows) Next(dest []driver.Value) error {
	if r.index == 8 {
		if r.readErr != nil {
			return r.readErr
		}
		return io.EOF
	}
	r.body[0] = byte('0' + r.index)
	dest[0], dest[1], dest[2] = r.body, int64(r.index), nil
	r.index++
	return nil
}
func streamTestClient(t *testing.T, rows *streamRows) *Doris {
	t.Helper()
	db := sql.OpenDB(streamConnector{rows: rows})
	client := &Doris{Addr: t.Name(), Timeout: 5000, MaxIdleConns: 1, MaxOpenConns: 1, MaxQueryRows: 500, ConnMaxLifetime: 60}
	key := client.Addr + "::"
	pool.PoolClient.Store(key, db)
	t.Cleanup(func() { pool.PoolClient.Delete(key); db.Close() })
	return client
}
func TestExecQueryStream(t *testing.T) {
	rows := &streamRows{body: make([]byte, 64)}
	client := streamTestClient(t, rows)
	seen := 0
	var first string
	n, err := client.ExecQueryStream(context.Background(), "", "SELECT", 20, func(row map[string]any) error {
		if rows.index != seen+1 {
			t.Fatal("read ahead of writer")
		}
		if row["sequence"] != int64(seen) || row["missing"] != nil {
			t.Fatalf("invalid types: %v", row)
		}
		if seen == 0 {
			first = row["message"].(string)
		}
		seen++
		return nil
	})
	if n != 8 || seen != 8 || err != nil || rows.closed != 1 || first[0] != '0' {
		t.Fatalf("n=%d seen=%d closed=%d first=%q err=%v", n, seen, rows.closed, first, err)
	}
}
func TestExecQueryStreamStops(t *testing.T) {
	for _, kind := range []string{"limit", "write_error", "read_error", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			rows := &streamRows{body: make([]byte, 64)}
			client := streamTestClient(t, rows)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			expected := errors.New(kind)
			limit := int64(20)
			if kind == "limit" {
				limit = 2
			}
			if kind == "read_error" {
				rows.readErr = expected
			}
			n, err := client.ExecQueryStream(ctx, "", "SELECT", limit, func(row map[string]any) error {
				if kind == "write_error" && row["sequence"] == int64(2) {
					return expected
				}
				if kind == "cancel" {
					cancel()
					return ctx.Err()
				}
				return nil
			})
			switch kind {
			case "limit":
				if n != 2 || rows.index != 2 || err != nil {
					t.Fatalf("n=%d read=%d err=%v", n, rows.index, err)
				}
			case "write_error":
				if n != 2 || rows.index != 3 || !errors.Is(err, expected) {
					t.Fatalf("n=%d read=%d err=%v", n, rows.index, err)
				}
			case "read_error":
				if n != 8 || !errors.Is(err, expected) {
					t.Fatalf("n=%d err=%v", n, err)
				}
			case "cancel":
				if n != 0 || !errors.Is(err, context.Canceled) {
					t.Fatalf("n=%d err=%v", n, err)
				}
			}
			if kind != "read_error" && !errors.Is(rows.closeErr, context.Canceled) {
				t.Fatalf("Close ran before cancellation: %v", rows.closeErr)
			}
			if rows.closed != 1 {
				t.Fatalf("closed=%d", rows.closed)
			}
		})
	}
}

func TestExecQueryAndStreamCompatibility(t *testing.T) {
	var oldEvent []QueryEvent
	saved := OnQuery
	OnQuery = func(_ context.Context, ev QueryEvent) { oldEvent = append(oldEvent, ev) }
	defer func() { OnQuery = saved }()
	rows := &streamRows{body: make([]byte, 64)}
	client := streamTestClient(t, rows)
	collected, err := client.ExecQuery(context.Background(), "", "SELECT")
	if err != nil || len(collected) != 8 {
		t.Fatalf("rows=%v err=%v", collected, err)
	}
	rows.index, rows.closed = 0, 0
	var streamed []map[string]any
	n, err := client.ExecQueryStream(context.Background(), "", "SELECT", 20, func(row map[string]any) error { streamed = append(streamed, row); return nil })
	if err != nil || n != 8 || !reflect.DeepEqual(collected, streamed) {
		t.Fatalf("count=%d err=%v unequal rows", n, err)
	}
	if len(oldEvent) != 2 || oldEvent[0].RowCount != 8 || oldEvent[1].RowCount != 8 || oldEvent[0].Err != nil || oldEvent[1].Err != nil {
		t.Fatalf("events=%+v", oldEvent)
	}
	for i, row := range streamed {
		if row["message"].(string)[0] != byte('0'+i) {
			t.Fatal("driver reused retained row storage")
		}
	}
}

func TestExecQueryPartialErrorAndAudit(t *testing.T) {
	expected := errors.New("driver read error")
	rows := &streamRows{body: make([]byte, 64), readErr: expected}
	client := streamTestClient(t, rows)
	saved := OnQuery
	defer func() { OnQuery = saved }()
	events := 0
	OnQuery = func(_ context.Context, ev QueryEvent) {
		events++
		if ev.RowCount != 8 || !errors.Is(ev.Err, expected) {
			t.Errorf("event=%+v", ev)
		}
	}
	result, err := client.ExecQuery(context.Background(), "", "SELECT")
	if len(result) != 8 || !errors.Is(err, expected) || events != 1 {
		t.Fatalf("rows=%d events=%d err=%v", len(result), events, err)
	}
}
