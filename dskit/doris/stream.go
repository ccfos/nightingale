package doris

import (
	"context"
	"database/sql"
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
	var turn *streamTurn
	if !skipScanErrors {
		turn, _ = ctx.Value(streamOrderKey{}).(*streamTurn)
		if turn != nil {
			db = turn.pool
			defer turn.release()
			if err := turn.wait(queryCtx); err != nil {
				return 0, err
			}
		}
	}
	conn, err := db.Conn(queryCtx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if turn != nil {
		turn.release()
	}
	var queryErr error
	rawErr := conn.Raw(func(raw any) error {
		dc := raw.(driver.Conn)
		open, replaced := true, false
		closeCurrent := func() {
			if open {
				_ = dc.Close()
				open = false
			}
		}
		defer func() {
			if replaced {
				closeCurrent()
			}
		}()
		var rows driver.Rows
		for attempt := range 3 {
			if queryErr = queryCtx.Err(); queryErr != nil {
				break
			}
			if attempt > 0 {
				// Keep the pool lease while retrying, so later ordered queries cannot
				// occupy our slot and block retries behind their output.
				dc, queryErr = d.openFreshConnection(queryCtx, db, database)
				if queryErr != nil {
					if errors.Is(queryErr, driver.ErrBadConn) {
						continue
					}
					break
				}
				open = true
			}
			querier, ok := dc.(driver.QueryerContext)
			if !ok {
				queryErr = fmt.Errorf("Doris driver does not support QueryContext")
				break
			}
			rows, queryErr = querier.QueryContext(queryCtx, query, nil)
			if queryErr == nil {
				break
			}
			if ctxErr := queryCtx.Err(); ctxErr != nil {
				queryErr = ctxErr
				closeCurrent()
				replaced = true
				break
			}
			if !errors.Is(queryErr, driver.ErrBadConn) {
				break
			}
			closeCurrent()
			replaced = true
		}
		if queryErr != nil {
			if replaced {
				return driver.ErrBadConn
			}
			return queryErr
		}
		exhausted, closed := false, false
		defer func() {
			if !exhausted {
				cancel()
				closeCurrent()
			}
			if !closed {
				_ = rows.Close()
			}
		}()
		count, queryErr, exhausted = consumeDriverRows(queryCtx, rows, limit, skipScanErrors, consume)
		if !exhausted {
			return driver.ErrBadConn
		}
		if closeErr := rows.Close(); closeErr != nil {
			exhausted = false
			queryErr = fmt.Errorf("error iterating rows: %w", closeErr)
		}
		closed = true
		if !exhausted || replaced {
			return driver.ErrBadConn
		}
		return queryErr
	})
	if queryErr != nil {
		return count, queryErr
	}
	if rawErr != nil && !errors.Is(rawErr, driver.ErrBadConn) {
		return count, rawErr
	}
	return count, nil
}

func (d *Doris) readDSN(database string) string {
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8", d.User, d.Password, d.Addr, database)
}

func (d *Doris) openFreshConnection(ctx context.Context, db *sql.DB, database string) (driver.Conn, error) {
	factory, ok := db.Driver().(driver.DriverContext)
	if !ok {
		return nil, fmt.Errorf("Doris driver does not support contextual connections")
	}
	connector, err := factory.OpenConnector(d.readDSN(database))
	if err != nil {
		return nil, err
	}
	return connector.Connect(ctx)
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
