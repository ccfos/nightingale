package es

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// XPackSQLRequest is the neutral request structure for ES SQL queries.
// Field names align with the ES SQL REST API spec.
type XPackSQLRequest struct {
	Query string `json:"query"`

	TimeZone                string          `json:"time_zone,omitempty"`
	FetchSize               int             `json:"fetch_size,omitempty"`
	Cursor                  string          `json:"cursor,omitempty"`
	Filter                  json.RawMessage `json:"filter,omitempty"`
	FieldMultiValueLeniency bool            `json:"field_multi_value_leniency,omitempty"`

	// Macro expansion params (not sent to ES, only for internal expansion)
	From       int64  `json:"from,omitempty"`
	To         int64  `json:"to,omitempty"`
	TimeFormat string `json:"time_format,omitempty"`
}

// XPackSQLColumn represents a column in the SQL result.
type XPackSQLColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// XPackSQLResponse is the normalized ES SQL response.
type XPackSQLResponse struct {
	Columns []XPackSQLColumn `json:"columns"`
	Rows    [][]any          `json:"rows"`
	Cursor  string           `json:"cursor,omitempty"`
}

// XPackSQL is the single entry point for ES SQL execution.
// It uses the go-elasticsearch/v8 SDK which is wire-compatible with ES 7.x, 8.x, and 9.x.
func XPackSQL(ctx context.Context, escli *Elasticsearch, req XPackSQLRequest) (*XPackSQLResponse, error) {
	if !IsESSQLSupported(escli.Version) {
		return nil, fmt.Errorf("ES SQL requires version 7.x or higher, got %q", escli.Version)
	}

	if strings.Contains(req.Query, "$__") {
		expanded, err := ExpandTimeMacros(req.Query, req.From, req.To, req.TimeZone, req.TimeFormat)
		if err != nil {
			return nil, fmt.Errorf("macro expansion failed: %w", err)
		}
		req.Query = expanded
	}

	return xpackSQLExec(ctx, escli, req)
}

// marshalSQLBody builds the JSON request body for the ES SQL REST API.
func marshalSQLBody(req XPackSQLRequest) ([]byte, error) {
	reqBody := map[string]interface{}{
		"query": req.Query,
	}
	if req.Cursor != "" {
		reqBody["cursor"] = req.Cursor
	}
	if req.TimeZone != "" {
		reqBody["time_zone"] = req.TimeZone
	}
	if req.FetchSize > 0 {
		reqBody["fetch_size"] = req.FetchSize
	}
	if len(req.Filter) > 0 {
		var filterDSL interface{}
		if err := json.Unmarshal(req.Filter, &filterDSL); err != nil {
			return nil, fmt.Errorf("failed to unmarshal filter DSL: %w", err)
		}
		reqBody["filter"] = filterDSL
	}
	if req.FieldMultiValueLeniency {
		reqBody["field_multi_value_leniency"] = true
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal SQL request: %w", err)
	}
	return data, nil
}

// decodeSQLResponse parses the ES SQL JSON response into our neutral struct.
func decodeSQLResponse(body io.Reader) (*XPackSQLResponse, error) {
	var raw struct {
		Columns []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"columns"`
		Rows   [][]interface{} `json:"rows"`
		Cursor string          `json:"cursor"`
	}

	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode SQL response: %w", err)
	}

	result := &XPackSQLResponse{
		Columns: make([]XPackSQLColumn, 0, len(raw.Columns)),
		Rows:    make([][]any, 0, len(raw.Rows)),
		Cursor:  raw.Cursor,
	}

	for _, col := range raw.Columns {
		result.Columns = append(result.Columns, XPackSQLColumn{
			Name: col.Name,
			Type: col.Type,
		})
	}

	for _, row := range raw.Rows {
		result.Rows = append(result.Rows, row)
	}

	return result, nil
}

// majorVersion extracts the major version from an ES version string.
// e.g. "8.19.4" → 8, "9.3.2" → 9, "8.14.0-SNAPSHOT" → 8
func majorVersion(version string) (int, error) {
	if version == "" {
		return 0, fmt.Errorf("version string is empty")
	}

	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid version format: %s", version)
	}

	majorStr := parts[0]
	if dashIdx := strings.Index(majorStr, "-"); dashIdx > 0 {
		majorStr = majorStr[:dashIdx]
	}

	major, err := strconv.Atoi(majorStr)
	if err != nil {
		return 0, fmt.Errorf("invalid major version in %s: %w", version, err)
	}

	return major, nil
}

// IsESSQLSupported reports whether the given ES version supports the SQL endpoint.
// Requires major version >= 7.
func IsESSQLSupported(version string) bool {
	major, err := majorVersion(version)
	if err != nil {
		return false
	}
	return major >= 7
}
