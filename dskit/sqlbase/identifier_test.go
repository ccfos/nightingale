package sqlbase

import (
	"strings"
	"testing"
)

func TestValidateIdentifier(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"simple", "users", false},
		{"with_underscore", "user_table_1", false},
		{"with_dash", "user-table", false},
		{"with_dot", "schema.table", false},
		{"unicode_letters", "用户表", false},
		{"empty", "", true},
		{"too_long", strings.Repeat("a", 129), true},
		{"semicolon", "users;DROP", true},
		{"single_quote", "users'", true},
		{"double_quote", `users"`, true},
		{"backtick", "users`", true},
		{"backslash", `users\`, true},
		{"null_byte", "users\x00", true},
		{"space", "users x", true},
		{"tab", "users\tx", true},
		{"newline", "users\nx", true},
		{"line_comment", "users--x", true},
		{"block_comment_open", "users/*x", true},
		{"block_comment_close", "users*/x", true},
		{"injection_payload", "x'; DROP TABLE y; --", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateIdentifier(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
		})
	}
}

func TestQuoteBacktick(t *testing.T) {
	if got, want := QuoteBacktick("users"), "`users`"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := QuoteBacktick("a`b"), "`a``b`"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestQuoteDouble(t *testing.T) {
	if got, want := QuoteDouble("users"), `"users"`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := QuoteDouble(`a"b`), `"a""b"`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
