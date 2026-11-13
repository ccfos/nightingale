package doris

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"time"
)

// ExecQueryStream executes SQL and synchronously consumes at most limit rows.
// Each row owns its strings and may be retained by the consumer. The next row is
// read only after consume returns. A consumer error or reaching limit cancels the
// unread response before closing rows; no remaining result is drained.
func (d *Doris) ExecQueryStream(ctx context.Context, database, query string, limit int64, consume func(map[string]any) error) (int64, error) {
	return d.readRows(ctx, database, query, limit, false, consume)
}

func (d *Doris) readRows(ctx context.Context, database, query string, limit int64, skipScanErrors bool, consume func(map[string]any) error) (count int64, err error) {
	started := time.Now()
	defer func() {
		if OnQuery != nil {
			OnQuery(ctx, QueryEvent{Database: database, SQL: query, StartedAt: started, Duration: time.Since(started), RowCount: int(count), Err: err})
		}
	}()
	if consume == nil {
		return 0, fmt.Errorf("Doris row consumer is nil")
	}
	queryCtx, cancel := d.createTimeoutContext(ctx)
	defer cancel()
	db, err := d.NewConn(queryCtx, database)
	if err != nil {
		return 0, err
	}
	// Match QueryContext's bounded retry of errors that guarantee no query ran.
	// Never replay SQL after receiving a cursor or invoking the consumer.
	for attempt := 0; attempt < 3; attempt++ {
		conn, acquireErr := db.Conn(queryCtx)
		if acquireErr != nil {
			return count, acquireErr
		}
		retry := false
		var queryErr error
		rawErr := conn.Raw(func(raw any) error {
			dc := raw.(driver.Conn)
			querier, ok := raw.(driver.QueryerContext)
			if !ok {
				queryErr = fmt.Errorf("Doris driver does not support QueryContext")
				return queryErr
			}
			if queryErr = queryCtx.Err(); queryErr != nil {
				return queryErr
			}
			rows, openErr := querier.QueryContext(queryCtx, query, nil)
			if openErr != nil {
				queryErr = openErr
				if ctxErr := queryCtx.Err(); ctxErr != nil {
					queryErr = ctxErr
					_ = dc.Close()
					return driver.ErrBadConn
				}
				retry = errors.Is(openErr, driver.ErrBadConn)
				if retry {
					_ = dc.Close()
					return driver.ErrBadConn
				}
				return openErr
			}
			exhausted := false
			closed := false
			defer func() {
				if !exhausted {
					cancel()
					// Rows.Close may drain unread results; close the socket first.
					_ = dc.Close()
				}
				if !closed {
					_ = rows.Close()
				}
			}()
			count, queryErr, exhausted = consumeDriverRows(queryCtx, rows, limit, skipScanErrors, consume)
			if !exhausted {
				// Poison the pool connection without replacing the consumer's result.
				return driver.ErrBadConn
			}
			closeErr := rows.Close()
			closed = true
			if closeErr != nil {
				exhausted = false
				queryErr = fmt.Errorf("error iterating rows: %w", closeErr)
				return driver.ErrBadConn
			}
			return queryErr
		})
		_ = conn.Close()
		if retry {
			continue
		}
		if queryErr != nil {
			return count, queryErr
		}
		if rawErr != nil && !errors.Is(rawErr, driver.ErrBadConn) {
			return count, rawErr
		}
		return count, nil
	}
	return count, driver.ErrBadConn
}

func consumeDriverRows(ctx context.Context, rows driver.Rows, limit int64, skipScanErrors bool, consume func(map[string]any) error) (count int64, err error, exhausted bool) {
	columns := rows.Columns()
	values := make([]driver.Value, len(columns))
	defer clear(values)
	for count < limit {
		if err := ctx.Err(); err != nil {
			return count, fmt.Errorf("error iterating rows: %w", err), false
		}
		nextErr := rows.Next(values)
		if nextErr != nil {
			if err := ctx.Err(); err != nil {
				return count, fmt.Errorf("error iterating rows: %w", err), false
			}
			if nextErr == io.EOF {
				more, ok := rows.(driver.RowsNextResultSet)
				return count, nil, !ok || !more.HasNextResultSet()
			}
			return count, fmt.Errorf("error iterating rows: %w", nextErr), false
		}
		if err := ctx.Err(); err != nil {
			return count, fmt.Errorf("error iterating rows: %w", err), false
		}
		row, scanErr := ownDriverRow(columns, values)
		if scanErr != nil {
			if skipScanErrors {
				continue
			}
			return count, scanErr, false
		}
		if err := consume(row); err != nil {
			return count, err, false
		}
		count++
	}
	if err := ctx.Err(); err != nil {
		return count, fmt.Errorf("error iterating rows: %w", err), false
	}
	return count, nil, false
}

func ownDriverRow(columns []string, values []driver.Value) (map[string]any, error) {
	defer clear(values)
	row := make(map[string]any, len(columns))
	for i, name := range columns {
		var value rowValue
		if err := value.Scan(values[i]); err != nil {
			return nil, fmt.Errorf("sql: Scan error on column index %d, name %q: %w", i, name, err)
		}
		row[name] = value.value
	}
	return row, nil
}

// Scanning into *any first clones driver-owned []byte before string conversion.
// Copy directly to the owned string to avoid that intermediate allocation.
type rowValue struct{ value any }

func (v *rowValue) Scan(src any) error {
	if data, ok := src.([]byte); ok {
		v.value = string(data)
	} else {
		v.value = src
	}
	return nil
}
