package victorialogs

import (
	"reflect"
	"testing"

	vlkit "github.com/ccfos/nightingale/v6/dskit/victorialogs"
)

func TestConvertHitsToHistogramValues(t *testing.T) {
	hits := []vlkit.HitResult{
		{
			Timestamps: []interface{}{"2026-07-02T00:00:00Z", float64(1782950700000), float64(1782951000000000000), "bad"},
			Values:     []interface{}{"3", nil, 4, "5"},
			Fields: map[string]string{
				"service": "api",
				"level":   "error",
			},
		},
	}

	got := convertHitsToHistogramValues(hits, "service")
	if len(got) != 1 {
		t.Fatalf("expected 1 series, got %d", len(got))
	}
	if got[0].Ref != "api" {
		t.Fatalf("unexpected ref: %q", got[0].Ref)
	}
	if !reflect.DeepEqual(got[0].Metric, map[string]interface{}{"service": "api", "level": "error"}) {
		t.Fatalf("unexpected metric: %#v", got[0].Metric)
	}

	wantValues := [][]interface{}{
		{int64(1782950400), float64(3)},
		{int64(1782950700), nil},
		{int64(1782951000), float64(4)},
	}
	if !reflect.DeepEqual(got[0].Values, wantValues) {
		t.Fatalf("unexpected values: %#v", got[0].Values)
	}
}

func TestHistogramRefFallbackSortsFields(t *testing.T) {
	got := histogramRef(map[string]string{"b": "2", "a": "1"}, "")
	if got != "a=1,b=2" {
		t.Fatalf("unexpected ref: %q", got)
	}
}

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
			if got := vlkit.DefaultHistogramStep(c.start, c.end); got != c.want {
				t.Fatalf("unexpected step: got %q, want %q", got, c.want)
			}
		})
	}
}

func TestVictoriaLogsFieldSuggestionTypes(t *testing.T) {
	field := FieldName{Field: "status", Type: "string", Builtin: false}
	if field.Field != "status" || field.Type != "string" || field.Builtin {
		t.Fatalf("unexpected field name suggestion: %+v", field)
	}

	value := FieldValue{Value: "200", Count: 123}
	if value.Value != "200" || value.Count != 123 {
		t.Fatalf("unexpected field value suggestion: %+v", value)
	}
}
