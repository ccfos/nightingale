package victorialogs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
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
		"start":  "11000",
		"end":    "22000",
		"limit":  "10",
		"offset": "30",
	}
	for key, value := range want {
		if got.Get(key) != value {
			t.Fatalf("unexpected %s: got %q, want %q", key, got.Get(key), value)
		}
	}
}

func TestFormatVictoriaLogsTimestamp(t *testing.T) {
	cases := []struct {
		name string
		in   int64
		want string
	}{
		{name: "seconds", in: 1710000000, want: "1710000000000"},
		{name: "milliseconds", in: 1710000000000, want: "1710000000000"},
		{name: "zero", in: 0, want: "0"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatVictoriaLogsTimestamp(c.in); got != c.want {
				t.Fatalf("unexpected timestamp: got %q want %q", got, c.want)
			}
		})
	}
}

func TestVictoriaLogs_QueryWithOffsetKeepsMillisecondRange(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		got = r.Form
		fmt.Fprintln(w, `{"_msg":"ok"}`)
	}))
	defer server.Close()

	_, err := newTestVictoriaLogs(server.URL).QueryWithOffset(context.Background(), "*", 1784526946385, 1784537746385, 10, 0)
	if err != nil {
		t.Fatalf("QueryWithOffset error: %v", err)
	}
	if got.Get("start") != "1784526946385" || got.Get("end") != "1784537746385" {
		t.Fatalf("unexpected range: start=%q end=%q", got.Get("start"), got.Get("end"))
	}
}

func TestVictoriaLogs_HitsLogsAddsStep(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/select/logsql/hits" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		got = r.Form
		fmt.Fprintln(w, `{"hits":[{"total":12345,"fields":{},"timestamps":[],"values":[]}]}`)
	}))
	defer server.Close()

	total, err := newTestVictoriaLogs(server.URL).HitsLogs(context.Background(), "*", 11, 22)
	if err != nil {
		t.Fatalf("HitsLogs error: %v", err)
	}
	if total != 12345 {
		t.Fatalf("unexpected total: %d", total)
	}
	if got.Get("step") != "1s" {
		t.Fatalf("unexpected step: got %q, want %q", got.Get("step"), "1s")
	}
	if got.Get("start") != "11000" || got.Get("end") != "22000" {
		t.Fatalf("unexpected range: start=%q end=%q", got.Get("start"), got.Get("end"))
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

func TestVictoriaLogs_FieldNames(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/select/logsql/field_names" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		got = r.Form
		fmt.Fprintln(w, `{"values":[{"value":"status","hits":4},{"value":"domain","hits":2}]}`)
	}))
	defer server.Close()

	fields, err := newTestVictoriaLogs(server.URL).FieldNames(context.Background(), "service:api", 11, 22, 50, "sta")
	if err != nil {
		t.Fatalf("FieldNames error: %v", err)
	}
	wantFields := []StreamFieldValue{
		{Value: "status", Hits: 4},
		{Value: "domain", Hits: 2},
	}
	if !reflect.DeepEqual(fields, wantFields) {
		t.Fatalf("unexpected fields: %#v", fields)
	}

	want := map[string]string{
		"query":        "service:api",
		"start":        "11000",
		"end":          "22000",
		"limit":        "50",
		"filter":       "sta",
		"ignore_pipes": "1",
	}
	for key, value := range want {
		if got.Get(key) != value {
			t.Fatalf("unexpected %s: got %q, want %q", key, got.Get(key), value)
		}
	}
}

func TestVictoriaLogs_FieldValues(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/select/logsql/field_values" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		got = r.Form
		fmt.Fprintln(w, `{"values":[{"value":"200","hits":123},{"value":"404","hits":31}]}`)
	}))
	defer server.Close()

	values, err := newTestVictoriaLogs(server.URL).FieldValues(context.Background(), "service:api", 11, 22, "status", 20, "20")
	if err != nil {
		t.Fatalf("FieldValues error: %v", err)
	}
	wantValues := []StreamFieldValue{
		{Value: "200", Hits: 123},
		{Value: "404", Hits: 31},
	}
	if !reflect.DeepEqual(values, wantValues) {
		t.Fatalf("unexpected values: %#v", values)
	}

	want := map[string]string{
		"query":        "service:api",
		"field":        "status",
		"limit":        "20",
		"filter":       "20",
		"ignore_pipes": "1",
	}
	for key, value := range want {
		if got.Get(key) != value {
			t.Fatalf("unexpected %s: got %q, want %q", key, got.Get(key), value)
		}
	}
}

// 一条超长日志（如 Java 堆栈）只应截断它自己，不能让同批的其他日志一起查询失败。
func TestVictoriaLogs_QueryTruncatesOversizedFieldOnly(t *testing.T) {
	huge := strings.Repeat("at com.example.Foo.bar(Foo.java:123)\n", 4000) // ~148KB
	lines := []string{
		`{"_time":"2026-08-13T07:00:00Z","_msg":"before"}`,
		mustMarshalLine(t, map[string]string{"_time": "2026-08-13T07:00:01Z", "_msg": huge}),
		`{"_time":"2026-08-13T07:00:02Z","_msg":"after"}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.Join(lines, "\n")+"\n")
	}))
	defer server.Close()

	logs, err := newTestVictoriaLogs(server.URL).Query(context.Background(), "*", 11, 22, 10)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 log entries, got %d", len(logs))
	}

	// 相邻的正常日志必须逐字节不变
	if got := logs[0]["_msg"]; got != "before" {
		t.Fatalf("entry before the oversized one was altered: %v", got)
	}
	if got := logs[2]["_msg"]; got != "after" {
		t.Fatalf("entry after the oversized one was altered: %v", got)
	}

	// 超长的那条被截断、保留了开头、并带上原始大小
	msg, ok := logs[1]["_msg"].(string)
	if !ok {
		t.Fatalf("oversized _msg is not a string: %T", logs[1]["_msg"])
	}
	if len(msg) >= len(huge) {
		t.Fatalf("oversized _msg was not truncated: %d bytes", len(msg))
	}
	if !strings.HasPrefix(msg, "at com.example.Foo.bar(Foo.java:123)") {
		t.Fatalf("truncation dropped the head of the stack trace: %.60q", msg)
	}
	if !strings.Contains(msg, fmt.Sprintf("(truncated, total %d bytes)", len(huge))) {
		t.Fatalf("missing truncation marker: %q", msg[len(msg)-80:])
	}
	// 其他字段不受牵连
	if got := logs[1]["_time"]; got != "2026-08-13T07:00:01Z" {
		t.Fatalf("sibling field of the oversized entry was altered: %v", got)
	}
}

// 截断点落在多字节字符中间时，不能切出非法 UTF-8。
func TestVictoriaLogs_TruncateKeepsValidUTF8(t *testing.T) {
	// 每个"中"占 3 字节，64KB 不是 3 的整数倍，截断点必然落在字符中间
	s := strings.Repeat("中", maxLogFieldSize)
	entry := LogEntry{"_msg": s}
	truncateLogEntry(entry)

	got := entry["_msg"].(string)
	if !utf8.ValidString(got) {
		t.Fatal("truncated value is not valid UTF-8")
	}
	if !strings.HasPrefix(got, "中中中") {
		t.Fatalf("unexpected truncated head: %.20q", got)
	}
}

func mustMarshalLine(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// 坏行报错时只回带开头一小段，且不能因按字节切断而产出非法 UTF-8。
func TestVictoriaLogs_QueryBadLineErrorIsBoundedAndValidUTF8(t *testing.T) {
	// 前缀 9 字节 + 每个"中"3 字节：1KB 和 64KB 两个截断点都必然落在某个"中"中间
	broken := `{"_msg":"` + strings.Repeat("中", 30000) // 少了收尾的引号和花括号，共 ~88KB
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"_time":"2026-08-13T07:00:00Z","_msg":"ok"}`+"\n"+broken+"\n")
	}))
	defer server.Close()

	_, err := newTestVictoriaLogs(server.URL).Query(context.Background(), "*", 11, 22, 10)
	if err == nil {
		t.Fatal("expected an error for the malformed line")
	}
	msg := err.Error()
	if !utf8.ValidString(msg) {
		t.Fatalf("error message is not valid UTF-8: %q", msg[max(0, len(msg)-16):])
	}
	if len(msg) > maxErrLineSnippet+256 {
		t.Fatalf("error message carries too much of the raw line: %d bytes", len(msg))
	}
	if !strings.Contains(msg, `line={"_msg":"中中中`) {
		t.Fatalf("error message lost the head of the bad line: %.80q", msg)
	}
}
