package loki

import "testing"

func TestAPIURLPreservesConfiguredLokiPath(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{
			name: "with loki suffix",
			addr: "http://example.com:3100/loki",
			want: "http://example.com:3100/loki/api/v1/labels",
		},
		{
			name: "bare host",
			addr: "http://example.com:3100",
			want: "http://example.com:3100/api/v1/labels",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vl := &Loki{LokiAddr: tt.addr}
			got, err := vl.apiURL("/api/v1/labels")
			if err != nil {
				t.Fatalf("apiURL failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected url: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestFormatLokiTimestamp(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want string
	}{
		{name: "seconds", in: 1710000000, want: "1710000000000000000"},
		{name: "milliseconds", in: 1710000000000, want: "1710000000000000000"},
		{name: "nanoseconds", in: 1710000000000000000, want: "1710000000000000000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatLokiTimestamp(tt.in); got != tt.want {
				t.Fatalf("unexpected timestamp: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeLogsKeepsMillisecondTimestampAndRawNanoseconds(t *testing.T) {
	resp := &QueryResponse{
		Data: QueryData{
			Result: []QueryItem{
				{
					Stream: map[string]string{"job": "api"},
					Values: [][]interface{}{
						{"1710000000123456789", "hello"},
					},
				},
			},
		},
	}

	logs := NormalizeLogs(resp)
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if got := logs[0]["timestamp"]; got != int64(1710000000123) {
		t.Fatalf("unexpected timestamp: got %#v", got)
	}
	if got := logs[0]["__timestamp__"]; got != "1710000000123456789" {
		t.Fatalf("unexpected __timestamp__: got %#v", got)
	}
	if _, exists := logs[0]["timestamp_ns"]; exists {
		t.Fatalf("timestamp_ns should not be returned")
	}
	if _, exists := logs[0]["stream"]; exists {
		t.Fatalf("stream should not be returned")
	}
	labels, ok := logs[0]["labels"].(map[string]string)
	if !ok {
		t.Fatalf("labels should be returned as map[string]string, got %#v", logs[0]["labels"])
	}
	if got := labels["job"]; got != "api" {
		t.Fatalf("unexpected labels job: got %#v", got)
	}
	parsedFields, ok := logs[0]["parsed_fields"].(map[string]string)
	if !ok {
		t.Fatalf("parsed_fields should be returned as map[string]string, got %#v", logs[0]["parsed_fields"])
	}
	if len(parsedFields) != 0 {
		t.Fatalf("parsed_fields should be empty without label names, got %#v", parsedFields)
	}
	if _, exists := logs[0]["job"]; exists {
		t.Fatalf("stream should not be flattened into top-level fields")
	}
	if _, exists := logs[0]["stream.job"]; exists {
		t.Fatalf("stream should not be flattened into stream.* fields")
	}
}

func TestNormalizeLogsWithLabelNamesSplitsParsedFields(t *testing.T) {
	resp := &QueryResponse{
		Data: QueryData{
			Result: []QueryItem{
				{
					Stream: map[string]string{
						"job":               "api",
						"level":             "error",
						"trace_id":          "abc",
						"__error__":         "JSONParserErr",
						"__error_details__": "detail",
					},
					Values: [][]interface{}{
						{"1710000000123456789", "hello"},
					},
				},
			},
		},
	}

	logs := NormalizeLogsWithLabelNames(resp, []string{"job", "level"})
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}

	labels, ok := logs[0]["labels"].(map[string]string)
	if !ok {
		t.Fatalf("labels should be map[string]string, got %#v", logs[0]["labels"])
	}
	if got := labels["job"]; got != "api" {
		t.Fatalf("unexpected labels job: got %#v", got)
	}
	if got := labels["level"]; got != "error" {
		t.Fatalf("unexpected labels level: got %#v", got)
	}
	if _, exists := labels["trace_id"]; exists {
		t.Fatalf("trace_id should not be in labels")
	}

	parsedFields, ok := logs[0]["parsed_fields"].(map[string]string)
	if !ok {
		t.Fatalf("parsed_fields should be map[string]string, got %#v", logs[0]["parsed_fields"])
	}
	if got := parsedFields["trace_id"]; got != "abc" {
		t.Fatalf("unexpected parsed trace_id: got %#v", got)
	}
	if _, exists := parsedFields["__error__"]; exists {
		t.Fatalf("__error__ should not be returned in parsed_fields")
	}
	if _, exists := parsedFields["__error_details__"]; exists {
		t.Fatalf("__error_details__ should not be returned in parsed_fields")
	}
}

func TestNormalizeLogsWithEmptyLabelNamesTreatsStreamAsParsedFields(t *testing.T) {
	resp := &QueryResponse{
		Data: QueryData{
			Result: []QueryItem{
				{
					Stream: map[string]string{"job": "api", "trace_id": "abc"},
					Values: [][]interface{}{
						{"1710000000123456789", "hello"},
					},
				},
			},
		},
	}

	logs := NormalizeLogsWithLabelNames(resp, []string{})
	labels := logs[0]["labels"].(map[string]string)
	if len(labels) != 0 {
		t.Fatalf("labels should be empty with an empty label names set, got %#v", labels)
	}
	parsedFields := logs[0]["parsed_fields"].(map[string]string)
	if got := parsedFields["job"]; got != "api" {
		t.Fatalf("job should fall back to parsed_fields when label names are unknown, got %#v", got)
	}
}

func TestAnalyzeLogQLForLogFields(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		selector   string
		needsNames bool
	}{
		{
			name:       "selector only",
			query:      `{app="api"}`,
			selector:   `{app="api"}`,
			needsNames: false,
		},
		{
			name:       "line filters do not need label names",
			query:      `{app="api"} |= "error" |~ "timeout|failed"`,
			selector:   `{app="api"}`,
			needsNames: false,
		},
		{
			name:       "json stage needs label names",
			query:      `{app="api"} | json | level="error"`,
			selector:   `{app="api"}`,
			needsNames: true,
		},
		{
			name:       "parser text inside string is ignored",
			query:      `{app="api"} |= "| json"`,
			selector:   `{app="api"}`,
			needsNames: false,
		},
		{
			name:       "regexp stage needs label names",
			query:      `{app="api"} | regexp "(?P<level>\\w+)"`,
			selector:   `{app="api"}`,
			needsNames: true,
		},
		{
			name:       "label format needs label names",
			query:      `{app="api"} | label_format level="{{.severity}}"`,
			selector:   `{app="api"}`,
			needsNames: true,
		},
		{
			name:       "selector keeps braces inside quoted matcher",
			query:      `{app=~"api\\{1\\}"} | logfmt`,
			selector:   `{app=~"api\\{1\\}"}`,
			needsNames: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnalyzeLogQLForLogFields(tt.query)
			if got.Selector != tt.selector {
				t.Fatalf("unexpected selector: got %q want %q", got.Selector, tt.selector)
			}
			if got.NeedsLabelNames != tt.needsNames {
				t.Fatalf("unexpected NeedsLabelNames: got %v want %v", got.NeedsLabelNames, tt.needsNames)
			}
		})
	}
}

func TestExtractLogQLLabelMatchers(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []LogQLLabelMatcher
	}{
		{
			name:  "multiple exact matchers",
			query: `{app="api",namespace="prod"}`,
			want: []LogQLLabelMatcher{
				{Key: "app", Op: "=", Value: "api"},
				{Key: "namespace", Op: "=", Value: "prod"},
			},
		},
		{
			name:  "comma inside value",
			query: `{app="api,prod"}`,
			want: []LogQLLabelMatcher{
				{Key: "app", Op: "=", Value: "api,prod"},
			},
		},
		{
			name:  "brace inside value",
			query: `{app="api}prod"}`,
			want: []LogQLLabelMatcher{
				{Key: "app", Op: "=", Value: "api}prod"},
			},
		},
		{
			name:  "regex matcher",
			query: `{app=~"api|web"}`,
			want: []LogQLLabelMatcher{
				{Key: "app", Op: "=~", Value: "api|web"},
			},
		},
		{
			name:  "negative matchers",
			query: `{app!="api",namespace!~"dev|test"}`,
			want: []LogQLLabelMatcher{
				{Key: "app", Op: "!=", Value: "api"},
				{Key: "namespace", Op: "!~", Value: "dev|test"},
			},
		},
		{
			name:  "pipeline is ignored",
			query: `{app="api"} |= "err" | json`,
			want: []LogQLLabelMatcher{
				{Key: "app", Op: "=", Value: "api"},
			},
		},
		{
			name:  "escaped quote and slash",
			query: `{app="api\"prod",path="c:\\tmp"}`,
			want: []LogQLLabelMatcher{
				{Key: "app", Op: "=", Value: `api"prod`},
				{Key: "path", Op: "=", Value: `c:\tmp`},
			},
		},
		{
			name:  "bad matcher is skipped",
			query: `{app="api",bad,namespace="prod"}`,
			want: []LogQLLabelMatcher{
				{Key: "app", Op: "=", Value: "api"},
				{Key: "namespace", Op: "=", Value: "prod"},
			},
		},
		{
			name:  "no selector",
			query: `rate(foo[5m])`,
			want:  nil,
		},
		{
			name:  "incomplete selector",
			query: `{app="api"`,
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractLogQLLabelMatchers(tt.query)
			if len(got) != len(tt.want) {
				t.Fatalf("unexpected matcher count: got %#v want %#v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("unexpected matcher[%d]: got %#v want %#v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
