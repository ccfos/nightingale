package oauth2x

import (
	"testing"
)

func TestGetUserinfoField(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		isArray bool
		prefix  string
		field   string
		want    string
	}{
		{
			name:  "no prefix, flat object",
			input: `{"username":"alice","email":"alice@example.com"}`,
			field: "username",
			want:  "alice",
		},
		{
			name:   "single-level prefix (backward compat)",
			input:  `{"data":{"username":"bob","phone":"123"}}`,
			prefix: "data",
			field:  "username",
			want:   "bob",
		},
		{
			name:   "multi-level prefix data.user",
			input:  `{"data":{"user":{"loginName":"charlie","staffPhone":"456"}}}`,
			prefix: "data.user",
			field:  "loginName",
			want:   "charlie",
		},
		{
			name:   "three-level prefix a.b.c",
			input:  `{"a":{"b":{"c":{"name":"deep"}}}}`,
			prefix: "a.b.c",
			field:  "name",
			want:   "deep",
		},
		{
			name:    "no prefix with array",
			input:   `[{"username":"first"},{"username":"second"}]`,
			isArray: true,
			field:   "username",
			want:    "first",
		},
		{
			name:    "single prefix with array",
			input:   `{"data":[{"username":"arrUser"}]}`,
			isArray: true,
			prefix:  "data",
			field:   "username",
			want:    "arrUser",
		},
		{
			name:    "multi-level prefix with array",
			input:   `{"data":{"users":[{"loginName":"arrDeep"}]}}`,
			isArray: true,
			prefix:  "data.users",
			field:   "loginName",
			want:    "arrDeep",
		},
		{
			name:   "literal dot-key prefix (backward compat)",
			input:  `{"data.user":{"username":"dotkey"}}`,
			prefix: "data.user",
			field:  "username",
			want:   "dotkey",
		},
		{
			name:    "literal dot-key prefix with array",
			input:   `{"data.users":[{"username":"arrdot"}]}`,
			isArray: true,
			prefix:  "data.users",
			field:   "username",
			want:    "arrdot",
		},
		{
			name:   "dot-key takes priority over nested path",
			input:  `{"data.user":{"username":"literal"},"data":{"user":{"username":"nested"}}}`,
			prefix: "data.user",
			field:  "username",
			want:   "literal",
		},
		{
			name:   "falls back to nested path when literal key missing",
			input:  `{"data":{"user":{"username":"nested"}}}`,
			prefix: "data.user",
			field:  "username",
			want:   "nested",
		},
		{
			name:   "literal dot-key with empty string value does not fallback",
			input:  `{"data.user":{"username":""},"data":{"user":{"username":"nested"}}}`,
			prefix: "data.user",
			field:  "username",
			want:   "",
		},
		{
			name:    "literal dot-key with empty string value does not fallback (array)",
			input:   `{"data.users":[{"username":""}],"data":{"users":[{"username":"nested"}]}}`,
			isArray: true,
			prefix:  "data.users",
			field:   "username",
			want:    "",
		},
		{
			name:  "missing field returns empty string",
			input: `{"username":"alice"}`,
			field: "nonexistent",
			want:  "",
		},
		{
			name:   "missing prefix path returns empty string",
			input:  `{"data":{"username":"alice"}}`,
			prefix: "no.such.path",
			field:  "username",
			want:   "",
		},
		{
			name:  "empty field returns empty string",
			input: `{"username":"alice"}`,
			field: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getUserinfoField([]byte(tt.input), tt.isArray, tt.prefix, tt.field)
			if got != tt.want {
				t.Errorf("getUserinfoField() = %q, want %q", got, tt.want)
			}
		})
	}
}
