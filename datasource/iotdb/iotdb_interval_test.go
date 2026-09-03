package iotdb

import (
	"strconv"
	"testing"
	"time"
)

// When the alert engine provides only an interval (no from/to), applyIntervalWindow
// should derive a [now-interval, now] window so the query is time-scoped.
func TestApplyIntervalWindowDerivesRange(t *testing.T) {
	qp := &QueryParam{Interval: 300}
	before := time.Now().Unix()
	applyIntervalWindow(qp)
	after := time.Now().Unix()

	to, ok := qp.To.(int64)
	if !ok {
		t.Fatalf("To not set to int64, got %T", qp.To)
	}
	from, ok := qp.From.(int64)
	if !ok {
		t.Fatalf("From not set to int64, got %T", qp.From)
	}
	if to < before || to > after {
		t.Fatalf("To=%d not within [%d,%d]", to, before, after)
	}
	if to-from != 300 {
		t.Fatalf("window=%d seconds, want 300", to-from)
	}

	// And the derived window must turn into a bounded time filter on the SQL.
	got, err := appendTimeFilter("select time, temperature from sensor_data", qp)
	if err != nil {
		t.Fatalf("append time filter failed: %v", err)
	}
	fromMs := from * 1000
	toMs := to * 1000
	want := "select time, temperature from sensor_data WHERE time >= " +
		strconv.FormatInt(fromMs, 10) + " AND time <= " + strconv.FormatInt(toMs, 10)
	if got != want {
		t.Fatalf("unexpected sql:\nwant: %s\ngot:  %s", want, got)
	}
}

// An explicit from/to (explorer/dashboard path) must NOT be overridden by interval.
func TestApplyIntervalWindowKeepsExplicitRange(t *testing.T) {
	qp := &QueryParam{Interval: 300, From: "2026-01-01T00:00:00Z", To: "2026-01-01T01:00:00Z"}
	applyIntervalWindow(qp)
	if qp.From != "2026-01-01T00:00:00Z" || qp.To != "2026-01-01T01:00:00Z" {
		t.Fatalf("explicit range overridden: from=%v to=%v", qp.From, qp.To)
	}
}

// Blank from + missing interval falls back to a bounded 60s window (matching
// tdengine/doris) instead of degrading into a whole-table scan.
func TestApplyIntervalWindowDefaultsWhenMissing(t *testing.T) {
	qp := &QueryParam{}
	applyIntervalWindow(qp)

	from, ok := qp.From.(int64)
	if !ok {
		t.Fatalf("From not set to int64, got %T", qp.From)
	}
	to, ok := qp.To.(int64)
	if !ok {
		t.Fatalf("To not set to int64, got %T", qp.To)
	}
	if to-from != 60 {
		t.Fatalf("window=%d seconds, want 60", to-from)
	}
}
