package victorialogs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	vlkit "github.com/ccfos/nightingale/v6/dskit/victorialogs"
)

func TestApplyLogSortQuery(t *testing.T) {
	tests := []struct {
		name  string
		query string
		desc  bool
		want  string
	}{
		{
			name:  "desc",
			query: "*",
			desc:  true,
			want:  "* | sort by (_time) desc",
		},
		{
			name:  "asc",
			query: "error",
			desc:  false,
			want:  "error | sort by (_time)",
		},
		{
			name:  "skip existing sort",
			query: "error | sort by (service) desc",
			desc:  true,
			want:  "error | sort by (service) desc",
		},
		{
			name:  "skip existing order alias",
			query: "error | order by (service) desc",
			desc:  true,
			want:  "error | order by (service) desc",
		},
		{
			name:  "do not treat sort prefix as sort pipe",
			query: "error | sort_stats by (service)",
			desc:  true,
			want:  "error | sort_stats by (service) | sort by (_time) desc",
		},
		{
			name:  "empty query unchanged",
			query: "",
			desc:  true,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := applyLogSortQuery(tt.query, tt.desc); got != tt.want {
				t.Fatalf("unexpected query: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestResolveLogTimeDesc(t *testing.T) {
	tests := []struct {
		name    string
		reverse bool
		want    bool
	}{
		{name: "reverse true", reverse: true, want: true},
		{name: "reverse false", want: false},
		{name: "default asc", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			param := &Query{Reverse: tt.reverse}
			if got := resolveLogTimeDesc(param); got != tt.want {
				t.Fatalf("resolveLogTimeDesc() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQueryLogAddsSort(t *testing.T) {
	var queryQuery string
	var hitsQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/select/logsql/query":
			queryQuery = r.Form.Get("query")
			fmt.Fprintln(w, `{"_time":"2026-07-02T00:00:01Z","_msg":"a"}`)
			fmt.Fprintln(w, `{"_time":"2026-07-02T00:00:02Z","_msg":"b"}`)
		case "/select/logsql/hits":
			hitsQuery = r.Form.Get("query")
			fmt.Fprintln(w, `{"hits":[{"total":2,"fields":{},"timestamps":[],"values":[]}]}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	vl := &VictoriaLogs{
		VictoriaLogs: vlkit.VictoriaLogs{
			VictorialogsAddr: server.URL,
			MaxQueryRows:     1000,
		},
	}
	if err := vl.InitClient(); err != nil {
		t.Fatalf("InitClient error: %v", err)
	}

	_, total, err := vl.QueryLog(context.Background(), Query{
		Query: "*",
		Start: 11,
		End:   22,
		Limit: 30,
	})
	if err != nil {
		t.Fatalf("QueryLog error: %v", err)
	}
	if queryQuery != "* | sort by (_time)" {
		t.Fatalf("unexpected query query: %q", queryQuery)
	}
	if hitsQuery != "*" {
		t.Fatalf("hits should use original query without sort pipe: %q", hitsQuery)
	}
	if total != 2 {
		t.Fatalf("unexpected total: %d", total)
	}

	_, _, err = vl.QueryLog(context.Background(), map[string]interface{}{
		"query":   "*",
		"start":   int64(11),
		"end":     int64(22),
		"limit":   30,
		"reverse": false,
	})
	if err != nil {
		t.Fatalf("QueryLog with reverse=false error: %v", err)
	}
	if queryQuery != "* | sort by (_time)" {
		t.Fatalf("unexpected asc query: %q", queryQuery)
	}

	_, _, err = vl.QueryLog(context.Background(), map[string]interface{}{
		"query":   "*",
		"start":   int64(11),
		"end":     int64(22),
		"limit":   30,
		"reverse": true,
	})
	if err != nil {
		t.Fatalf("QueryLog with reverse=true error: %v", err)
	}
	if queryQuery != "* | sort by (_time) desc" {
		t.Fatalf("unexpected desc query: %q", queryQuery)
	}
}

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
