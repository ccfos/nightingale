package types

import "testing"

func TestDefaultHistogramStep(t *testing.T) {
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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DefaultHistogramStep(c.start, c.end); got != c.want {
				t.Fatalf("unexpected step: got %q, want %q", got, c.want)
			}
		})
	}
}
