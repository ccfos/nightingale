package mysql

import "testing"

func TestNormalizeDSNExtraParams(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"blank", "   ", ""},
		{"leading question mark", "?timeout=5s", "timeout=5s"},
		{"leading ampersand", "&timeout=5s", "timeout=5s"},
		{"go syntax passes through", "charset=utf8mb4&timeout=5s&tls=true", "charset=utf8mb4&timeout=5s&tls=true"},
		{
			"jdbc session variables",
			"sessionVariables=ob_read_consistency=WEAK,@proxy_route_policy=follower_first",
			"ob_read_consistency=%27WEAK%27&@proxy_route_policy=%27follower_first%27",
		},
		{
			"mixed",
			"readTimeout=30s&sessionVariables=ob_read_consistency=WEAK",
			"readTimeout=30s&ob_read_consistency=%27WEAK%27",
		},
		{
			"numeric and boolean stay bare",
			"sessionVariables=isolation_level=1,autocommit=true,ratio=0.5",
			"isolation_level=1&autocommit=true&ratio=0.5",
		},
		{
			"already single quoted",
			"sessionVariables=time_zone='+08:00'",
			"time_zone=%27%2B08%3A00%27",
		},
		{
			"already percent encoded",
			"sessionVariables=time_zone=%27UTC%27",
			"time_zone=%27UTC%27",
		},
		{"empty segments skipped", "timeout=5s&&", "timeout=5s"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NormalizeDSNExtraParams(c.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestNormalizeDSNExtraParamsInvalid(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"segment without equals", "timeout"},
		{"empty key", "=5s"},
		{"empty session variables", "sessionVariables="},
		{"session variable without equals", "sessionVariables=ob_read_consistency"},
		{"session variable empty key", "sessionVariables==WEAK"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NormalizeDSNExtraParams(c.raw); err == nil {
				t.Fatalf("expected error for %q", c.raw)
			}
		})
	}
}
