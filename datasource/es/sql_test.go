package es

import (
	"testing"
)

func TestMajorVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    int
		wantErr bool
	}{
		{"v7.17.10", "7.17.10", 7, false},
		{"v7.10.0", "7.10.0", 7, false},
		{"v8.19.4", "8.19.4", 8, false},
		{"v9.3.2", "9.3.2", 9, false},
		{"v8.14.0-SNAPSHOT", "8.14.0-SNAPSHOT", 8, false},
		{"v9.0.0", "9.0.0", 9, false},
		{"v8", "8", 8, false},
		{"empty", "", 0, true},
		{"invalid", "abc.def.ghi", 0, true},
		{"no-dot", "8abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := majorVersion(tt.version)
			if tt.wantErr {
				if err == nil {
					t.Errorf("majorVersion(%q) expected error, got nil", tt.version)
				}
				return
			}
			if err != nil {
				t.Fatalf("majorVersion(%q) unexpected error: %v", tt.version, err)
			}
			if got != tt.want {
				t.Errorf("majorVersion(%q) = %d, want %d", tt.version, got, tt.want)
			}
		})
	}
}

func TestExpandTimeMacros(t *testing.T) {
	from := int64(1700000000)
	to := int64(1700003600)

	tests := []struct {
		name       string
		sql        string
		timezone   string
		timeFormat string
		want       string
	}{
		{
			name: "no macro",
			sql:  `SELECT * FROM "logs"`,
			want: `SELECT * FROM "logs"`,
		},
		{
			name: "timeFilter",
			sql:  `SELECT * FROM "logs" WHERE $__timeFilter("@timestamp")`,
			want: `SELECT * FROM "logs" WHERE ("@timestamp" >= 1700000000 AND "@timestamp" < 1700003600)`,
		},
		{
			name: "timeFilter_ms",
			sql:  `SELECT * FROM "logs" WHERE $__timeFilter_ms("@timestamp")`,
			want: `SELECT * FROM "logs" WHERE ("@timestamp" >= 1700000000000 AND "@timestamp" < 1700003600000)`,
		},
		{
			name:       "datetimeFilter UTC",
			sql:        `SELECT * FROM "logs" WHERE $__datetimeFilter("@timestamp")`,
			timezone:   "UTC",
			timeFormat: "2006-01-02T15:04:05.000Z",
			want:       `SELECT * FROM "logs" WHERE ("@timestamp" >= '2023-11-14T22:13:20.000Z' AND "@timestamp" < '2023-11-14T23:13:20.000Z')`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandTimeMacros(tt.sql, from, to, tt.timezone, tt.timeFormat)
			if err != nil {
				t.Fatalf("ExpandTimeMacros() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ExpandTimeMacros() =\n  %s\nwant:\n  %s", got, tt.want)
			}
		})
	}
}
