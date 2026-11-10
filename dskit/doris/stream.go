package doris

import (
	"context"
	"fmt"
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
	rows, err := db.QueryContext(queryCtx, query)
	if err != nil {
		return 0, err
	}
	defer func() { cancel(); rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	values := make([]rowValue, len(columns))
	dest := make([]any, len(columns))
	for i := range values {
		dest[i] = &values[i]
	}
	for count < limit && rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			// ExecQuery historically skips rows that cannot be scanned.
			if skipScanErrors {
				continue
			}
			return count, err
		}
		row := make(map[string]any, len(columns))
		for i, name := range columns {
			row[name] = values[i].value
			values[i].value = nil
		}
		if err := consume(row); err != nil {
			return count, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return count, fmt.Errorf("error iterating rows: %w", err)
	}
	return count, nil
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
