package victorialogs

import (
	"context"
	"testing"
	"time"
)

var v = VictoriaLogs{
	VictorialogsAddr: "http://127.0.0.1:9428",
	Headers:          make(map[string]string),
	Timeout:          10000, // 10 seconds in milliseconds
}

func TestVictoriaLogs_InitHTTPClient(t *testing.T) {
	if err := v.InitHTTPClient(); err != nil {
		t.Fatalf("InitHTTPClient failed: %v", err)
	}
	if v.HTTPClient == nil {
		t.Fatal("HTTPClient should not be nil after initialization")
	}
}

func TestVictoriaLogs_Query(t *testing.T) {
	ctx := context.Background()
	if err := v.InitHTTPClient(); err != nil {
		t.Fatalf("InitHTTPClient failed: %v", err)
	}

	// Query logs with basic query
	now := time.Now().UnixNano()
	start := now - int64(time.Hour) // 1 hour ago
	end := now

	logs, err := v.Query(ctx, "*", start, end, 10)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	t.Logf("Query returned %d log entries", len(logs))
	for i, log := range logs {
		t.Logf("Log[%d]: %v", i, log)
	}
}

func TestVictoriaLogs_StatsQuery(t *testing.T) {
	ctx := context.Background()
	if err := v.InitHTTPClient(); err != nil {
		t.Fatalf("InitHTTPClient failed: %v", err)
	}

	// Stats query with count
	now := time.Now().UnixNano()
	result, err := v.StatsQuery(ctx, "* | stats count() as total", now)
	if err != nil {
		t.Fatalf("StatsQuery failed: %v", err)
	}
	t.Logf("StatsQuery result: status=%s, resultType=%s", result.Status, result.Data.ResultType)
	for i, item := range result.Data.Result {
		t.Logf("Result[%d]: metric=%v, value=%v", i, item.Metric, item.Value)
	}
}

func TestVictoriaLogs_StatsQueryRange(t *testing.T) {
	ctx := context.Background()
	if err := v.InitHTTPClient(); err != nil {
		t.Fatalf("InitHTTPClient failed: %v", err)
	}

	// Stats query range
	now := time.Now().UnixNano()
	start := now - int64(time.Hour) // 1 hour ago
	end := now

	result, err := v.StatsQueryRange(ctx, "* | stats count() as total", start, end, "5m")
	if err != nil {
		t.Fatalf("StatsQueryRange failed: %v", err)
	}
	t.Logf("StatsQueryRange result: status=%s, resultType=%s", result.Status, result.Data.ResultType)
	for i, item := range result.Data.Result {
		t.Logf("Result[%d]: metric=%v, values count=%d", i, item.Metric, len(item.Values))
	}
}

func TestVictoriaLogs_HitsLogs(t *testing.T) {
	ctx := context.Background()
	if err := v.InitHTTPClient(); err != nil {
		t.Fatalf("InitHTTPClient failed: %v", err)
	}

	// Get total hits count
	now := time.Now().UnixNano()
	start := now - int64(time.Hour) // 1 hour ago
	end := now

	count, err := v.HitsLogs(ctx, "*", start, end)
	if err != nil {
		t.Fatalf("HitsLogs failed: %v", err)
	}
	t.Logf("HitsLogs total count: %d", count)
}

func TestVictoriaLogs_QueryWithFilter(t *testing.T) {
	ctx := context.Background()
	if err := v.InitHTTPClient(); err != nil {
		t.Fatalf("InitHTTPClient failed: %v", err)
	}

	// Query with a filter condition
	now := time.Now().UnixNano()
	start := now - int64(time.Hour)
	end := now

	logs, err := v.Query(ctx, "_stream:{app=\"test\"}", start, end, 5)
	if err != nil {
		t.Fatalf("Query with filter failed: %v", err)
	}
	t.Logf("Query with filter returned %d log entries", len(logs))
}

func TestVictoriaLogs_StatsQueryByField(t *testing.T) {
	ctx := context.Background()
	if err := v.InitHTTPClient(); err != nil {
		t.Fatalf("InitHTTPClient failed: %v", err)
	}

	// Stats query grouped by field
	now := time.Now().UnixNano()
	result, err := v.StatsQuery(ctx, "* | stats by (level) count() as cnt", now)
	if err != nil {
		t.Fatalf("StatsQuery by field failed: %v", err)
	}
	t.Logf("StatsQuery by field result: status=%s", result.Status)
	for i, item := range result.Data.Result {
		t.Logf("Result[%d]: metric=%v, value=%v", i, item.Metric, item.Value)
	}
}
