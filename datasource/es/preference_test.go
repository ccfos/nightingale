package es

import (
	"context"
	"testing"

	dskittypes "github.com/ccfos/nightingale/v6/dskit/types"
)

func TestRulePreference(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"no call context", context.Background(), ""},
		{"user query", dskittypes.WithCallContext(context.Background(), dskittypes.CallContext{Operator: "root"}), ""},
		{"alert rule", dskittypes.WithCallContext(context.Background(), dskittypes.CallContext{Operator: "alert_rule", RuleID: 94}), "n9e-alert_rule-94"},
		{"recording rule", dskittypes.WithCallContext(context.Background(), dskittypes.CallContext{Operator: "recording_rule", RuleID: 94}), "n9e-recording_rule-94"},
	}
	for _, c := range cases {
		if got := rulePreference(c.ctx); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
