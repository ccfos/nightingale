package prom

import (
	"testing"
)

func TestAddLabelToPromQL(t *testing.T) {
	testCases := []struct {
		name     string
		label    string
		promql   string
		expected string
	}{
		{
			name:     "Add label to PromQL without existing labels",
			label:    "{ident=\"dev-backup-01\"}",
			promql:   "sum(\n  irate(container_cpu_usage_seconds_total{image!=\"\", image!~\".*pause.*\"}[3m])\n) by (pod,namespace,container,image)\n/\nsum(\n  container_spec_cpu_quota/container_spec_cpu_period\n) by (pod,namespace,container,image)",
			expected: "sum(\n  irate(container_cpu_usage_seconds_total{ident=\"dev-backup-01\",image!=\"\", image!~\".*pause.*\"}[3m])\n) by (pod,namespace,container,image)\n/\nsum(\n  container_spec_cpu_quota{ident=\"dev-backup-01\"}/container_spec_cpu_period{ident=\"dev-backup-01\"}\n) by (pod,namespace,container,image)",
		},
		{
			name:     "Add label to PromQL without existing labels",
			label:    "{new_label=\"value\"}",
			promql:   "metric_name{}",
			expected: "metric_name{new_label=\"value\"}",
		},
		{
			name:     "Add label to PromQL without existing labels",
			label:    "",
			promql:   "avg without (mode,cpu) ( irate(node_cpu_seconds_total{mode=\"idle\"}[2m]) ) * 100",
			expected: "avg without (mode,cpu) ( irate(node_cpu_seconds_total{mode=\"idle\"}[2m]) ) * 100",
		},
		{
			name:     "Add label to PromQL without existing labels",
			label:    "{new_label=\"value\"}",
			promql:   "metric_name",
			expected: "metric_name{new_label=\"value\"}",
		},
		{
			name:     "Add label to PromQL with existing labels",
			label:    "{new_label=\"value\"}",
			promql:   "metric_name{existing_label=\"value\"}",
			expected: "metric_name{new_label=\"value\",existing_label=\"value\"}",
		},
		{
			name:     "Add label with spaces to PromQL",
			label:    "{ new_label = \"value\" }",
			promql:   "metric_name",
			expected: "metric_name{new_label=\"value\"}",
		},
		{
			name:     "Add label to PromQL with multiple metrics",
			label:    "{new_label=\"value\"}",
			promql:   "metric1 + metric2{existing_label=\"value\"}",
			expected: "metric1{new_label=\"value\"} + metric2{new_label=\"value\",existing_label=\"value\"}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := AddLabelToPromQL(tc.label, tc.promql)
			if result != tc.expected {
				t.Errorf("Expected: %s, Got: %s", tc.expected, result)
			}
		})
	}
}
