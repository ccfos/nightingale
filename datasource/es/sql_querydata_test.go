package es

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractTSRequest(t *testing.T) {
	const sql = "SELECT COUNT(*) AS cnt, HISTOGRAM(\"@timestamp\", INTERVAL 1 MINUTE) AS t FROM \"logs\" GROUP BY t"

	tests := []struct {
		name    string
		input   interface{}
		wantErr string // substring; empty means no error
		wantDSL bool   // routed to the DSL path
		wantRef string
	}{
		{
			name: "valid timeseries request",
			input: map[string]interface{}{
				"ref":  "A",
				"sql":  sql,
				"keys": map[string]interface{}{"valueKey": "cnt", "timeKey": "t"},
				"from": int64(1700000000),
				"to":   int64(1700003600),
			},
			wantRef: "A",
		},
		{
			name:    "DSL request has no sql",
			input:   map[string]interface{}{"index": "logs-*", "date_field": "@timestamp"},
			wantDSL: true,
		},
		{
			name:    "blank sql is a DSL request",
			input:   map[string]interface{}{"index": "logs-*", "sql": "   "},
			wantDSL: true,
		},
		{
			name:    "nil input",
			input:   nil,
			wantDSL: true,
		},
		{
			name: "missing valueKey is reported, not routed to DSL",
			input: map[string]interface{}{
				"sql":  sql,
				"keys": map[string]interface{}{"valueKey": "  "},
			},
			wantErr: "keys.valueKey",
		},
		{
			name: "unreadable time range is reported, not routed to DSL",
			input: map[string]interface{}{
				"sql":  sql,
				"keys": map[string]interface{}{"valueKey": "cnt"},
				"from": "2026-09-16T00:00:00Z",
			},
			wantErr: "invalid ES SQL timeseries query",
		},
		{
			name:    "non-string sql is reported",
			input:   map[string]interface{}{"sql": 123},
			wantErr: "invalid ES query",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractTSRequest(tc.input)

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			if tc.wantDSL {
				assert.Nil(t, got)
				return
			}

			require.NotNil(t, got)
			assert.Equal(t, sql, got.SQL)
			assert.Equal(t, tc.wantRef, got.Ref)
		})
	}
}

func TestQueryDataViaSQL(t *testing.T) {
	// Mock ES SQL endpoint
	sqlResp := map[string]interface{}{
		"columns": []map[string]interface{}{
			{"name": "t", "type": "datetime"},
			{"name": "cnt", "type": "long"},
			{"name": "service", "type": "keyword"},
		},
		"rows": [][]interface{}{
			{"2024-01-01T00:00:00.000Z", 10, "web"},
			{"2024-01-01T00:01:00.000Z", 15, "web"},
			{"2024-01-01T00:00:00.000Z", 5, "api"},
			{"2024-01-01T00:01:00.000Z", 8, "api"},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sqlResp)
	}))
	defer ts.Close()

	escli := &Elasticsearch{
		Nodes:   []string{ts.URL},
		Version: "8.12.0",
	}

	p := &tsQueryParam{
		Ref: "A",
		SQL: "SELECT HISTOGRAM(\"@timestamp\", INTERVAL 1 MINUTE) AS t, COUNT(*) AS cnt, service FROM logs GROUP BY t, service",
		Keys: datasource.Keys{
			ValueKey: "cnt",
			LabelKey: "service",
			TimeKey:  "t",
		},
		From: 1704067200,
		To:   1704070800,
	}

	data, err := escli.queryDataViaSQL(context.Background(), p)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// All results should carry the ref
	for _, d := range data {
		assert.Equal(t, "A", d.Ref)
	}

	// Should have 2 series (web + api)
	assert.Len(t, data, 2)

	// Each series should have 2 data points
	for _, d := range data {
		assert.Len(t, d.Values, 2)
	}
}

func TestQueryDataViaSQL_EmptyResult(t *testing.T) {
	sqlResp := map[string]interface{}{
		"columns": []map[string]interface{}{
			{"name": "t", "type": "datetime"},
			{"name": "cnt", "type": "long"},
		},
		"rows": [][]interface{}{},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sqlResp)
	}))
	defer ts.Close()

	escli := &Elasticsearch{
		Nodes:   []string{ts.URL},
		Version: "8.12.0",
	}

	p := &tsQueryParam{
		Ref: "B",
		SQL: "SELECT COUNT(*) AS cnt FROM empty_index",
		Keys: datasource.Keys{
			ValueKey: "cnt",
			TimeKey:  "t",
		},
		From: 1704067200,
		To:   1704070800,
	}

	data, err := escli.queryDataViaSQL(context.Background(), p)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestQueryDataViaSQL_ESError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"reason":"parse error"},"status":400}`))
	}))
	defer ts.Close()

	escli := &Elasticsearch{
		Nodes:   []string{ts.URL},
		Version: "8.12.0",
	}

	p := &tsQueryParam{
		SQL: "INVALID SQL",
		Keys: datasource.Keys{
			ValueKey: "cnt",
		},
	}

	_, err := escli.queryDataViaSQL(context.Background(), p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ES SQL query failed")
}
