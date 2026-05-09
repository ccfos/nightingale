package es

import (
	"bytes"
	"context"
	"fmt"
)

func xpackSQLViaV8(ctx context.Context, escli *Elasticsearch, req XPackSQLRequest) (*XPackSQLResponse, error) {
	client, err := officialClientV8(escli)
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
		return nil, fmt.Errorf("v8 SQL query failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("v8 SQL query error: %s", res.String())
	}

	return decodeSQLResponse(res.Body)
}
