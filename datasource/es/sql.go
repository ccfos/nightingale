package es

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ccfos/nightingale/v6/pkg/macros"
)

// SQLPreprocess is called before SQL execution to expand macros.
// Defaults to macros.Macro. Downstream projects (e.g. n9e-plus)
// can override this via RegisterSQLPreprocess to adapt macro
// dialects for different SQL engines.
var SQLPreprocess func(sql string, from, to int64) (string, error)

// RegisterSQLPreprocess sets a custom SQL preprocessor for ES SQL
// macro expansion, replacing the default macros.Macro delegation.
func RegisterSQLPreprocess(f func(sql string, from, to int64) (string, error)) {
	SQLPreprocess = f
}

// XPackSQLRequest is the neutral request structure for ES SQL queries.
// Field names align with the ES SQL REST API spec.
type XPackSQLRequest struct {
	Query string `json:"query"`

	FetchSize               int             `json:"fetch_size,omitempty"`
	Cursor                  string          `json:"cursor,omitempty"`
	Filter                  json.RawMessage `json:"filter,omitempty"`
	FieldMultiValueLeniency bool            `json:"field_multi_value_leniency,omitempty"`

	// Macro expansion params (not sent to ES, only for internal expansion)
	From int64 `json:"from,omitempty"`
	To   int64 `json:"to,omitempty"`
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

// maxCursorBatchSize caps the fetch_size sent to ES SQL per request.
// ES default is 1000; setting too high risks ES heap pressure / OOM.
// 10000 matches ES search.max_buckets default and is a safe upper bound
// for most cluster configurations.
const maxCursorBatchSize = 10000

// maxCursorIterations limits cursor follow-up rounds to prevent runaway
// loops (e.g. LIMIT 10000000 with fetch_size 10000 would be 1000 rounds).
// Initial batch + 100 follow-ups = 101 × 10000 = 1,010,000 rows hard ceiling.
const maxCursorIterations = 100

// parseSQLLimit extracts the LIMIT value from a SQL statement.
// Returns 0 if no LIMIT clause is found. Occurrences inside
// single-quoted or double-quoted strings are skipped.
func parseSQLLimit(sql string) int {
	stripped := stripSQLQuotedStrings(sql)
	upper := strings.ToUpper(stripped)
	idx := strings.LastIndex(upper, "LIMIT")
	if idx < 0 {
		return 0
	}
	after := strings.TrimSpace(upper[idx+5:])
	end := 0
	for end < len(after) && after[end] >= '0' && after[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, err := strconv.Atoi(after[:end])
	if err != nil {
		return 0
	}
	return n
}

// stripSQLQuoted replaces content inside single-quoted strings,
// double-quoted identifiers, line comments (--), and block comments
// (/* */) with spaces, so that keywords inside them are not matched.
func stripSQLQuotedStrings(sql string) string {
	var b strings.Builder
	b.Grow(len(sql))
	for i := 0; i < len(sql); i++ {
		ch := sql[i]

		// -- line comment: blank until newline
		if ch == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			for i < len(sql) && sql[i] != '\n' {
				b.WriteByte(' ')
				i++
			}
			if i < len(sql) {
				b.WriteByte('\n')
			}
			continue
		}

		// /* block comment */: blank until */
		if ch == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			b.WriteByte(' ')
			b.WriteByte(' ')
			i += 2
			for i < len(sql) {
				if sql[i] == '*' && i+1 < len(sql) && sql[i+1] == '/' {
					b.WriteByte(' ')
					b.WriteByte(' ')
					i++
					break
				}
				b.WriteByte(' ')
				i++
			}
			continue
		}

		// single-quoted string literal
		if ch == '\'' {
			b.WriteByte(' ')
			i++
			for i < len(sql) {
				if sql[i] == '\'' {
					if i+1 < len(sql) && sql[i+1] == '\'' {
						b.WriteByte(' ')
						b.WriteByte(' ')
						i += 2
						continue
					}
					b.WriteByte(' ')
					break
				}
				b.WriteByte(' ')
				i++
			}
			continue
		}

		// double-quoted identifier
		if ch == '"' {
			b.WriteByte(' ')
			i++
			for i < len(sql) {
				if sql[i] == '"' {
					b.WriteByte(' ')
					break
				}
				b.WriteByte(' ')
				i++
			}
			continue
		}

		b.WriteByte(ch)
	}
	return b.String()
}

// XPackSQL is the single entry point for ES SQL execution.
// It uses the go-elasticsearch/v8 SDK which is wire-compatible with ES 7.x, 8.x, and 9.x.
// Macro expansion is delegated to SQLPreprocess (defaults to macros.Macro).
//
// When the response contains a cursor (indicating more data is available),
// XPackSQL automatically fetches subsequent pages until the LIMIT is reached
// or the cursor is exhausted. This is necessary because ES SQL's default
// fetch_size is 1000, which would silently truncate larger result sets.
func XPackSQL(ctx context.Context, escli *Elasticsearch, req XPackSQLRequest) (*XPackSQLResponse, error) {
	if !IsESSQLSupported(escli.Version) {
		return nil, fmt.Errorf("ES SQL requires version 7.x or higher, got %q", escli.Version)
	}

	if strings.Contains(req.Query, "$__") {
		preprocess := SQLPreprocess
		if preprocess == nil {
			preprocess = defaultSQLPreprocess
		}
		expanded, err := preprocess(req.Query, req.From, req.To)
		if err != nil {
			return nil, fmt.Errorf("macro expansion failed: %w", err)
		}
		req.Query = expanded
	}

	limit := parseSQLLimit(req.Query)
	if limit > 0 && req.FetchSize == 0 {
		if limit <= maxCursorBatchSize {
			req.FetchSize = limit
		} else {
			req.FetchSize = maxCursorBatchSize
		}
	}

	resp, err := xpackSQLExec(ctx, escli, req)
	if err != nil {
		return nil, err
	}

	if resp.Cursor == "" || limit == 0 {
		return resp, nil
	}

	allRows := resp.Rows
	iterations := 0
	for iterations < maxCursorIterations && resp.Cursor != "" && len(allRows) < limit {
		nextReq := XPackSQLRequest{
			Cursor:    resp.Cursor,
			FetchSize: req.FetchSize,
		}
		resp, err = xpackSQLExec(ctx, escli, nextReq)
		if err != nil {
			go xpackSQLClearCursor(escli, nextReq.Cursor)
			return nil, fmt.Errorf("cursor fetch failed on iteration %d: %w", iterations+1, err)
		}
		allRows = append(allRows, resp.Rows...)
		iterations++
	}

	if resp.Cursor != "" {
		go xpackSQLClearCursor(escli, resp.Cursor)
		if len(allRows) < limit {
			return nil, fmt.Errorf("ES SQL result incomplete: fetched %d rows in %d cursor iterations "+
				"but LIMIT is %d; consider reducing LIMIT (max supported: %d)",
				len(allRows), iterations, limit, (1+maxCursorIterations)*maxCursorBatchSize)
		}
	}

	if len(allRows) > limit {
		allRows = allRows[:limit]
	}

	resp.Rows = allRows
	resp.Cursor = ""
	return resp, nil
}

func defaultSQLPreprocess(sql string, from, to int64) (string, error) {
	if macros.Macro != nil {
		return macros.Macro(sql, from, to)
	}
	return sql, nil
}

// marshalSQLBody builds the JSON request body for the ES SQL REST API.
// For cursor continuation requests (Cursor != ""), only cursor and fetch_size
// are sent — ES rejects requests that mix cursor with query/filter.
func marshalSQLBody(req XPackSQLRequest) ([]byte, error) {
	reqBody := make(map[string]interface{})

	if req.Cursor != "" {
		reqBody["cursor"] = req.Cursor
		if req.FetchSize > 0 {
			reqBody["fetch_size"] = req.FetchSize
		}
	} else {
		reqBody["query"] = req.Query
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
