package es

import (
	"bytes"
	"context"
	"fmt"
)

// xpackSQLExec executes a SQL query via the go-elasticsearch/v8 client.
// The v8 SDK's esapi.SQLQueryRequest sends a plain POST to /_sql,
// which is wire-compatible with ES 7.x, 8.x, and 9.x.
func xpackSQLExec(ctx context.Context, escli *Elasticsearch, req XPackSQLRequest) (*XPackSQLResponse, error) {
	client, err := officialClient(escli)
	if err != nil {
		return nil, err
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
