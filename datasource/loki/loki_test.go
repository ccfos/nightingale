package loki

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	lokikit "github.com/ccfos/nightingale/v6/dskit/loki"
)

func TestQueryLogReturnsTotalFromCountQuery(t *testing.T) {
	var sawRangeQuery bool
	var sawCountQuery bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/query_range":
			sawRangeQuery = true
			if got := r.URL.Query().Get("direction"); got != "forward" {
				t.Fatalf("unexpected default direction: %q", got)
			}
			if got := r.URL.Query().Get("limit"); got != "1" {
				t.Fatalf("unexpected range limit: %q", got)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[{"stream":{"app":"api"},"values":[["1710000060000000000","line"]]}]}}`))
		case "/api/v1/query":
			sawCountQuery = true
			query := r.URL.Query().Get("query")
			if !strings.Contains(query, `sum(count_over_time({app="api"} [60s]))`) {
				t.Fatalf("unexpected count query: %q", query)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1710000060,"42"]}]}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	vl := &Loki{
		Loki: lokikit.Loki{
			LokiAddr: server.URL,
		},
	}
	logs, total, err := vl.QueryLog(context.Background(), Query{
		Query: `{app="api"}`,
		Start: 1710000000,
		End:   1710000060,
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("QueryLog failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("unexpected logs length: %d", len(logs))
	}
	if total != 42 {
		t.Fatalf("unexpected total: %d", total)
	}
	log, ok := logs[0].(lokikit.NormalizedLog)
	if !ok {
		t.Fatalf("unexpected log type: %#v", logs[0])
	}
	labels, ok := log["labels"].(map[string]string)
	if !ok || labels["app"] != "api" {
		t.Fatalf("unexpected labels: %#v", log["labels"])
	}
	if _, exists := log["stream"]; exists {
		t.Fatalf("stream should not be returned")
	}
	if !sawRangeQuery || !sawCountQuery {
		t.Fatalf("expected range and count queries, sawRange=%v sawCount=%v", sawRangeQuery, sawCountQuery)
	}
}

func TestQueryLogUsesReverseForDirection(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		requestCount++
		wantDirection := "forward"
		if requestCount == 2 {
			wantDirection = "backward"
		}
		if got := r.URL.Query().Get("direction"); got != wantDirection {
			t.Fatalf("unexpected Loki direction: got %q want %q", got, wantDirection)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[{"stream":{"app":"api"},"values":[["1710000060000000000","new"],["1710000000000000000","old"]]}]}}`))
	}))
	defer server.Close()

	vl := &Loki{
		Loki: lokikit.Loki{
			LokiAddr: server.URL,
		},
	}
	logs, total, err := vl.QueryLog(context.Background(), map[string]interface{}{
		"query":      `{app="api"}`,
		"start":      1710000000,
		"end":        1710000060,
		"limit":      2,
		"reverse":    false,
		"skip_count": true,
	})
	if err != nil {
		t.Fatalf("QueryLog failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("unexpected total: %d", total)
	}
	if got := logs[0].(lokikit.NormalizedLog)["line"]; got != "old" {
		t.Fatalf("reverse=false should sort logs asc, first line=%v", got)
	}

	logs, _, err = vl.QueryLog(context.Background(), map[string]interface{}{
		"query":      `{app="api"}`,
		"start":      1710000000,
		"end":        1710000060,
		"limit":      2,
		"reverse":    true,
		"skip_count": true,
	})
	if err != nil {
		t.Fatalf("QueryLog with reverse=true failed: %v", err)
	}
	if got := logs[0].(lokikit.NormalizedLog)["line"]; got != "new" {
		t.Fatalf("reverse=true should sort logs desc, first line=%v", got)
	}
}

func TestQueryLogSplitsParsedFieldsWithCachedSelectorLabelNames(t *testing.T) {
	labelNamesCache.Flush()
	var labelsQueryCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/query_range":
			if got := r.URL.Query().Get("query"); got != `{app="api"} | json` {
				t.Fatalf("unexpected range query: %q", got)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[{"stream":{"app":"api","trace_id":"abc"},"values":[["1710000060000000000","line"]]}]}}`))
		case "/api/v1/labels":
			labelsQueryCount++
			if got := r.URL.Query().Get("query"); got != `{app="api"}` {
				t.Fatalf("unexpected labels query: %q", got)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"success","data":["app"]}`))
		case "/api/v1/query":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1710000060,"1"]}]}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	vl := &Loki{
		Loki: lokikit.Loki{
			LokiAddr: server.URL,
		},
	}
	query := Query{
		Query: `{app="api"} | json`,
		Start: 1710000000,
		End:   1710000060,
		Limit: 1,
	}
	logs, _, err := vl.QueryLog(context.Background(), query)
	if err != nil {
		t.Fatalf("QueryLog failed: %v", err)
	}
	if _, _, err := vl.QueryLog(context.Background(), query); err != nil {
		t.Fatalf("second QueryLog failed: %v", err)
	}
	if labelsQueryCount != 1 {
		t.Fatalf("label names should be cached, got labelsQueryCount=%d", labelsQueryCount)
	}

	log := logs[0].(lokikit.NormalizedLog)
	labels := log["labels"].(map[string]string)
	if got := labels["app"]; got != "api" {
		t.Fatalf("unexpected app label: %q", got)
	}
	if _, exists := labels["trace_id"]; exists {
		t.Fatalf("trace_id should not be treated as label")
	}
	parsedFields := log["parsed_fields"].(map[string]string)
	if got := parsedFields["trace_id"]; got != "abc" {
		t.Fatalf("unexpected parsed trace_id: %q", got)
	}
}

func TestBuildLogCountQueryNormalizesMillisecondRange(t *testing.T) {
	got, ok := buildLogCountQuery(`{app="api"}`, 1710000000000, 1710003600000)
	if !ok {
		t.Fatalf("expected valid count query")
	}
	want := `sum(count_over_time({app="api"} [3600s]))`
	if got != want {
		t.Fatalf("unexpected query: got %q want %q", got, want)
	}
}

func TestDecodeQueryParamAcceptsStringNanosecondRange(t *testing.T) {
	param := new(Query)
	if err := decodeQueryParam(map[string]interface{}{
		"query": `{app="api"}`,
		"start": "1710000000000000000",
		"end":   "1710000001000000000",
		"limit": "30",
	}, param); err != nil {
		t.Fatalf("decodeQueryParam failed: %v", err)
	}
	if param.Start != 1710000000000000000 {
		t.Fatalf("unexpected start: %d", param.Start)
	}
	if param.End != 1710000001000000000 {
		t.Fatalf("unexpected end: %d", param.End)
	}
	if param.Limit != 30 {
		t.Fatalf("unexpected limit: %d", param.Limit)
	}
}

func TestNormalizeLogLimitDoesNotApplyHardCap(t *testing.T) {
	vl := &Loki{}
	if got := vl.normalizeLogLimit(20000); got != 20000 {
		t.Fatalf("expected explicit limit to pass through, got %d", got)
	}
	if got := vl.normalizeLogLimit(0); got != LokiDefaultLogLimit {
		t.Fatalf("unexpected default limit: %d", got)
	}

	vl.MaxQueryRows = 800
	if got := vl.normalizeLogLimit(0); got != 800 {
		t.Fatalf("unexpected datasource default limit: %d", got)
	}
}

func TestDefaultHistogramStepAcceptsMilliseconds(t *testing.T) {
	start := int64(1710000000000)
	end := int64(1710003600000)
	if got := defaultHistogramStep(start, end); got != "1m" {
		t.Fatalf("unexpected step for 1h millisecond range: %q", got)
	}
}

func TestLimitHistogramValuesUsesTopTotals(t *testing.T) {
	values := []HistogramValues{
		{
			Ref: "low",
			Values: [][]interface{}{
				{int64(1), float64(1)},
				{int64(2), "2"},
			},
		},
		{
			Ref: "high",
			Values: [][]interface{}{
				{int64(1), float64(5)},
				{int64(2), "6"},
			},
		},
		{
			Ref: "mid",
			Values: [][]interface{}{
				{int64(1), float64(4)},
			},
		},
	}

	got := limitHistogramValues(values, "app", 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 series, got %d", len(got))
	}
	if got[0].Ref != "high" || got[1].Ref != "mid" {
		t.Fatalf("unexpected top series order: %#v", got)
	}
}

func TestSortLokiLogsByDesc(t *testing.T) {
	logs := []lokikit.NormalizedLog{
		{"timestamp": int64(1000), "line": "old"},
		{"timestamp": int64(3000), "line": "new"},
		{"timestamp": int64(2000), "line": "mid"},
	}

	sortLokiLogs(logs, true)
	if got := logs[0]["line"]; got != "new" {
		t.Fatalf("desc should sort by timestamp desc, first line=%v", got)
	}
	if got := logs[2]["line"]; got != "old" {
		t.Fatalf("desc should sort by timestamp desc, last line=%v", got)
	}

	sortLokiLogs(logs, false)
	if got := logs[0]["line"]; got != "old" {
		t.Fatalf("asc should sort by timestamp asc, first line=%v", got)
	}
	if got := logs[2]["line"]; got != "new" {
		t.Fatalf("asc should sort by timestamp asc, last line=%v", got)
	}
}

func TestResolveLogTimeDesc(t *testing.T) {
	tests := []struct {
		name      string
		direction string
		reverse   bool
		want      bool
	}{
		{
			name:      "direction backward",
			direction: "backward",
			want:      true,
		},
		{
			name:      "direction forward",
			direction: "forward",
			want:      false,
		},
		{
			name:    "map reverse true",
			reverse: true,
			want:    true,
		},
		{
			name: "map reverse false",
			want: false,
		},
		{
			name: "map reverse string false",
			want: false,
		},
		{
			name:    "struct reverse true",
			reverse: true,
			want:    true,
		},
		{
			name:      "direction overrides reverse",
			direction: "forward",
			want:      false,
		},
		{
			name: "default asc",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			param := &Query{Direction: tt.direction, Reverse: tt.reverse}
			if got := resolveLogTimeDesc(param); got != tt.want {
				t.Fatalf("resolveLogTimeDesc() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubtractUnixSecondsPreservesPrecision(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want int64
	}{
		{name: "seconds", in: 1710000030, want: 1710000000},
		{name: "milliseconds", in: 1710000030000, want: 1710000000000},
		{name: "microseconds", in: 1710000030000000, want: 1710000000000000},
		{name: "nanoseconds", in: 1710000030000000000, want: 1710000000000000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := subtractUnixSeconds(tt.in, 30); got != tt.want {
				t.Fatalf("unexpected value: got %d want %d", got, tt.want)
			}
		})
	}
}
