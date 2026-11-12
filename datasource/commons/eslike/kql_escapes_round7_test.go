package eslike

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompileKQL_DecodesQuotedControlAndUnicodeEscapes(t *testing.T) {
	query, err := CompileKQL(`message: "line\nnext\t\r\u0041\u006a\u00AF\"\\"`, KQLOptions{})

	require.NoError(t, err)
	require.Equal(t, should(map[string]interface{}{
		"match_phrase": map[string]interface{}{"message": "line\nnext\t\rAj¯\"\\"},
	}), query)
}

func TestCompileKQL_RejectsIncompleteOrMalformedEscapes(t *testing.T) {
	cases := []struct{ name, filter, want string }{
		{name: "quoted trailing backslash", filter: `message: "abc\`, want: "unterminated escape"},
		{name: "atom trailing backslash", filter: `message: abc\`, want: "unterminated escape"},
		{name: "short unicode", filter: `message: "\u12`, want: "invalid unicode escape"},
		{name: "invalid unicode digit", filter: `message: "\u00XZ"`, want: "invalid unicode escape"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query, err := CompileKQL(tc.filter, KQLOptions{})

			require.ErrorContains(t, err, tc.want)
			require.Contains(t, err.Error(), "position")
			require.Nil(t, query)
		})
	}
}

func TestCompileKQL_RejectsUnclosedScopesAndMissingValues(t *testing.T) {
	cases := []struct{ name, filter, want string }{
		{name: "group", filter: `(host: api`, want: "missing closing parenthesis"},
		{name: "field group", filter: `host: (api OR worker`, want: "missing closing parenthesis"},
		{name: "nested", filter: `event: {host: api`, want: "missing closing nested scope"},
		{name: "empty field value", filter: `host:`, want: "value is required"},
		{name: "empty range value", filter: `latency >=`, want: "value is required"},
		{name: "empty nested", filter: `event: {}`, want: "expected KQL term"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query, err := CompileKQL(tc.filter, KQLOptions{})

			require.ErrorContains(t, err, tc.want)
			require.Nil(t, query)
		})
	}
}
