package es

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/toolkits/pkg/logger"
)

// xpackSQLExec executes a SQL query via the go-elasticsearch/v8 client.
// The v8 SDK's esapi.SQLQueryRequest sends a plain POST to /_sql,
// which is wire-compatible with ES 7.x, 8.x, and 9.x.
// If the caller's ctx has no deadline, a per-request timeout derived from
// the datasource configuration (es.timeout) is applied to guard against
// hung responses during body reads.
func xpackSQLExec(ctx context.Context, escli *Elasticsearch, req XPackSQLRequest) (*XPackSQLResponse, error) {
	client, err := officialClient(escli)
	if err != nil {
		return nil, err
	}

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		timeout := time.Duration(escli.Timeout) * time.Millisecond
		if timeout == 0 {
			timeout = time.Duration(defaultTimeout) * time.Millisecond
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	bodyJSON, err := marshalSQLBody(req)
	if err != nil {
		return nil, err
	}

	res, err := client.SQL.Query(
		bytes.NewReader(bodyJSON),
		client.SQL.Query.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("SQL query failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("SQL query error: %s", res.String())
	}

	return decodeSQLResponse(res.Body)
}

const clearCursorTimeout = 10 * time.Second

// xpackSQLClearCursor releases a server-side SQL cursor.
// Uses a fixed 10s timeout to avoid goroutine leaks on network issues.
// Errors are logged but not returned since cursor cleanup is best-effort.
func xpackSQLClearCursor(escli *Elasticsearch, cursor string) {
	if cursor == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), clearCursorTimeout)
	defer cancel()

	client, err := officialClient(escli)
	if err != nil {
		logger.Warningf("clear SQL cursor: failed to get ES client: %v", err)
		return
	}

	body, _ := json.Marshal(map[string]string{"cursor": cursor})
	res, err := client.SQL.ClearCursor(
		bytes.NewReader(body),
		client.SQL.ClearCursor.WithContext(ctx),
	)
	if err != nil {
		logger.Warningf("clear SQL cursor failed: %v", err)
		return
	}
	defer res.Body.Close()

	if res.IsError() {
		logger.Warningf("clear SQL cursor error: %s", res.String())
	}
}
