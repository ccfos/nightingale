package victorialogs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

func newTestVictoriaLogs(serverURL string) *VictoriaLogs {
	vl := &VictoriaLogs{
		VictorialogsAddr: serverURL,
		Headers:          make(map[string]string),
		MaxQueryRows:     1000,
	}
	if err := vl.InitHTTPClient(); err != nil {
		panic(err)
	}
	return vl
}

func TestVictoriaLogs_QueryWithOffsetAddsOffset(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/select/logsql/query" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		got = r.Form
		fmt.Fprintln(w, `{"_msg":"ok"}`)
	}))
	defer server.Close()

	logs, err := newTestVictoriaLogs(server.URL).QueryWithOffset(context.Background(), "*", 11, 22, 10, 30)
	if err != nil {
		t.Fatalf("QueryWithOffset error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}

	want := map[string]string{
		"query":  "*",
		"start":  "11",
		"end":    "22",
		"limit":  "10",
		"offset": "30",
	}
	for key, value := range want {
		if got.Get(key) != value {
			t.Fatalf("unexpected %s: got %q, want %q", key, got.Get(key), value)
		}
	}
}

func TestVictoriaLogs_QueryHitsWithFieldsLimit(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/select/logsql/hits" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		got = r.Form
		fmt.Fprintln(w, `{"hits":[{"total":3,"timestamps":["2026-07-02T00:00:00Z"],"values":[3],"fields":{"service":"api"}}]}`)
	}))
	defer server.Close()

	result, err := newTestVictoriaLogs(server.URL).QueryHitsWithFieldsLimit(context.Background(), "*", 11, 22, "5m", 20, "service")
	if err != nil {
		t.Fatalf("QueryHitsWithFieldsLimit error: %v", err)
	}
	if len(result.Hits) != 1 || result.Hits[0].Total != 3 {
		t.Fatalf("unexpected hits result: %+v", result)
	}

	if got.Get("step") != "5m" {
		t.Fatalf("unexpected step: %q", got.Get("step"))
	}
	if got.Get("fields_limit") != "20" {
		t.Fatalf("unexpected fields_limit: %q", got.Get("fields_limit"))
	}
	if fields := got["field"]; !reflect.DeepEqual(fields, []string{"service"}) {
		t.Fatalf("unexpected field params: %#v", fields)
	}
}

func TestVictoriaLogs_StreamFieldNames(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/select/logsql/stream_field_names" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		got = r.Form
		fmt.Fprintln(w, `{"values":[{"value":"z","hits":1},{"value":"a","hits":2}]}`)
	}))
	defer server.Close()

	fields, err := newTestVictoriaLogs(server.URL).StreamFieldNames(context.Background(), "", 11, 22, "svc")
	if err != nil {
		t.Fatalf("StreamFieldNames error: %v", err)
	}
	wantFields := []StreamFieldValue{
		{Value: "z", Hits: 1},
		{Value: "a", Hits: 2},
	}
	if !reflect.DeepEqual(fields, wantFields) {
		t.Fatalf("unexpected fields: %#v", fields)
	}
	if got.Get("query") != "*" {
		t.Fatalf("unexpected query: %q", got.Get("query"))
	}
	if got.Get("ignore_pipes") != "1" {
		t.Fatalf("unexpected ignore_pipes: %q", got.Get("ignore_pipes"))
	}
	if got.Get("filter") != "svc" {
		t.Fatalf("unexpected filter: %q", got.Get("filter"))
	}
}

func TestVictoriaLogs_StreamFieldValues(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/select/logsql/stream_field_values" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		got = r.Form
		fmt.Fprintln(w, `{"values":[{"value":"api","hits":7}]}`)
	}))
	defer server.Close()

	values, err := newTestVictoriaLogs(server.URL).StreamFieldValues(context.Background(), "_time:5m", 11, 22, "service", 10, "")
	if err != nil {
		t.Fatalf("StreamFieldValues error: %v", err)
	}
	if len(values) != 1 || values[0].Value != "api" || values[0].Hits != 7 {
		t.Fatalf("unexpected values: %#v", values)
	}

	want := map[string]string{
		"query":        "_time:5m",
		"field":        "service",
		"limit":        "10",
		"ignore_pipes": "1",
	}
	for key, value := range want {
		if got.Get(key) != value {
			t.Fatalf("unexpected %s: got %q, want %q", key, got.Get(key), value)
		}
	}
}
