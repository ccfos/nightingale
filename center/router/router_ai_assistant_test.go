package router

import "testing"

func TestParseChatIDFromStreamID(t *testing.T) {
	cases := []struct {
		name     string
		streamID string
		want     string
	}{
		{"new format", "chat-uuid-1234:7:f47ac10b-58cc-4372-a567-0e02b2c3d479", "chat-uuid-1234"},
		{"legacy format", "chat-uuid-1234:f47ac10b-58cc-4372-a567-0e02b2c3d479", "chat-uuid-1234"},
		{"empty", "", ""},
		{"no colon", "chatid-only", ""},
		{"leading colon", ":foo", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseChatIDFromStreamID(tc.streamID)
			if got != tc.want {
				t.Fatalf("parseChatIDFromStreamID(%q) = %q, want %q", tc.streamID, got, tc.want)
			}
		})
	}
}

func TestParseSeqIDFromStreamID(t *testing.T) {
	cases := []struct {
		name     string
		streamID string
		want     int64
	}{
		{"new format", "chat-uuid-1234:7:f47ac10b-58cc-4372-a567-0e02b2c3d479", 7},
		{"new format large seq", "chat-uuid-1234:9999999999:abc", 9999999999},
		{"legacy 2-segment", "chat-uuid-1234:f47ac10b-58cc-4372-a567-0e02b2c3d479", 0},
		{"non-numeric middle", "chat-uuid-1234:notanumber:abc", 0},
		{"empty", "", 0},
		{"only colons", "::", 0},
		{"chatid with no rest", "chat-uuid", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSeqIDFromStreamID(tc.streamID)
			if got != tc.want {
				t.Fatalf("parseSeqIDFromStreamID(%q) = %d, want %d", tc.streamID, got, tc.want)
			}
		})
	}
}
