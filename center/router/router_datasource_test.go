package router

import "testing"

func TestSanitizeDsError(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "url with credentials",
			in:   `request url:http://admin:s3cret@10.0.0.12:9090/api/v1/query failed`,
			want: `request url:http://***:***@10.0.0.12:9090/api/v1/query failed`,
		},
		{
			name: "https url with credentials",
			in:   `dial https://user:pa$$word@es.internal:9200: connection refused`,
			want: `dial https://***:***@es.internal:9200: connection refused`,
		},
		{
			name: "url without credentials untouched",
			in:   `request url:http://10.0.0.12:9090/api/v1/query failed code:404`,
			want: `request url:http://10.0.0.12:9090/api/v1/query failed code:404`,
		},
		{
			name: "plain message untouched",
			in:   `x509: certificate signed by unknown authority`,
			want: `x509: certificate signed by unknown authority`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeDsError(tc.in); got != tc.want {
				t.Errorf("sanitizeDsError(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
