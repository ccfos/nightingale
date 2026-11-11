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
	p, ok := decodeSQLQueryParam(queryParam)
	if !ok {
		return nil, false
	}

	return &XPackSQLRequest{
		Query:                   p.SQL,
		From:                    p.Start,
		To:                      p.End,
		FieldMultiValueLeniency: true,
	}, true
}

// IsSQLQueryLog reports whether QueryLog routes queryParam through the SQL
// branch (and thus returns plain column→value row maps instead of SearchHit
// objects). It shares decodeSQLQueryParam with extractSQLRequest, so callers
// outside the plugin — e.g. the batch query v2 router, which must know the
// shape of QueryLog's return value — cannot drift from the plugin's own
// routing decision by re-parsing the payload themselves.
func IsSQLQueryLog(queryParam interface{}) bool {
	_, ok := decodeSQLQueryParam(queryParam)
	return ok
}

// decodeSQLQueryParam is the single source of truth for the SQL branch
// routing decision: mapstructure must decode the payload and "sql" must be
// non-empty. A decode failure (e.g. a wrong-typed sql/index/start/end field)
// means QueryLog falls back to the DSL path and returns SearchHits.
func decodeSQLQueryParam(queryParam interface{}) (sqlQueryParam, bool) {
	var p sqlQueryParam
	if err := mapstructure.Decode(queryParam, &p); err != nil {
		return p, false
	}
	if p.SQL == "" {
		return p, false
	}
	return p, true
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
