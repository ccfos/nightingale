package es

import (
	"context"
	"fmt"

	"github.com/mitchellh/mapstructure"
)

// sqlQueryParam is used to detect whether a queryParam contains SQL mode fields.
type sqlQueryParam struct {
	SQL   string `json:"sql" mapstructure:"sql"`
	Index string `json:"index" mapstructure:"index"`
	Start int64  `json:"start" mapstructure:"start"`
	End   int64  `json:"end" mapstructure:"end"`
}

// extractSQLRequest checks if queryParam contains a non-empty "sql" field.
// If so, it returns the constructed XPackSQLRequest and true.
func extractSQLRequest(queryParam interface{}) (*XPackSQLRequest, bool) {
	var p sqlQueryParam
	if err := mapstructure.Decode(queryParam, &p); err != nil {
		return nil, false
	}
	if p.SQL == "" {
		return nil, false
	}

	return &XPackSQLRequest{
		Query:                   p.SQL,
		From:                    p.Start,
		To:                      p.End,
		FieldMultiValueLeniency: true,
	}, true
}

// queryLogViaSQL executes a SQL query and flattens the result into the
// []interface{} format that QueryLog callers expect.
// Each row is returned as a map[string]interface{} keyed by column name.
func (e *Elasticsearch) queryLogViaSQL(ctx context.Context, req *XPackSQLRequest) ([]interface{}, int64, error) {
	resp, err := XPackSQL(ctx, e, *req)
	if err != nil {
		return nil, 0, fmt.Errorf("ES SQL query failed: %w", err)
	}

	results := make([]interface{}, 0, len(resp.Rows))
	for _, row := range resp.Rows {
		record := make(map[string]interface{}, len(resp.Columns))
		for i, col := range resp.Columns {
			if i < len(row) {
				record[col.Name] = row[i]
			}
		}
		results = append(results, record)
	}

	return results, int64(len(resp.Rows)), nil
}
