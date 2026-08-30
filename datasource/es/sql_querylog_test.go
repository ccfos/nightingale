package es

import (
	"testing"
)

// IsSQLQueryLog and extractSQLRequest must answer the SQL-branch question
// identically: they share decodeSQLQueryParam, and the batch query v2 router
// relies on IsSQLQueryLog to know the shape of QueryLog's return value (row
// maps vs SearchHit objects). If the two ever disagreed — e.g. a payload with
// a non-empty sql but a wrong-typed index fails the mapstructure decode and
// makes QueryLog fall back to the DSL path — the router would skip the
// _source unwrapping while the plugin returns SearchHits, leaking _id/_index
// into the records.
func TestIsSQLQueryLogMatchesExtractSQLRequest(t *testing.T) {
	cases := []struct {
		name       string
		queryParam interface{}
		want       bool
	}{
		{"sql_only", map[string]interface{}{"sql": `SELECT 1`}, true},
		{"sql_with_range", map[string]interface{}{"sql": `SELECT 1`, "start": int64(1), "end": int64(2)}, true},
		{"empty_sql", map[string]interface{}{"sql": "", "index": "idx*"}, false},
		{"missing_sql", map[string]interface{}{"index": "idx*"}, false},
		{"nil_payload", nil, false},
		{"wrong_typed_sql", map[string]interface{}{"sql": 123, "index": "idx*"}, false},
		// Divergence hazard: sql decodes fine, index does not. The decode
		// failure must send BOTH the plugin and the router to the DSL path.
		{"wrong_typed_index", map[string]interface{}{"sql": `SELECT 1`, "index": 123}, false},
		{"wrong_typed_start", map[string]interface{}{"sql": `SELECT 1`, "start": "yesterday"}, false},
		{"sql_log_shape", map[string]interface{}{
			"sql": `SELECT COUNT("status") AS cnt FROM "idx*"`, "index": "idx*",
			"start": int64(1784971399), "end": int64(1784974999),
		}, true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSQLQueryLog(tt.queryParam)
			if got != tt.want {
				t.Fatalf("IsSQLQueryLog(%#v) = %v, want %v", tt.queryParam, got, tt.want)
			}

			_, routed := extractSQLRequest(tt.queryParam)
			if routed != got {
				t.Fatalf("extractSQLRequest(%#v) routes SQL = %v, IsSQLQueryLog = %v", tt.queryParam, routed, got)
			}
		})
	}

	req, ok := extractSQLRequest(map[string]interface{}{
		"sql": `SELECT COUNT("status") AS cnt FROM "idx*"`,
		"start": int64(1784971399), "end": int64(1784974999),
	})
	if !ok {
		t.Fatal("extractSQLRequest() = false for a valid SQL payload")
	}
	if req.Query != `SELECT COUNT("status") AS cnt FROM "idx*"` {
		t.Errorf("Query = %q", req.Query)
	}
	if req.From != 1784971399 || req.To != 1784974999 {
		t.Errorf("range = [%d, %d], want [1784971399, 1784974999]", req.From, req.To)
	}
	if !req.FieldMultiValueLeniency {
		t.Error("FieldMultiValueLeniency = false, want true")
	}
}
