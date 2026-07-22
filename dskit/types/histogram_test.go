package types

import "testing"

func TestNormalizeUnixTimestamp(t *testing.T) {
	cases := []struct {
		name  string
		value float64
		want  int64
	}{
		{name: "seconds", value: 1710000030, want: 1710000030},
		{name: "milliseconds", value: 1710000030000, want: 1710000030},
		{name: "microseconds", value: 1710000030000000, want: 1710000030},
		{name: "nanoseconds", value: 1710000030000000000, want: 1710000030},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeUnixTimestamp(c.value); got != c.want {
				t.Fatalf("unexpected value: got %d want %d", got, c.want)
			}
		})
	}
}

func TestNormalizeUnixSeconds(t *testing.T) {
	if got := NormalizeUnixSeconds(0); got != 0 {
		t.Fatalf("unexpected zero: %d", got)
	}
	if got := NormalizeUnixSeconds(1784526946385); got != 1784526946 {
		t.Fatalf("unexpected ms normalization: got %d want %d", got, 1784526946)
	}
}

func TestNormalizeUnixMillisecondsInt(t *testing.T) {
	cases := []struct {
		name  string
		value int64
		want  int64
	}{
		{name: "zero", value: 0, want: 0},
		{name: "seconds", value: 1710000030, want: 1710000030000},
		{name: "milliseconds", value: 1710000030000, want: 1710000030000},
		{name: "microseconds", value: 1710000030000000, want: 1710000030000},
		{name: "nanoseconds", value: 1710000030000000000, want: 1710000030000},
		{name: "second input", value: 1784526946, want: 1784526946000},
		{name: "millisecond input", value: 1784526946385, want: 1784526946385},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeUnixMillisecondsInt(c.value); got != c.want {
				t.Fatalf("unexpected value: got %d want %d", got, c.want)
			}
		})
	}
}

func TestUnixRangeDurationSeconds(t *testing.T) {
	cases := []struct {
		name  string
		start int64
		end   int64
		want  int64
	}{
		{name: "seconds", start: 1710000000, end: 1710003600, want: 3600},
		{name: "milliseconds", start: 1710000000000, end: 1710003600000, want: 3600},
		{name: "nanoseconds", start: 1710000000000000000, end: 1710003600000000000, want: 3600},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := UnixRangeDurationSeconds(c.start, c.end); got != c.want {
				t.Fatalf("unexpected duration: got %d want %d", got, c.want)
			}
		})
	}
}

func TestDefaultHistogramStepFromUnixRange(t *testing.T) {
	cases := []struct {
		name  string
		start int64
		end   int64
		want  string
	}{
		{name: "empty range", start: 10, end: 10, want: "1s"},
		{name: "one minute", start: 0, end: 60, want: "1s"},
		{name: "five minutes", start: 0, end: 300, want: "5s"},
		{name: "fifteen minutes", start: 0, end: 900, want: "30s"},
		{name: "thirty minutes", start: 0, end: 1800, want: "30s"},
		{name: "one hour", start: 0, end: 3600, want: "1m"},
		{name: "six hours", start: 0, end: 3600 * 6, want: "5m"},
		{name: "twelve hours", start: 0, end: 3600 * 12, want: "10m"},
		{name: "one day", start: 0, end: 86400, want: "30m"},
		{name: "two days", start: 0, end: 86400 * 2, want: "1h"},
		{name: "one week", start: 0, end: 86400 * 7, want: "3h"},
		{name: "thirty days", start: 0, end: 86400 * 30, want: "12h"},
		{name: "ninety days", start: 0, end: 86400 * 90, want: "1d"},
		{name: "more than ninety days", start: 0, end: 86400*90 + 1, want: "2d"},
		{name: "milliseconds", start: 1710000000000, end: 1710003600000, want: "1m"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DefaultHistogramStepFromUnixRange(c.start, c.end); got != c.want {
				t.Fatalf("unexpected step: got %q, want %q", got, c.want)
			}
		})
	}
}
