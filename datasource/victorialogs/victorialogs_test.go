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

func TestVictoriaLogsFieldSuggestionTypes(t *testing.T) {
	field := FieldName{Field: "status"}
	if field.Field != "status" {
		t.Fatalf("unexpected field name suggestion: %+v", field)
	}

	value := FieldValue{Value: "200", Count: 123}
	if value.Value != "200" || value.Count != 123 {
		t.Fatalf("unexpected field value suggestion: %+v", value)
	}
}
