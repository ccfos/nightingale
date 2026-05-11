package es

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
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

func TestXPackSQL_MacroExpansion(t *testing.T) {
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
