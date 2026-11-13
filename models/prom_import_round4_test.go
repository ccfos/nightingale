package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePromRuleYAML_NormalizesSupportedDocumentShapes(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		group   string
		record  string
		alert   string
	}{
		{
			name:    "groups",
			payload: "groups:\n- name: production\n  interval: 30s\n  rules:\n  - alert: Down\n    expr: up == 0",
			group:   "production", alert: "Down",
		},
		{name: "list", payload: "- alert: Down\n  expr: up == 0", group: "imported_rules", alert: "Down"},
		{name: "single recording rule", payload: "record: job:up\nexpr: sum(up)", group: "imported_rules", record: "job:up"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			groups, err := ParsePromRuleYAML(tc.payload)

			require.NoError(t, err)
			require.Len(t, groups, 1)
			require.Equal(t, tc.group, groups[0].Name)
			require.Len(t, groups[0].Rules, 1)
			require.Equal(t, tc.alert, groups[0].Rules[0].Alert)
			require.Equal(t, tc.record, groups[0].Rules[0].Record)
			require.NotEmpty(t, groups[0].Rules[0].Expr)
			if tc.name == "groups" {
				require.Equal(t, "30s", groups[0].Interval)
			}
		})
	}
}

func TestParsePromRuleYAML_RejectsEmptyMalformedAndUnrecognizedDocuments(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "blank", payload: " \n ", want: "payload is empty"},
		{name: "malformed", payload: "alert: [", want: "invalid yaml format"},
		{name: "unrecognized", payload: "name: not-a-rule", want: "empty or invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			groups, err := ParsePromRuleYAML(tc.payload)

			require.ErrorContains(t, err, tc.want)
			require.Nil(t, groups)
		})
	}
}

func TestDealPromGroup_SkipsRecordingRulesAndUsesGroupIntervals(t *testing.T) {
	groups := []PromRuleGroup{
		{Name: "default", Rules: []PromRule{{Record: "job:up", Expr: "sum(up)"}, {Alert: "Down", Expr: "up == 0"}}},
		{Name: "fast", Interval: "15s", Rules: []PromRule{{Alert: "High", Expr: "load > 2", For: "30s"}}},
	}
	sources := []DatasourceQuery{{MatchType: 0, Op: "in", Values: []interface{}{float64(7)}}}

	rules := DealPromGroup(groups, sources, 1)

	require.Len(t, rules, 2)
	require.Equal(t, "Down", rules[0].Name)
	require.Equal(t, "@every 60s", rules[0].CronPattern)
	require.Equal(t, "High", rules[1].Name)
	require.Equal(t, "@every 15s", rules[1].CronPattern)
	require.Equal(t, 30, rules[1].PromForDuration)
	for _, rule := range rules {
		require.Equal(t, 1, rule.Disabled)
		require.Equal(t, sources, rule.DatasourceQueries)
	}
}
