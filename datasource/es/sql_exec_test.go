package es

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/macros"
)

func syncMapNew() sync.Map { return sync.Map{} }

// mockESServer creates a test HTTP server that mimics ES cluster info and /_sql endpoint.
// When includeProductHeader is true, the server returns the X-Elastic-Product header
// (as ES >= 7.14 does). When false, it simulates ES < 7.14 behavior.
func mockESServer(t *testing.T, version, sqlResponse string, includeProductHeader bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if includeProductHeader {
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			info := map[string]interface{}{
				"name":         "test-node",
				"cluster_name": "test-cluster",
				"version": map[string]interface{}{
					"number": version,
				},
				"tagline": "You Know, for Search",
			}
			json.NewEncoder(w).Encode(info)

		case r.Method == http.MethodPost && r.URL.Path == "/_sql":
			body, _ := io.ReadAll(r.Body)
			var reqBody map[string]interface{}
			if err := json.Unmarshal(body, &reqBody); err != nil {
				t.Errorf("invalid SQL request body: %s", body)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if _, ok := reqBody["query"]; !ok {
				http.Error(w, `{"error":"missing query"}`, http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(sqlResponse))

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

func newTestElasticsearch(version string, nodes []string) *Elasticsearch {
	return &Elasticsearch{
		Version: version,
		Nodes:   nodes,
	}
}

// Recorded from real ES 7.17.10
const sqlResponseV7 = `{
  "columns": [
    {"name": "status", "type": "text"},
    {"name": "value", "type": "long"}
  ],
  "rows": [
    ["ok", 42]
  ]
}`

// Recorded from real ES 8.15.0 — same wire format as v7
const sqlResponseV8 = `{
  "columns": [
    {"name": "status", "type": "text"},
    {"name": "value", "type": "long"}
  ],
  "rows": [
    ["ok", 42]
  ]
}`

// Recorded from real ES 9.0.0 — same wire format
const sqlResponseV9 = `{
  "columns": [
    {"name": "status", "type": "text"},
    {"name": "value", "type": "long"}
  ],
  "rows": [
    ["ok", 42]
  ]
}`

func TestXPackSQL_V8SDKCompatibility(t *testing.T) {
	tests := []struct {
		name                 string
		version              string
		sqlResponse          string
		includeProductHeader bool
	}{
		{"ES_7.10.2_no_product_header", "7.10.2", sqlResponseV7, false},
		{"ES_7.17.10", "7.17.10", sqlResponseV7, true},
		{"ES_8.15.0", "8.15.0", sqlResponseV8, true},
		{"ES_9.0.0", "9.0.0", sqlResponseV9, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := mockESServer(t, tt.version, tt.sqlResponse, tt.includeProductHeader)
			defer srv.Close()

			// Clear client cache so each sub-test gets a fresh client
			clientCache = syncMapNew()

			escli := newTestElasticsearch(tt.version, []string{srv.URL})

			resp, err := XPackSQL(context.Background(), escli, XPackSQLRequest{
				Query: `SELECT status, value FROM "test_index"`,
			})
			if err != nil {
				t.Fatalf("XPackSQL() error: %v", err)
			}

			if len(resp.Columns) != 2 {
				t.Errorf("expected 2 columns, got %d", len(resp.Columns))
			}
			if resp.Columns[0].Name != "status" || resp.Columns[1].Name != "value" {
				t.Errorf("unexpected columns: %+v", resp.Columns)
			}
			if len(resp.Rows) != 1 {
				t.Errorf("expected 1 row, got %d", len(resp.Rows))
			}
			if resp.Rows[0][0] != "ok" {
				t.Errorf("expected row[0][0]='ok', got %v", resp.Rows[0][0])
			}
			val, ok := resp.Rows[0][1].(float64)
			if !ok || val != 42 {
				t.Errorf("expected row[0][1]=42, got %v (%T)", resp.Rows[0][1], resp.Rows[0][1])
			}
		})
	}
}

func TestXPackSQL_UnsupportedVersion(t *testing.T) {
	escli := newTestElasticsearch("6.8.0", []string{"http://localhost:9200"})
	_, err := XPackSQL(context.Background(), escli, XPackSQLRequest{
		Query: `SELECT * FROM "test"`,
	})
	if err == nil {
		t.Fatal("expected error for ES 6.x, got nil")
	}
}

func TestXPackSQL_NoMacroSkipsPreprocess(t *testing.T) {
	origPreprocess := SQLPreprocess
	SQLPreprocess = func(sql string, from, to int64) (string, error) {
		t.Fatalf("SQLPreprocess should not be called for SQL without macros")
		return "", nil
	}
	defer func() { SQLPreprocess = origPreprocess }()

	sqlResp := `{"columns":[{"name":"cnt","type":"long"}],"rows":[[100]]}`
	srv := mockESServer(t, "8.15.0", sqlResp, true)
	defer srv.Close()

	clientCache = syncMapNew()
	escli := newTestElasticsearch("8.15.0", []string{srv.URL})

	resp, err := XPackSQL(context.Background(), escli, XPackSQLRequest{
		Query: `SELECT COUNT(*) AS cnt FROM "logs"`,
	})
	if err != nil {
		t.Fatalf("XPackSQL() without macro error: %v", err)
	}
	if len(resp.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(resp.Rows))
	}
}

func TestXPackSQL_MacroExpansion(t *testing.T) {
	// Register a simple $__timeFilter macro for testing, mimicking the
	// pattern used by fc-datasource-kit's ReplaceMacros.
	origMacro := macros.Macro
	macros.RegisterMacro(func(sql string, start, end int64) (string, error) {
		if strings.Contains(sql, "$__timeFilter") {
			// Simple replacement: $__timeFilter("col") to ("col" >= start AND "col" < end)
			sql = strings.Replace(sql, `$__timeFilter("@timestamp")`,
				fmt.Sprintf(`("@timestamp" >= %d AND "@timestamp" < %d)`, start, end), 1)
		}
		return sql, nil
	})
	defer func() { macros.Macro = origMacro }()

	sqlResp := `{"columns":[{"name":"cnt","type":"long"}],"rows":[[100]]}`
	srv := mockESServer(t, "8.15.0", sqlResp, true)
	defer srv.Close()

	clientCache = syncMapNew()
	escli := newTestElasticsearch("8.15.0", []string{srv.URL})

	resp, err := XPackSQL(context.Background(), escli, XPackSQLRequest{
		Query: `SELECT COUNT(*) AS cnt FROM "logs" WHERE $__timeFilter("@timestamp")`,
		From:  1700000000,
		To:    1700003600,
	})
	if err != nil {
		t.Fatalf("XPackSQL() with macro error: %v", err)
	}
	if len(resp.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(resp.Rows))
	}
}

func TestParseSQLLimit(t *testing.T) {
	tests := []struct {
		sql  string
		want int
	}{
		{`SELECT * FROM "t" LIMIT 100`, 100},
		{`SELECT * FROM "t" LIMIT 100000`, 100000},
		{`SELECT * FROM "t" limit 500`, 500},
		{`SELECT * FROM "t"`, 0},
		{`SELECT * FROM "t" LIMIT abc`, 0},
		{"SELECT * FROM \"t\"\nLIMIT\n  900", 900},
		{`SELECT * FROM "t" WHERE x LIKE '%LIMIT%' LIMIT 50`, 50},
		{`SELECT * FROM "t" WHERE msg = 'LIMIT 999'`, 0},
		{`SELECT * FROM "t" WHERE msg = 'LIMIT 5' LIMIT 200`, 200},
		{`SELECT * FROM "t" WHERE msg = 'has ''LIMIT 5'' inside'`, 0},
		{`SELECT * FROM "limit 5"`, 0},
		{`SELECT * FROM t -- LIMIT 5`, 0},
		{`SELECT * FROM t /* LIMIT 5 */ LIMIT 300`, 300},
		{`SELECT * FROM t -- LIMIT 5` + "\n" + `LIMIT 100`, 100},
	}
	for _, tt := range tests {
		got := parseSQLLimit(tt.sql)
		if got != tt.want {
			t.Errorf("parseSQLLimit(%q) = %d, want %d", tt.sql, got, tt.want)
		}
	}
}

// TestXPackSQL_CursorFollowed verifies that XPackSQL follows the cursor
// to fetch all pages. It also asserts the wire protocol: first request has
// query+fetch_size, subsequent requests have cursor+fetch_size only.
func TestXPackSQL_CursorFollowed(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"version": map[string]interface{}{"number": "8.15.0"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/_sql":
			callCount++
			body, _ := io.ReadAll(r.Body)
			var reqBody map[string]interface{}
			json.Unmarshal(body, &reqBody)

			if callCount == 1 {
				if _, ok := reqBody["query"]; !ok {
					t.Errorf("first request should have 'query', got: %s", body)
				}
				if _, ok := reqBody["fetch_size"]; !ok {
					t.Errorf("first request should have 'fetch_size', got: %s", body)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"columns":[{"name":"id","type":"long"}],
					"rows":[[1],[2]],
					"cursor":"page2"
				}`))
			} else {
				if _, ok := reqBody["query"]; ok {
					t.Errorf("cursor request should NOT have 'query', got: %s", body)
				}
				if _, ok := reqBody["cursor"]; !ok {
					t.Errorf("cursor request should have 'cursor', got: %s", body)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"rows":[[3],[4]]
				}`))
			}
		}
	}))
	defer srv.Close()

	clientCache = syncMapNew()
	escli := newTestElasticsearch("8.15.0", []string{srv.URL})

	resp, err := XPackSQL(context.Background(), escli, XPackSQLRequest{
		Query: `SELECT id FROM "test" LIMIT 10000`,
	})
	if err != nil {
		t.Fatalf("XPackSQL() error: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 /_sql calls (initial + cursor), got %d", callCount)
	}
	if len(resp.Rows) != 4 {
		t.Errorf("expected 4 rows (all pages), got %d", len(resp.Rows))
	}
	if len(resp.Columns) != 1 || resp.Columns[0].Name != "id" {
		t.Errorf("expected columns preserved from first page, got %+v", resp.Columns)
	}
	if resp.Cursor != "" {
		t.Errorf("expected cursor to be cleared, got %q", resp.Cursor)
	}
}

// TestXPackSQL_CursorTruncatedByLimit verifies rows are truncated to LIMIT
// and that a clear cursor request is sent for the leftover cursor.
func TestXPackSQL_CursorTruncatedByLimit(t *testing.T) {
	callCount := 0
	clearCursorCalled := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"version": map[string]interface{}{"number": "8.15.0"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/_sql/close":
			body, _ := io.ReadAll(r.Body)
			var reqBody map[string]string
			json.Unmarshal(body, &reqBody)
			clearCursorCalled <- reqBody["cursor"]
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"succeeded":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/_sql":
			callCount++
			w.Header().Set("Content-Type", "application/json")
			if callCount == 1 {
				w.Write([]byte(`{
					"columns":[{"name":"id","type":"long"}],
					"rows":[[1],[2],[3]],
					"cursor":"leftover_cursor"
				}`))
			} else {
				w.Write([]byte(`{
					"columns":[{"name":"id","type":"long"}],
					"rows":[[4],[5],[6]],
					"cursor":"still_more"
				}`))
			}
		}
	}))
	defer srv.Close()

	clientCache = syncMapNew()
	escli := newTestElasticsearch("8.15.0", []string{srv.URL})

	resp, err := XPackSQL(context.Background(), escli, XPackSQLRequest{
		Query: `SELECT id FROM "test" LIMIT 5`,
	})
	if err != nil {
		t.Fatalf("XPackSQL() error: %v", err)
	}

	if len(resp.Rows) != 5 {
		t.Errorf("expected 5 rows (truncated to LIMIT), got %d", len(resp.Rows))
	}
	if resp.Cursor != "" {
		t.Errorf("expected cursor cleared in response, got %q", resp.Cursor)
	}

	select {
	case cursor := <-clearCursorCalled:
		if cursor != "still_more" {
			t.Errorf("expected clear cursor for 'still_more', got %q", cursor)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("clear cursor request not received within 2s — cursor cleanup may be broken")
	}
}

// TestXPackSQL_NoCursorWithoutLimit verifies that without LIMIT,
// cursor is not followed (preserves existing behavior for unbounded queries).
func TestXPackSQL_NoCursorWithoutLimit(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"version": map[string]interface{}{"number": "8.15.0"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/_sql":
			callCount++
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"columns":[{"name":"id","type":"long"}],
				"rows":[[1],[2]],
				"cursor":"ignored"
			}`))
		}
	}))
	defer srv.Close()

	clientCache = syncMapNew()
	escli := newTestElasticsearch("8.15.0", []string{srv.URL})

	resp, err := XPackSQL(context.Background(), escli, XPackSQLRequest{
		Query: `SELECT id FROM "test"`,
	})
	if err != nil {
		t.Fatalf("XPackSQL() error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 call (no cursor follow without LIMIT), got %d", callCount)
	}
	if len(resp.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(resp.Rows))
	}
}

func TestXPackSQL_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch {
		case r.URL.Path == "/":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"version": map[string]interface{}{"number": "8.15.0"},
			})
		case r.URL.Path == "/_sql":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"type":"parsing_exception","reason":"line 1:1: Unknown command [SELEC]"}}`))
		}
	}))
	defer srv.Close()

	clientCache = syncMapNew()
	escli := newTestElasticsearch("8.15.0", []string{srv.URL})

	_, err := XPackSQL(context.Background(), escli, XPackSQLRequest{
		Query: `SELEC * FROM "test"`,
	})
	if err == nil {
		t.Fatal("expected error for bad SQL, got nil")
	}
}
