package iotdb

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	iot "github.com/ccfos/nightingale/v6/dskit/iotdb"
	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"
)

func TestInitRPCSettings(t *testing.T) {
	plug, err := (&IoTDB{}).Init(map[string]interface{}{
		"iotdb.addr":         "http://127.0.0.1:18080",
		"iotdb.rpc_addr":     "127.0.0.1:6667",
		"iotdb.database":     "telemetry",
		"iotdb.dial_timeout": 1200,
		"iotdb.timeout":      3400,
		"iotdb.basic":        iot.IotdbBasicAuth{User: "root", Password: "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := plug.(*IoTDB)
	if got.RPCAddr != "127.0.0.1:6667" || got.Database != "telemetry" || got.DialTimeout != 1200 || got.Timeout != 3400 {
		t.Fatalf("decoded settings: rpc=%q database=%q dial_timeout=%d timeout=%d", got.RPCAddr, got.Database, got.DialTimeout, got.Timeout)
	}
	if got.Basic == nil || got.Basic.Password != "secret" {
		t.Fatalf("basic auth was not decoded")
	}
}

func TestQueryDataRequiresDatabase(t *testing.T) {
	it := &IoTDB{}
	_, err := it.QueryData(context.Background(), map[string]interface{}{
		"sql":  "select time, value from table1",
		"keys": map[string]interface{}{"valueKey": "value", "timeKey": "time"},
	})
	if err == nil || !strings.Contains(err.Error(), "database is required") {
		t.Fatalf("expected database validation error, got %v", err)
	}
}

func TestQueryDataReadOnlyRejectsWrites(t *testing.T) {
	ctx := types.WithCallContext(context.Background(), types.CallContext{EnforceReadOnly: true})
	it := &IoTDB{}
	_, err := it.QueryData(ctx, map[string]interface{}{
		"database": "metrics",
		"sql":      "DELETE FROM table1",
		"keys":     map[string]interface{}{"valueKey": "value", "timeKey": "time"},
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "read-only") {
		t.Fatalf("expected read-only validation error, got %v", err)
	}
}

func TestResponseToRowsPreservesTableValues(t *testing.T) {
	ts := time.UnixMilli(1778493600000).UTC()
	rows := responseToRows(iot.APIResponse{
		ColumnNames: []string{"time", "instance", "value"},
		Values:      [][]interface{}{{ts, "node-a", float64(1.5)}, {ts.Add(time.Minute), "node-b", nil}},
	})
	if len(rows) != 2 || rows[0]["instance"] != "node-a" || rows[0]["value"] != float64(1.5) || rows[1]["value"] != nil {
		t.Fatalf("unexpected rows: %#v", rows)
	}
}

func TestResponseToRowsSupportsColumnarValues(t *testing.T) {
	rows := responseToRows(iot.APIResponse{
		ColumnNames: []string{"time", "instance", "value"},
		Values: [][]interface{}{
			{time.UnixMilli(1778493600000).UTC(), time.UnixMilli(1778493660000).UTC()},
			{"node-a", "node-b"},
			{float64(1), float64(2)},
		},
	})
	if len(rows) != 2 || rows[0]["instance"] != "node-a" || rows[1]["value"] != float64(2) {
		t.Fatalf("unexpected columnar rows: %#v", rows)
	}
}

func TestResponseToRowsCanonicalizesTimeColumnCase(t *testing.T) {
	rows := responseToRows(iot.APIResponse{
		ColumnNames: []string{"Time", "value"},
		Values:      [][]interface{}{{time.Unix(1778493600, 0).UTC(), float64(1)}},
	})
	if len(rows) != 1 || rows[0]["time"] == nil || rows[0]["Time"] != nil {
		t.Fatalf("expected canonical time column, got %#v", rows)
	}
}

func TestResponseToRowsSanitizesNonFiniteValues(t *testing.T) {
	rows := responseToRows(iot.APIResponse{
		ColumnNames: []string{"time", "value"},
		Values:      [][]interface{}{{time.Unix(1778493600, 0).UTC(), math.NaN()}, {time.Unix(1778493660, 0).UTC(), math.Inf(1)}},
	})
	if len(rows) != 2 || rows[0]["value"] != nil || rows[1]["value"] != nil {
		t.Fatalf("expected non-finite values to become nil, got %#v", rows)
	}
	if _, err := json.Marshal(rows); err != nil {
		t.Fatalf("sanitized rows must be JSON encodable: %v", err)
	}
}

func TestEffectiveTimeKeyUsesCanonicalColumn(t *testing.T) {
	rows := responseToRows(iot.APIResponse{
		ColumnNames: []string{"TIME", "value"},
		Values:      [][]interface{}{{time.Unix(1778493600, 0).UTC(), float64(1)}},
	})
	if got := effectiveTimeKey(rows, "TIME"); got != "time" {
		t.Fatalf("effective time key=%q, want time", got)
	}
}

func TestFormatIoTDBRowsCreatesMultiSeriesAndSkipsInvalidValues(t *testing.T) {
	rows := []map[string]interface{}{
		{"time": int64(1778493600000), "instance": "node-a", "value": float64(1)},
		{"time": int64(1778493660000), "instance": "node-a", "value": math.NaN()},
		{"time": int64(1778493720000), "instance": "node-b", "value": math.Inf(1)},
		{"time": int64(1778493780000), "instance": "node-b", "value": float64(2)},
		{"time": int64(1778493840000), "instance": nil, "value": float64(3)},
	}
	items := sqlbase.FormatMetricValues(types.Keys{ValueKey: "value", LabelKey: "instance", TimeKey: "time"}, rows)
	if len(items) != 3 {
		t.Fatalf("series=%d, want 3 (node-a, node-b, and unlabeled)", len(items))
	}
	for _, item := range items {
		if len(item.Values) != 1 {
			t.Fatalf("metric %s has %d values, want one finite sample: %#v", item.Metric, len(item.Values), item.Values)
		}
		if math.IsNaN(item.Values[0][1]) || math.IsInf(item.Values[0][1], 0) {
			t.Fatalf("non-finite sample leaked into %s: %#v", item.Metric, item.Values)
		}
	}
}

func TestExpandIoTDBMacros(t *testing.T) {
	sqlText := "SELECT time, value FROM table1 WHERE time >= $__timeFrom() AND time < $__timeTo AND $__timeFilter(time)"
	got, err := expandIoTDBMacros(sqlText, 1778493600, 1778497200)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT time, value FROM table1 WHERE time >= 1778493600000 AND time < 1778497200000 AND (time >= 1778493600000 AND time < 1778497200000)"
	if got != want {
		t.Fatalf("expanded SQL=%q, want %q", got, want)
	}
}

func TestExpandIoTDBMacrosRejectsMissingRange(t *testing.T) {
	_, err := expandIoTDBMacros("SELECT $__timeFrom()", 0, 0)
	if err == nil || !strings.Contains(err.Error(), "requires a query time range") {
		t.Fatalf("expected missing range error, got %v", err)
	}
}

func TestAppendTimeFilterNoWhere(t *testing.T) {
	got, err := appendTimeFilter("select time, temperature, device_id from sensor_data", queryParamWithRange(""))
	if err != nil {
		t.Fatalf("append time filter failed: %v", err)
	}

	want := "select time, temperature, device_id from sensor_data WHERE time >= 1778493600000 AND time <= 1778497200000"
	if got != want {
		t.Fatalf("unexpected sql:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestAppendTimeFilterWithWhere(t *testing.T) {
	got, err := appendTimeFilter("select * from sensor_data where device_id = 'd1'", queryParamWithRange(""))
	if err != nil {
		t.Fatalf("append time filter failed: %v", err)
	}

	want := "select * from sensor_data where device_id = 'd1' AND time >= 1778493600000 AND time <= 1778497200000"
	if got != want {
		t.Fatalf("unexpected sql:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestAppendTimeFilterBeforeOrderBy(t *testing.T) {
	got, err := appendTimeFilter("select * from sensor_data order by time desc", queryParamWithRange(""))
	if err != nil {
		t.Fatalf("append time filter failed: %v", err)
	}

	want := "select * from sensor_data WHERE time >= 1778493600000 AND time <= 1778497200000 order by time desc"
	if got != want {
		t.Fatalf("unexpected sql:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestAppendTimeFilterBeforeLimit(t *testing.T) {
	got, err := appendTimeFilter("select * from sensor_data limit 100", queryParamWithRange(""))
	if err != nil {
		t.Fatalf("append time filter failed: %v", err)
	}

	want := "select * from sensor_data WHERE time >= 1778493600000 AND time <= 1778497200000 limit 100"
	if got != want {
		t.Fatalf("unexpected sql:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestAppendTimeFilterBeforeOrderByAndLimit(t *testing.T) {
	got, err := appendTimeFilter("select * from sensor_data where device_id = 'd1' order by time desc limit 10", queryParamWithRange(""))
	if err != nil {
		t.Fatalf("append time filter failed: %v", err)
	}

	want := "select * from sensor_data where device_id = 'd1' AND time >= 1778493600000 AND time <= 1778497200000 order by time desc limit 10"
	if got != want {
		t.Fatalf("unexpected sql:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestAppendTimeFilterSkipExistingTimeFilter(t *testing.T) {
	original := "select * from sensor_data where time >= 1778493600000 and time <= 1778497200000"
	got, err := appendTimeFilter(original, queryParamWithRange(""))
	if err != nil {
		t.Fatalf("append time filter failed: %v", err)
	}

	if got != original {
		t.Fatalf("time filter should stay unchanged:\nwant: %s\ngot:  %s", original, got)
	}
}

func TestAppendTimeFilterSkipExistingQualifiedTimeFilter(t *testing.T) {
	original := "select * from sensor_data s where s.time >= 1778493600000"
	got, err := appendTimeFilter(original, queryParamWithRange(""))
	if err != nil {
		t.Fatalf("append time filter failed: %v", err)
	}

	if got != original {
		t.Fatalf("time filter should stay unchanged:\nwant: %s\ngot:  %s", original, got)
	}
}

func TestAppendTimeFilterDefaultTimeKey(t *testing.T) {
	got, err := appendTimeFilter("select * from sensor_data", queryParamWithRange(""))
	if err != nil {
		t.Fatalf("append time filter failed: %v", err)
	}

	want := "select * from sensor_data WHERE time >= 1778493600000 AND time <= 1778497200000"
	if got != want {
		t.Fatalf("unexpected sql:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestAppendTimeFilterCustomTimeKey(t *testing.T) {
	got, err := appendTimeFilter("select event_time, temperature from sensor_data", queryParamWithRange("event_time"))
	if err != nil {
		t.Fatalf("append time filter failed: %v", err)
	}

	want := "select event_time, temperature from sensor_data WHERE event_time >= 1778493600000 AND event_time <= 1778497200000"
	if got != want {
		t.Fatalf("unexpected sql:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestAppendTimeFilterRFC3339String(t *testing.T) {
	got, err := appendTimeFilter("select * from sensor_data", &QueryParam{
		From: "2026-05-11T10:00:00.000Z",
		To:   "2026-05-11T11:00:00.000Z",
	})
	if err != nil {
		t.Fatalf("append time filter failed: %v", err)
	}

	want := "select * from sensor_data WHERE time >= 1778493600000 AND time <= 1778497200000"
	if got != want {
		t.Fatalf("unexpected sql:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestAppendTimeFilterUnixTimestampMillis(t *testing.T) {
	got, err := appendTimeFilter("select * from sensor_data", &QueryParam{
		From: int64(1778493600000),
		To:   int64(1778497200000),
	})
	if err != nil {
		t.Fatalf("append time filter failed: %v", err)
	}

	want := "select * from sensor_data WHERE time >= 1778493600000 AND time <= 1778497200000"
	if got != want {
		t.Fatalf("unexpected sql:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestAppendTimeFilterSkipWhenNoRange(t *testing.T) {
	original := "select * from sensor_data"
	got, err := appendTimeFilter(original, &QueryParam{})
	if err != nil {
		t.Fatalf("append time filter failed: %v", err)
	}

	if got != original {
		t.Fatalf("sql should stay unchanged:\nwant: %s\ngot:  %s", original, got)
	}
}

func queryParamWithRange(timeKey string) *QueryParam {
	return &QueryParam{
		From: "2026-05-11T10:00:00.000Z",
		To:   "2026-05-11T11:00:00.000Z",
		Keys: datasource.Keys{TimeKey: timeKey},
	}
}
