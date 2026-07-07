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
	if !sawRangeQuery || !sawCountQuery {
		t.Fatalf("expected both range and count queries, sawRange=%v sawCount=%v", sawRangeQuery, sawCountQuery)
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

func TestSortLokiLogsByDirection(t *testing.T) {
	logs := []lokikit.NormalizedLog{
		{"timestamp": int64(1000), "line": "old"},
		{"timestamp": int64(3000), "line": "new"},
		{"timestamp": int64(2000), "line": "mid"},
	}

	sortLokiLogs(logs, "backward")
	if got := logs[0]["line"]; got != "new" {
		t.Fatalf("backward should sort by timestamp desc, first line=%v", got)
	}
	if got := logs[2]["line"]; got != "old" {
		t.Fatalf("backward should sort by timestamp desc, last line=%v", got)
	}

	sortLokiLogs(logs, "forward")
	if got := logs[0]["line"]; got != "old" {
		t.Fatalf("forward should sort by timestamp asc, first line=%v", got)
	}
	if got := logs[2]["line"]; got != "new" {
		t.Fatalf("forward should sort by timestamp asc, last line=%v", got)
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
