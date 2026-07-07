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
	stream, ok := logs[0]["stream"].(map[string]string)
	if !ok {
		t.Fatalf("stream should be returned as map[string]string, got %#v", logs[0]["stream"])
	}
	if got := stream["job"]; got != "api" {
		t.Fatalf("unexpected stream job: got %#v", got)
	}
	if _, exists := logs[0]["labels"]; exists {
		t.Fatalf("labels should not be returned; Loki stream should be returned as stream")
	}
	if _, exists := logs[0]["job"]; exists {
		t.Fatalf("stream should not be flattened into top-level fields")
	}
	if _, exists := logs[0]["stream.job"]; exists {
		t.Fatalf("stream should not be flattened into stream.* fields")
	}
}
