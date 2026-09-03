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

func TestIsESSQLSupported(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"7.0.0", true},
		{"7.17.10", true},
		{"8.15.0", true},
		{"9.0.0", true},
		{"6.8.0", false},
		{"5.6.0", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := IsESSQLSupported(tt.version); got != tt.want {
				t.Errorf("IsESSQLSupported(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}
