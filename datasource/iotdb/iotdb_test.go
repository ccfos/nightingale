package iotdb

import (
	"testing"

	"github.com/ccfos/nightingale/v6/datasource"
)

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
