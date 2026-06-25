package skillgateway

import "testing"

func TestGrantMatched(t *testing.T) {
	cases := []struct {
		pats  []string
		token string
		want  bool
	}{
		{[]string{"alert:read"}, "alert:read", true},
		{[]string{"alert:read"}, "alert:write", false},
		{[]string{"alert:read"}, "datasource:read", false},
		{[]string{"*:read"}, "alert:read", true},
		{[]string{"*:read"}, "alert:write", false},
		{[]string{"alert:*"}, "alert:write", true},
		{[]string{"*:*"}, "anything:goes", true},
		{[]string{"*"}, "anything:goes", true},
		{[]string{"ALERT:READ"}, "alert:read", true}, // case-insensitive
		{nil, "alert:read", false},
		{[]string{""}, "alert:read", false},
		// hard-deny patterns
		{[]string{"*:write", "*:delete", "user:*"}, "alert:write", true},
		{[]string{"*:write", "*:delete", "user:*"}, "user:read", true},
		{[]string{"*:write", "*:delete", "user:*"}, "alert:read", false},
	}
	for _, c := range cases {
		if got := grantMatched(c.pats, c.token); got != c.want {
			t.Errorf("grantMatched(%v, %q) = %v, want %v", c.pats, c.token, got, c.want)
		}
	}
}
